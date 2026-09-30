#!/usr/bin/env bash
# Install immutable tools only; no service, configuration, wallet or state mutation.
set -euo pipefail
cd -- "$(dirname -- "$0")"
case "$(uname -s)" in Linux) os=linux;; Darwin) os=darwin;; *) echo 'Unsupported OS'; exit 1;; esac
case "$(uname -m)" in x86_64) arch=amd64;; arm64|aarch64) arch=arm64;; *) echo 'Unsupported architecture'; exit 1;; esac
test "$(cat TARGET)" = "$os-$arch"
if command -v sha256sum >/dev/null 2>&1; then sha256sum -c SHA256SUMS; else shasum -a 256 -c SHA256SUMS; fi
commit=$(cat SOURCE_COMMIT)
[[ "$commit" =~ ^[0-9a-f]{40}$ ]]
base="${HASHBURST_TOOLS_DIR:-$HOME/.local/hashburst-tools}"
test ! -L "$base"
mkdir -p -- "$base"
destination="$base/$commit-$os-$arch"
if test -e "$destination" || test -L "$destination"; then
  echo "Destination already exists; retained: $destination"; exit 1
fi
stage=$(mktemp -d "$base/.stage-XXXXXXXX")
trap 'rm -rf -- "$stage"' EXIT
for tool in hashburst-wallet hashburst-testnet hashburst-mainnet hvm-apow-miner; do
  install -m 0755 "$tool" "$stage/$tool"
done
for file in prompt.py WALLET.md RELEASE-GATES.md economics.draft.json SOURCE_COMMIT TARGET SHA256SUMS STATUS.txt RELEASE_NOTES.md install-tools.sh; do
  install -m 0644 "$file" "$stage/$file"
done
chmod 0755 "$stage"
mv -- "$stage" "$destination"
trap - EXIT
printf 'TOOLS_INSTALLED=%s\nNO_SERVICE_STARTED_NO_NETWORK_ACTIVATED\n' "$destination"
