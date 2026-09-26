#!/usr/bin/env bash
set -euo pipefail
ACTION="${1:-}"
NODES="${NODES:-4}"
BR="hb3e-br0"
cleanup() {
  for i in $(seq 0 $((NODES-1))); do ip netns del "hb3e-n$i" 2>/dev/null || true; ip link del "hb3e-r$i" 2>/dev/null || true; done
  ip link del "$BR" 2>/dev/null || true
}
setup() {
  cleanup
  ip link add "$BR" type bridge
  ip addr add 10.203.0.1/24 dev "$BR"
  ip link set "$BR" up
  for i in $(seq 0 $((NODES-1))); do
    ns="hb3e-n$i"; rv="hb3e-r$i"; nv="hb3e-p$i"; ipaddr="10.203.0.$((11+i))"
    ip netns add "$ns"
    ip link add "$rv" type veth peer name "$nv"
    ip link set "$nv" netns "$ns"
    ip link set "$rv" master "$BR"
    ip link set "$rv" up
    ip netns exec "$ns" ip link set lo up
    ip netns exec "$ns" ip link set "$nv" name eth0
    ip netns exec "$ns" ip addr add "$ipaddr/24" dev eth0
    ip netns exec "$ns" ip link set eth0 up
  done
}
case "$ACTION" in setup) setup;; cleanup) cleanup;; *) echo "usage: $0 setup|cleanup" >&2; exit 2;; esac
