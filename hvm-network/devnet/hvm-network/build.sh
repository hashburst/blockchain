#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
BIN="${BIN:-$ROOT/devnet/hvm-network/bin}"
mkdir -p "$BIN"
cd "$ROOT"
go test ./consensus ./protocolv2 ./hvm ./blockchain
go build -trimpath -o "$BIN/hvm-network-fixture" ./devnet/hvm-network/cmd/fixture
go build -trimpath -o "$BIN/hvm-network-node" ./devnet/hvm-network/cmd/node
go build -trimpath -o "$BIN/hvm-network-ctl" ./devnet/hvm-network/cmd/ctl
printf 'HVM_NETWORK_BUILD_OK\n'
