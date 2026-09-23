#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
MODE="${MODE:-loopback}"
NODES="${NODES:-4}"
OUT="${OUT:-$ROOT/devnet/phase3e/run-$MODE-$NODES}"
BIN="$ROOT/devnet/phase3e/bin"
MANIFEST="$OUT/manifest.json"
PIDS="$OUT/pids"
LOGS="$OUT/logs"
export MODE NODES OUT BIN

"$ROOT/devnet/phase3e/preflight.sh"
"$ROOT/devnet/phase3e/build.sh"
"$BIN/phase3e-fixture" --out "$OUT" --nodes "$NODES" --mode "$MODE"
mkdir -p "$PIDS" "$LOGS"

cleanup_nodes() {
  if [ -d "$PIDS" ]; then
    for f in "$PIDS"/*.pid; do [ -e "$f" ] || continue; pid="$(cat "$f" 2>/dev/null || true)"; [ -n "$pid" ] && kill "$pid" 2>/dev/null || true; done
    sleep 0.3
    for f in "$PIDS"/*.pid; do [ -e "$f" ] || continue; pid="$(cat "$f" 2>/dev/null || true)"; [ -n "$pid" ] && kill -9 "$pid" 2>/dev/null || true; done
  fi
}
cleanup_all() {
  cleanup_nodes
  if [ "$MODE" = netns ]; then NODES="$NODES" "$ROOT/devnet/phase3e/netns.sh" cleanup || true; fi
}
trap cleanup_all EXIT INT TERM
if [ "$MODE" = netns ]; then NODES="$NODES" "$ROOT/devnet/phase3e/netns.sh" setup; fi

node_field() { jq -r ".nodes[$1].$2" "$MANIFEST"; }
health_url() { printf 'http://%s:%s/health' "$(node_field "$1" ip)" "$(node_field "$1" rpc_port)"; }
rpc_url() { printf 'http://%s:%s/rpc' "$(node_field "$1" ip)" "$(node_field "$1" rpc_port)"; }
start_url() { printf 'http://%s:%s/control/start' "$(node_field "$1" ip)" "$(node_field "$1" rpc_port)"; }
stop_url() { printf 'http://%s:%s/control/stop' "$(node_field "$1" ip)" "$(node_field "$1" rpc_port)"; }

wait_consensus_state() {
  want="$1"; skip="${2:--1}"; deadline=$((SECONDS+12))
  while [ "$SECONDS" -lt "$deadline" ]; do
    ok=1
    for i in $(seq 0 $((NODES-1))); do
      [ "$i" = "$skip" ] && continue
      if j="$(curl -fsS --max-time 1 "$(health_url "$i")" 2>/dev/null)"; then
        got="$(printf '%s' "$j" | jq -r '.consensus_started')"
        [ "$got" = "$want" ] || ok=0
      else
        ok=0
      fi
    done
    [ "$ok" = 1 ] && return 0
    sleep 0.2
  done
  return 1
}
start_consensus_node() {
  i="$1"; label="${2:-start}"
  if ! j="$(curl -fsS --max-time 5 -X POST "$(start_url "$i")" 2>/dev/null)"; then
    echo "ERROR: $label node $i /control/start failed" >&2
    tail -80 "$LOGS/node$i.log" >&2 || true
    return 1
  fi
  if ! printf '%s' "$j" | jq -e '.started == true' >/dev/null 2>&1; then
    echo "ERROR: $label node $i invalid /control/start response: $j" >&2
    return 1
  fi
}
stop_consensus_all() {
  skip="${1:--1}"
  for i in $(seq 0 $((NODES-1))); do
    [ "$i" = "$skip" ] && continue
    curl -fsS --max-time 1 -X POST "$(stop_url "$i")" >/dev/null 2>&1 || true
  done
  wait_consensus_state false "$skip" || { echo "ERROR: consensus stop timeout" >&2; return 1; }
}
start_consensus_alive() {
  skip="${1:--1}"
  for i in $(seq 0 $((NODES-1))); do
    [ "$i" = "$skip" ] && continue
    start_consensus_node "$i" "consensus restart" || return 1
  done
  wait_consensus_state true "$skip" || { echo "ERROR: consensus start timeout" >&2; return 1; }
}

start_node() {
  i="$1"; log="$LOGS/node$i.log"; pidfile="$PIDS/node$i.pid"
  cmd=("$BIN/phase3e-node" --manifest "$MANIFEST" --node-dir "$OUT/node$i" --index "$i")
  if [ "$MODE" = netns ]; then
    ip netns exec "hb3e-n$i" "${cmd[@]}" >"$log" 2>&1 &
  else
    "${cmd[@]}" >"$log" 2>&1 &
  fi
  echo $! > "$pidfile"
}
for i in $(seq 0 $((NODES-1))); do start_node "$i"; done

wait_health() {
  i="$1"; deadline=$((SECONDS+30))
  while [ "$SECONDS" -lt "$deadline" ]; do curl -fsS --max-time 1 "$(health_url "$i")" >/dev/null 2>&1 && return 0; sleep 0.2; done
  echo "ERROR: node $i health timeout" >&2; tail -80 "$LOGS/node$i.log" >&2 || true; return 1
}
for i in $(seq 0 $((NODES-1))); do wait_health "$i"; done

# Wait for full mesh before consensus starts.
deadline=$((SECONDS+30))
while :; do
  ok=1
  for i in $(seq 0 $((NODES-1))); do
    p="$(curl -fsS "$(health_url "$i")" | jq -r '.peer_count')"
    [ "$p" -ge $((NODES-1)) ] || ok=0
  done
  [ "$ok" = 1 ] && break
  [ "$SECONDS" -lt "$deadline" ] || { echo "ERROR: full-mesh timeout" >&2; exit 1; }
  sleep 0.3
done

BOOT="$(jq -r '.bootstrap_height' "$MANIFEST")"
NEXT=$((BOOT+1))
PROP="$(curl -fsS -X POST -H 'Content-Type: application/json' "$(rpc_url 0)" -d "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"hb_getProposer\",\"params\":[$NEXT,0]}" | jq -r '.result.proposer.id')"
PROP_IDX="$(jq -r --arg p "$PROP" '.nodes[] | select((.validator_id|ascii_downcase)==($p|ascii_downcase)) | .index' "$MANIFEST")"
# /control/start is synchronous: a successful response means reactor.Start()
# completed. Start non-proposers first, but do NOT poll/wait between that and
# the round-0 proposer. The previous readiness poll let the non-proposer quorum
# run several view changes and finalize blocks while the scheduled proposer was
# intentionally held offline.
for i in $(seq 0 $((NODES-1))); do
  [ "$i" = "$PROP_IDX" ] && continue
  start_consensus_node "$i" "initial non-proposer start" || exit 1
done
start_consensus_node "$PROP_IDX" "initial proposer start" || exit 1
wait_consensus_state true || { echo "ERROR: initial consensus start timeout" >&2; exit 1; }

echo "PHASE3E_CONSENSUS_START_BARRIER_OK proposer_node=$PROP_IDX bootstrap=$BOOT"
echo "PHASE3E_CONSENSUS_STARTED proposer_node=$PROP_IDX bootstrap=$BOOT"

wait_height_at_least() {
  i="$1"; target="$2"; timeout="${3:-40}"; deadline=$((SECONDS+timeout))
  while [ "$SECONDS" -lt "$deadline" ]; do
    if j="$(curl -fsS --max-time 1 "$(health_url "$i")" 2>/dev/null)"; then
      h="$(printf '%s' "$j" | jq -r '.height')"
      [ "$h" -ge "$target" ] && return 0
    fi
    sleep 0.3
  done
  return 1
}

wait_finalized_delta() {
  base="$1"; delta="$2"; timeout="${3:-30}"; deadline=$((SECONDS+timeout)); target=$((base+delta))
  while [ "$SECONDS" -lt "$deadline" ]; do
    min=999999999
    alive=0
    for i in $(seq 0 $((NODES-1))); do
      if j="$(curl -fsS --max-time 1 "$(health_url "$i")" 2>/dev/null)"; then
        h="$(printf '%s' "$j" | jq -r '.finalized_height')"; [ "$h" -lt "$min" ] && min="$h"; alive=$((alive+1))
      fi
    done
    [ "$alive" -gt 0 ] && [ "$min" -ge "$target" ] && { echo "$min"; return 0; }
    sleep 0.2
  done
  return 1
}
dump_consensus_diagnostics() {
  echo "=== PHASE3E_CONSENSUS_DIAGNOSTICS ===" >&2
  for i in $(seq 0 $((NODES-1))); do
    echo "--- node$i health ---" >&2
    curl -fsS --max-time 2 "$(health_url "$i")" 2>/dev/null | jq '{node_index,height,finalized_height,peer_count,consensus_started,consensus_requested,reactor,network}' >&2 || true
    echo "--- node$i consensus log tail ---" >&2
    grep -E 'consensus|sync:|rejected|reactor|finaliz|proposal|prevote|precommit|round' "$LOGS/node$i.log" 2>/dev/null | tail -120 >&2 || tail -120 "$LOGS/node$i.log" >&2 || true
  done
}

BASE="$BOOT"
FINAL="$(wait_finalized_delta "$BASE" 3 35)" || { dump_consensus_diagnostics; echo "ERROR: initial finality failed" >&2; exit 1; }
echo "PHASE3E_FINALITY_OK finalized=$FINAL"

"$BIN/phase3e-ctl" --manifest "$MANIFEST" --out "$OUT" --command canary

current_min_finalized() {
  min=999999999
  for i in $(seq 0 $((NODES-1))); do
    h="$(curl -fsS "$(health_url "$i")" | jq -r '.finalized_height')"
    [ "$h" -lt "$min" ] && min="$h"
  done
  echo "$min"
}
FINAL="$(current_min_finalized)"
echo "PHASE3E_MPR_FINALIZED finalized=$FINAL"
stop_consensus_all
FINAL="$(current_min_finalized)"

if [ "$MODE" = netns ]; then
  # Packet loss/latency: all validators keep enough connectivity to finalize.
  BASE="$FINAL"
  for i in $(seq 0 $((NODES-1))); do ip netns exec "hb3e-n$i" tc qdisc replace dev eth0 root netem delay 35ms 10ms loss 5%; done
  start_consensus_alive
  FINAL="$(wait_finalized_delta "$BASE" 1 45)" || { echo "ERROR: finality under packet loss failed" >&2; exit 1; }
  for i in $(seq 0 $((NODES-1))); do ip netns exec "hb3e-n$i" tc qdisc del dev eth0 root 2>/dev/null || true; done
  stop_consensus_all
  echo "PHASE3E_PACKET_LOSS_OK finalized=$FINAL"

  # 1-node partition: start all validators, then isolate one while its reactor
  # is live. The reachable quorum must continue and the isolated node must
  # catch up after reconnect.
  ISO=$((NODES-1))
  start_consensus_alive
  ip netns exec "hb3e-n$ISO" tc qdisc replace dev eth0 root netem loss 100%
  tc qdisc replace dev "hb3e-r$ISO" root netem loss 100%
  sleep 0.5
  BASE=999999999
  for i in $(seq 0 $((NODES-2))); do
    h="$(curl -fsS "$(health_url "$i")" | jq -r '.finalized_height')"
    [ "$h" -lt "$BASE" ] && BASE="$h"
  done
  # Only require the non-isolated quorum to move from the post-partition base.
  deadline=$((SECONDS+40)); moved=0
  while [ "$SECONDS" -lt "$deadline" ]; do
    min=999999999
    for i in $(seq 0 $((NODES-2))); do h="$(curl -fsS "$(health_url "$i")" | jq -r '.finalized_height')"; [ "$h" -lt "$min" ] && min="$h"; done
    if [ "$min" -ge $((BASE+1)) ]; then FINAL="$min"; moved=1; break; fi
    sleep 0.2
  done
  [ "$moved" = 1 ] || { echo "ERROR: majority stalled during 1-node partition" >&2; exit 1; }
  stop_consensus_all "$ISO"
  ip netns exec "hb3e-n$ISO" tc qdisc del dev eth0 root 2>/dev/null || true
  tc qdisc del dev "hb3e-r$ISO" root 2>/dev/null || true
  curl -fsS --max-time 2 -X POST "$(stop_url "$ISO")" >/dev/null 2>&1 || true
  wait_consensus_state false || { echo "ERROR: isolated validator did not stop after reconnect" >&2; exit 1; }
  wait_height_at_least "$ISO" "$FINAL" 40 || { echo "ERROR: isolated validator did not catch up after reconnect" >&2; exit 1; }
  start_consensus_alive
  BASE="$FINAL"
  FINAL="$(wait_finalized_delta "$BASE" 1 45)" || { echo "ERROR: finality after reconnect failed" >&2; exit 1; }
  stop_consensus_all
  echo "PHASE3E_PARTITION_RECONNECT_OK isolated=$ISO finalized=$FINAL"

  # Proposer crash: stop the next scheduled proposer, require view-change finality, then restart/catch up.
  NEXT=$((FINAL+1))
  PROP="$(curl -fsS -X POST -H 'Content-Type: application/json' "$(rpc_url 0)" -d "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"hb_getProposer\",\"params\":[$NEXT,0]}" | jq -r '.result.proposer.id')"
  CRASH="$(jq -r --arg p "$PROP" '.nodes[] | select((.validator_id|ascii_downcase)==($p|ascii_downcase)) | .index' "$MANIFEST")"
  CRASH_PID="$(cat "$PIDS/node$CRASH.pid")"
  kill "$CRASH_PID"
  deadline=$((SECONDS+8))
  while kill -0 "$CRASH_PID" 2>/dev/null && [ "$SECONDS" -lt "$deadline" ]; do sleep 0.1; done
  kill -0 "$CRASH_PID" 2>/dev/null && { echo "ERROR: proposer process did not stop" >&2; exit 1; }
  start_consensus_alive "$CRASH"
  deadline=$((SECONDS+45)); moved=0
  while [ "$SECONDS" -lt "$deadline" ]; do
    min=999999999
    for i in $(seq 0 $((NODES-1))); do [ "$i" = "$CRASH" ] && continue; h="$(curl -fsS "$(health_url "$i")" | jq -r '.finalized_height')"; [ "$h" -lt "$min" ] && min="$h"; done
    if [ "$min" -ge "$NEXT" ]; then FINAL="$min"; moved=1; break; fi
    sleep 0.2
  done
  [ "$moved" = 1 ] || { echo "ERROR: proposer crash did not recover via view change" >&2; exit 1; }
  stop_consensus_all "$CRASH"
  start_node "$CRASH"; wait_health "$CRASH"
  wait_height_at_least "$CRASH" "$FINAL" 45 || { echo "ERROR: restarted proposer did not catch up" >&2; exit 1; }
  start_consensus_alive
  BASE="$FINAL"
  FINAL="$(wait_finalized_delta "$BASE" 1 45)" || { echo "ERROR: restarted proposer node did not rejoin finality" >&2; exit 1; }
  stop_consensus_all
  echo "PHASE3E_PROPOSER_CRASH_OK node=$CRASH finalized=$FINAL"
fi

start_consensus_alive
if [ "$NODES" -ge 5 ]; then "$BIN/phase3e-ctl" --manifest "$MANIFEST" --out "$OUT" --command slash --target $((NODES-1)); fi
if [ "$NODES" -ge 6 ]; then "$BIN/phase3e-ctl" --manifest "$MANIFEST" --out "$OUT" --command unbond --target $((NODES-2)); fi
stop_consensus_all

"$BIN/phase3e-ctl" --manifest "$MANIFEST" --out "$OUT" --command status > "$OUT/final-status.txt"
echo "PHASE3E_RUN_OK mode=$MODE nodes=$NODES out=$OUT"
