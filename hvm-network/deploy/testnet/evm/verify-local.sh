#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../../../.." && pwd)"
GO_BIN="${GO_BIN:-go}"
test "$("$GO_BIN" version | awk '{print $3}')" = go1.25.7
export GOTOOLCHAIN=local
export GOMAXPROCS="${GOMAXPROCS:-2}"
REPORT="$(mktemp -d "${TMPDIR:-/tmp}/hvm-evm-integration.XXXXXX")"
trap 'echo "STOP: tests failed; diagnostics=$REPORT"' ERR
(cd "$ROOT/evm/execution" && "$GO_BIN" test -p 1 -race -count=1 -v ./...) >"$REPORT/execution.log" 2>&1
(cd "$ROOT/hvm-network" && "$GO_BIN" test -p 1 -count=1 ./...) >"$REPORT/node-suite.log" 2>&1
(cd "$ROOT/hvm-network" && "$GO_BIN" test -p 1 -race -count=1 -v ./blockchain -run '^TestEVM') >"$REPORT/integration.log" 2>&1
(cd "$ROOT/hvm-network" && "$GO_BIN" vet ./...) >"$REPORT/vet.log" 2>&1
echo "HVM_EVM_LOCAL_INTEGRATION_OK"
echo "FOUR_LOCAL_VALIDATORS_LIBP2P_ACCOUNTING_REPLAY_WS_OBSERVER_OK"
echo "NO_PRODUCTION_STATE_MODIFIED"
echo "LIVE_ROLLOUT_AND_METAMASK_NOT_CERTIFIED"
echo "REPORT=$REPORT"
