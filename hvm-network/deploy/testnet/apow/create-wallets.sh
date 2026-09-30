#!/bin/sh
set -eu
cd "$(dirname "$0")"
case "$(uname -s):$(uname -m)" in
 Darwin:arm64) BIN=./hvm-wallet-init-darwin-arm64 ;;
 Darwin:x86_64) BIN=./hvm-wallet-init-darwin-amd64 ;;
 *) echo 'Questo script e per macOS; nessun wallet creato.' >&2; exit 1 ;;
esac
shasum -a 256 -c SHA256SUMS
"$BIN" --network testnet --directory "$HOME/.hashburst-native-wallets"
"$BIN" --network mainnet --directory "$HOME/.hashburst-native-wallets"
echo 'NATIVE_WALLETS_READY_NO_ALLOCATION_NO_TRANSACTION_NO_SERVICE_CHANGED'
