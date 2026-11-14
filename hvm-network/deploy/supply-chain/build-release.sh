#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
OUT="${1:?absolute output directory required}"
case "$OUT" in /*) ;; *) echo "absolute output path required" >&2; exit 2;; esac
test -z "$(git -C "$ROOT" status --porcelain)" || { echo "build requires a clean checkout" >&2; exit 1; }
mkdir -p "$OUT"
export GOTOOLCHAIN=local CGO_ENABLED=0 GOOS=linux GOARCH=amd64
build() { (cd "$ROOT" && go build -buildvcs=false -trimpath -ldflags="-buildid=" -o "$OUT/$1" "./cmd/$2"); }
build hashburst-mainnet hashburst-mainnet
build hashburst-testnet hashburst-testnet
build hvm-mainnet-evm-gateway hvm-mainnet-evm-gateway
git -C "$ROOT" rev-parse HEAD > "$OUT/SOURCE_COMMIT"
printf 'G5_RELEASE_BUILT=%s\n' "$OUT"
