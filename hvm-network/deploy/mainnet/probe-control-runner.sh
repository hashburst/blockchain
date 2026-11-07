#!/usr/bin/env bash
set -euo pipefail
# READ-ONLY founder coordinator probe.
echo "HVM_CONTROL_RUNNER_PROBE"
date -u '+UTC=%Y-%m-%dT%H:%M:%SZ'
echo "hostname=$(hostname -f 2>/dev/null || hostname)"
. /etc/os-release; echo "os=$PRETTY_NAME"
echo "arch=$(uname -m) cpus=$(nproc)"
free -h
df -h /
for x in curl git ssh ssh-keygen ssh-keyscan python3 tar systemctl sudo; do command -v "$x" || echo "MISSING:$x"; done
ps -eo user,pid,args | grep -E '[R]unner\.(Listener|Worker)|[r]unsvc\.sh' || true
for h in 77.90.188.153 77.90.188.154 77.90.188.155 77.90.188.157 64.31.4.9; do timeout 4 bash -c "</dev/tcp/$h/22" 2>/dev/null && echo "$h tcp/22=OPEN" || echo "$h tcp/22=UNREACHABLE"; done
curl -fsSI --max-time 10 https://api.github.com/ | head -1
echo "HVM_CONTROL_RUNNER_PROBE_COMPLETE"
