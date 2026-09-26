#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
MODE="${MODE:-loopback}"
NODES="${NODES:-4}"
OUT="${OUT:-$ROOT/devnet/hvm-network/run-$MODE-$NODES}"

case "$NODES" in 4|5|6) ;; *) echo "ERROR: NODES must be 4, 5 or 6" >&2; exit 2;; esac
case "$MODE" in loopback|netns) ;; *) echo "ERROR: MODE must be loopback or netns" >&2; exit 2;; esac
command -v realpath >/dev/null || { echo "ERROR: missing realpath" >&2; exit 1; }
SAFE_OUT_ROOT="$ROOT/devnet/hvm-network"
OUT_ABS="$(realpath -m "$OUT")"
case "$OUT_ABS" in
  "$SAFE_OUT_ROOT"/run-*) ;;
  *) echo "ERROR: OUT must be a run-* directory under $SAFE_OUT_ROOT; got $OUT_ABS" >&2; exit 2;;
esac
OUT="$OUT_ABS"
export OUT

for c in bash go jq curl sha256sum awk sed grep seq; do command -v "$c" >/dev/null || { echo "ERROR: missing $c" >&2; exit 1; }; done
GV="$(GOTOOLCHAIN=local go version | awk '{print $3}')"
if [ "$GV" != "go1.25.7" ]; then
  echo "ERROR: Phase 3E requires Go 1.25.7 exactly; found $GV" >&2
  exit 1
fi
if [ "$MODE" = netns ]; then
  [ "$(id -u)" = 0 ] || { echo "ERROR: MODE=netns requires root" >&2; exit 1; }
  for c in ip tc; do command -v "$c" >/dev/null || { echo "ERROR: missing $c" >&2; exit 1; }; done
fi
[ -f "$ROOT/devnet/hvm-network/cmd/node/main.go" ] || { echo "ERROR: Phase 3E harness missing from source tree" >&2; exit 1; }
[ -f "$ROOT/go.mod" ] || { echo "ERROR: run from extracted Phase 3E source tree" >&2; exit 1; }

echo "HVM_NETWORK_PREFLIGHT_OK mode=$MODE nodes=$NODES go=$GV out=$OUT"
