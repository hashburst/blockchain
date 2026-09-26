#!/usr/bin/env bash
# Collect only public health data and devnet logs, never keys/databases/journals.
set -uo pipefail
OUT="${1:?run directory required}"
MODE="${2:?mode required}"
NODES="${3:?node count required}"
case "$OUT" in */devnet/hvm-network/run-*) ;; *) exit 2;; esac
case "$MODE" in loopback|netns) ;; *) exit 2;; esac
[[ "$NODES" =~ ^[4-6]$ ]] || exit 2
umask 077
DEST="$OUT/diagnostics"
mkdir -p "$DEST"
date -u +%FT%TZ > "$DEST/collected-at.txt"
if [ -d "$OUT/control" ]; then
  mkdir -p "$DEST/control"
  for f in "$OUT/control"/node*-start-*; do
    [ ! -f "$f" ] || cp -- "$f" "$DEST/control/"
  done
fi
for ((i=0;i<NODES;i++)); do
  endpoint="$(jq -er --argjson i "$i" '.nodes[$i] | "http://\(.ip):\(.rpc_port)/health"' "$OUT/manifest.json")" || continue
  curl --noproxy '*' -sS --max-time 3 -D "$DEST/node$i.headers" "$endpoint" > "$DEST/node$i.health.json" 2> "$DEST/node$i.http-error.txt"
  echo "$?" > "$DEST/node$i.curl-exit.txt"
  if [ -f "$OUT/logs/node$i.log" ]; then
    tail -n 2000 "$OUT/logs/node$i.log" > "$DEST/node$i.log-tail.txt"
    grep -Ei 'rejected|stopped|error|panic|timeout|failed|finaliz' "$OUT/logs/node$i.log" > "$DEST/node$i.events.txt" || true
  fi
  if [ "$MODE" = netns ]; then
    ip netns exec "hb3e-n$i" tc -s qdisc show > "$DEST/node$i.qdisc.txt" 2>&1 || true
    ip netns exec "hb3e-n$i" ss -ntup > "$DEST/node$i.sockets.txt" 2>&1 || true
  fi
done
tar -C "$OUT" -czf "$OUT/diagnostics.tar.gz" diagnostics
echo "HVM_NETWORK_DIAGNOSTICS=$OUT/diagnostics.tar.gz"
