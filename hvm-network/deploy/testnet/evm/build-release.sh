#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
OUT="${1:?absolute output directory required}"
case "$OUT" in /*) ;; *) echo 'absolute output path required' >&2; exit 2;; esac
test -z "$(git -C "$ROOT" status --porcelain)" || { echo "build requires a clean reviewed checkout" >&2; exit 1; }
mkdir -p "$OUT"
export GOTOOLCHAIN=local CGO_ENABLED=0
(cd "$ROOT" && go build -buildvcs=false -trimpath -o "$OUT/hashburst-testnet" ./cmd/hashburst-testnet && go build -buildvcs=false -trimpath -o "$OUT/hvm-evm-gateway" ./cmd/hvm-evm-gateway)
cp "$ROOT/deploy/testnet/evm/"*.py "$ROOT/deploy/testnet/evm/"*.html "$ROOT/deploy/testnet/evm/routes.conf" "$ROOT/deploy/testnet/evm/README.md" "$OUT/"
git -C "$ROOT" rev-parse HEAD > "$OUT/SOURCE_COMMIT"
(cd "$OUT" && sha256sum hashburst-testnet hvm-evm-gateway *.py *.html routes.conf README.md SOURCE_COMMIT > SHA256SUMS)
printf 'EVM_RELEASE_BUILT=%s\n' "$OUT"
