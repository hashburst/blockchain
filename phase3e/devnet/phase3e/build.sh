#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
BIN="${BIN:-$ROOT/devnet/phase3e/bin}"
mkdir -p "$BIN"
cd "$ROOT"
go test ./consensus ./protocolv2 ./hvm ./blockchain
go build -trimpath -o "$BIN/phase3e-fixture" ./devnet/phase3e/cmd/fixture
go build -trimpath -o "$BIN/phase3e-node" ./devnet/phase3e/cmd/node
go build -trimpath -o "$BIN/phase3e-ctl" ./devnet/phase3e/cmd/ctl
printf 'PHASE3E_BUILD_OK\n'
