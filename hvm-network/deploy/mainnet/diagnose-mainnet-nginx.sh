#!/usr/bin/env bash
set -u

V=/etc/nginx/sites-available/blockchainapi.one.conf
E=/etc/nginx/sites-enabled/blockchainapi.one.conf
MAINNET=/etc/nginx/snippets/hvm-mainnet-evm.conf
URL=https://blockchainapi.one/api/hashburst/hvm/mainnet/evm
BODY='{"jsonrpc":"2.0","id":1,"method":"eth_chainId","params":[]}'

echo "=== HVM_MAINNET_NGINX_DIAGNOSTICS ==="
date -u '+UTC=%Y-%m-%dT%H:%M:%SZ'
printf 'AVAILABLE='; readlink -f "$V" 2>/dev/null || true
printf 'ENABLED='; readlink -f "$E" 2>/dev/null || true
printf 'MAINNET_SNIPPET='; test -f "$MAINNET" && echo present || echo absent
nginx -t 2>&1 || true

echo "=== EFFECTIVE_HTTPS_VHOST ==="
nginx -T 2>&1 | python3 -c '
import sys
lines=sys.stdin.read().splitlines(); blocks=[]; depth=0; start=None; server_depth=None
for i,line in enumerate(lines):
    s=line.split("#",1)[0]; before=depth
    if start is None and s.strip().startswith("server") and "{" in s: start=i; server_depth=before
    depth += s.count("{")-s.count("}")
    if start is not None and depth==server_depth:
        block=lines[start:i+1]
        text="\n".join(block)
        if "server_name" in text and "blockchainapi.one" in text and any("listen" in x and "443" in x and "ssl" in x for x in block):
            blocks.append(block)
        start=None
print("HTTPS_BLOCKS="+str(len(blocks)))
for n,b in enumerate(blocks,1):
    print("--- HTTPS_BLOCK_%d ---"%n)
    for x in b:
        if any(k in x for k in ("listen","server_name","include","location","proxy_pass")): print(x)
' || true

echo "=== DIRECT_GATEWAY ==="
curl -sS --max-time 10 -o /tmp/hvm-mainnet-direct.body -w 'HTTP=%{http_code}\n' \
  -H 'Content-Type: application/json' --data "$BODY" http://127.0.0.1:18021/rpc || true
cat /tmp/hvm-mainnet-direct.body 2>/dev/null || true; echo

echo "=== LOCAL_TLS_SNI ==="
curl -sS --max-time 10 --resolve blockchainapi.one:443:127.0.0.1 \
  -o /tmp/hvm-mainnet-tls.body -D /tmp/hvm-mainnet-tls.headers -w 'HTTP=%{http_code}\n' \
  -H 'Content-Type: application/json' --data "$BODY" "$URL" || true
grep -Ei '^(HTTP/|server:|x-hvm-evm:|location:)' /tmp/hvm-mainnet-tls.headers 2>/dev/null || true
cat /tmp/hvm-mainnet-tls.body 2>/dev/null || true; echo
rm -f /tmp/hvm-mainnet-direct.body /tmp/hvm-mainnet-tls.body /tmp/hvm-mainnet-tls.headers
echo "=== END_HVM_MAINNET_NGINX_DIAGNOSTICS ==="
