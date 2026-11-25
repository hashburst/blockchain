#!/usr/bin/env bash
set -euo pipefail
umask 077

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
REPO="$(cd "$ROOT/.." && pwd)"
DEPLOY="$ROOT/deploy/mainnet"
SUPPLY="$ROOT/deploy/supply-chain"

KNOWN=/etc/hashburst-control/fleet_known_hosts
SOCK=/run/hashburst-control/ssh-agent.sock

VALIDATORS=(
    77.90.188.153
    77.90.188.154
    77.90.188.155
    77.90.188.157
)

VSERVICE=hashburst-hvm-mainnet.service

CHAIN_ID=4735489
TESTNET_CHAIN_ID=4735490

EXPECTED_CONFIG_DIGEST=86a9c1e1ed65c2e3afcaf469a3155fad94add9e88a2b90e5bc80e0c2e33b3fb3

EXPECTED_ARTIFACT_SOURCE_COMMIT=83cc7bc43325aaeebf24e11c58ecd80c336dd8a6

EXPECTED_PRODUCTION_SIGNATURE_SHA256=0aba5f6d53e8a9bb165234da1a54611c9afd6fadbf6cd45b25136e88efd33367

PACKAGE="${HASHBURST_PRODUCTION_PACKAGE:-/var/lib/hashburst-actions/g5-master-acceptance/master-${EXPECTED_ARTIFACT_SOURCE_COMMIT}/production-signed-release}"

ROLLBACK_ROOT="${HASHBURST_ROLLBACK_ROOT:-/var/lib/hashburst-actions/mainnet-evidence/rollback/2026-11-18-pre-g5-signed-runtime-upgrade}"
ROLLBACK_INDEX="$ROLLBACK_ROOT/FORENSIC-INDEX.json"
EXPECTED_OLD_RUNTIME=cf111db0d141e76787b76cc319f97fa6a4608de2868d4ba0d94381ba853fb2b8

TRUST_VERIFY="$SUPPLY/verify-production-trust-anchor.sh"
RELEASE_VERIFY="$SUPPLY/verify-release.sh"
TRUST_PUB="$SUPPLY/trust/release-ed25519.pub.pem"

LOG="${HASHBURST_RELEASE_LOG:-$REPO/mainnet-production-release.log}"

RELEASE_MODE="${HASHBURST_RELEASE_MODE:-canary}"
RECOVERY_TARGET="${HASHBURST_RECOVERY_TARGET:-}"

SSH=(
    ssh
    -o UserKnownHostsFile="$KNOWN"
    -o StrictHostKeyChecking=yes
    -o BatchMode=yes
)

SCP=(
    scp
    -q
    -o UserKnownHostsFile="$KNOWN"
    -o StrictHostKeyChecking=yes
    -o BatchMode=yes
)

remote() {
    local ip="$1"
    shift

    SSH_AUTH_SOCK="$SOCK" \
        "${SSH[@]}" "root@$ip" "$@"
}

copy() {
    local src="$1"
    local ip="$2"
    local dst="$3"

    SSH_AUTH_SOCK="$SOCK" \
        "${SCP[@]}" "$src" "root@$ip:$dst"
}

log() {
    printf '%s\n' "$*" | tee -a "$LOG"
}

fail() {
    log "HVM_PRODUCTION_RELEASE_FAIL: $*"
    exit 1
}

health_json() {
    local ip="$1"

    remote "$ip" \
        "curl -fsS --max-time 5 http://127.0.0.1:18019/health"
}

validate_health() {
    local expected_role="$1"
    local minimum_peers="$2"

    python3 -c '
import json
import sys

role = sys.argv[1]
minimum_peers = int(sys.argv[2])

d = json.load(sys.stdin)

assert d["ok"] is True
assert d["network"] == "mainnet"
assert d["chain_id"] == 4735489
assert d["config_digest"] == "'"$EXPECTED_CONFIG_DIGEST"'"
assert d["role"] == role

assert isinstance(d["height"], int)
assert isinstance(d["finalized_height"], int)
assert d["height"] >= d["finalized_height"]

assert isinstance(d["peer_count"], int)
assert d["peer_count"] >= minimum_peers

assert d["reactor_running"] is True
assert d["reactor_status_fresh"] is True

r = d["reactor"]

assert isinstance(r, dict)
assert r["running"] is True
assert isinstance(r["height"], int)
assert r["local_validator_id"]
' "$expected_role" "$minimum_peers"
}

validator_health() {
    local ip="$1"
    local minimum_peers="${2:-0}"

    health_json "$ip" |
        validate_health validator "$minimum_peers"
}

finalized_height() {
    local ip="$1"

    health_json "$ip" |
        python3 -c '
import json,sys
d=json.load(sys.stdin)
assert d["ok"] is True
print(d["finalized_height"])
'
}

consensus_state() {
    local ip="$1"

    health_json "$ip" |
        python3 -c '
import json
import sys

d = json.load(sys.stdin)

assert d["ok"] is True
assert d["network"] == "mainnet"
assert d["chain_id"] == 4735489
assert d["config_digest"] == "'"$EXPECTED_CONFIG_DIGEST"'"
assert d["reactor_running"] is True
assert d["reactor_status_fresh"] is True

r = d["reactor"]

assert isinstance(r, dict)
assert r["running"] is True
assert isinstance(d["finalized_height"], int)
assert isinstance(r["height"], int)
assert isinstance(r["round"], int)
assert r["round"] >= 0
assert r["local_validator_id"]

print(
    str(d["finalized_height"]) + " " +
    str(r["height"]) + " " +
    str(r["round"])
)
'
}

verify_testnet_sentinel() {
    local body

    body="$(
        remote 77.90.188.155 \
            "systemctl is-active --quiet hashburst-hvm-testnet.service;
             systemctl is-enabled --quiet hashburst-hvm-testnet.service;
             curl -fsS --max-time 5 http://127.0.0.1:18009/health"
    )"

    printf '%s\n' "$body" |
        tail -n 1 |
        python3 -c '
import json,sys
d=json.load(sys.stdin)

assert d["ok"] is True
assert d["network"] == "testnet"
assert d["chain_id"] == 4735490
assert d["role"] == "validator"
assert d["reactor_running"] is True
assert d["reactor_status_fresh"] is True
'

    log "HVM_R8U_TESTNET_PRESERVATION_OK"
}

verify_four_of_four() {
    local ip

    for ip in "${VALIDATORS[@]}"; do
        remote "$ip" \
            "systemctl is-active --quiet '$VSERVICE';
             systemctl is-enabled --quiet '$VSERVICE'"

        validator_health "$ip" 0
    done

    log "HVM_R8U_FEDERATION_RESTORED_4OF4_OK"
}

verify_survivor_finality() {
    local target="$1"
    local -a survivors=()
    local -a initial_heights=()
    local -a initial_rounds=()
    local -a current_heights=()
    local -a current_rounds=()
    local ip
    local state
    local finalized
    local reactor_height
    local reactor_round
    local before
    local after
    local attempt
    local index
    local advanced=0
    local reactor_advanced=0

    for ip in "${VALIDATORS[@]}"; do
        if [[ "$ip" != "$target" ]]; then
            survivors+=("$ip")
        fi
    done

    test "${#survivors[@]}" -eq 3

    #
    # All three surviving validators must remain active members of the
    # expected mainnet/config with live and fresh consensus reactors.
    #
    for ip in "${survivors[@]}"; do
        remote "$ip" \
            "systemctl is-active --quiet '$VSERVICE';
             systemctl is-enabled --quiet '$VSERVICE'"

        validator_health "$ip" 0
    done

    #
    # Capture an initial consensus state from all three survivors.
    # The deployment availability gate distinguishes block-production
    # progress from consensus/pacemaker progress: APoW may legitimately
    # leave finalized_height unchanged while the reactor remains live.
    #
    for index in "${!survivors[@]}"; do
        state="$(consensus_state "${survivors[$index]}")"
        read -r finalized reactor_height reactor_round <<<"$state"

        if (( index == 0 )); then
            before="$finalized"
        else
            test "$finalized" = "$before" || return 1
        fi

        initial_heights+=("$reactor_height")
        initial_rounds+=("$reactor_round")
    done

    test "${initial_heights[0]}" = "${initial_heights[1]}" || return 1
    test "${initial_heights[0]}" = "${initial_heights[2]}" || return 1

    after="$before"

    for attempt in $(seq 1 30); do
        sleep 2

        current_heights=()
        current_rounds=()
        reactor_advanced=1

        for index in "${!survivors[@]}"; do
            ip="${survivors[$index]}"

            validator_health "$ip" 0

            state="$(consensus_state "$ip")"
            read -r finalized reactor_height reactor_round <<<"$state"

            current_heights+=("$reactor_height")
            current_rounds+=("$reactor_round")

            if (( index == 0 )); then
                after="$finalized"
            else
                test "$finalized" = "$after" || return 1
            fi
        done

        #
        # A newly finalized block is the strongest liveness proof.
        #
        if (( after > before )); then
            advanced=1
            break
        fi

        #
        # Otherwise require coherent pacemaker progress from all three
        # survivors. They must remain on the same consensus height.
        #
        if test "${current_heights[0]}" = "${current_heights[1]}" \
            && test "${current_heights[0]}" = "${current_heights[2]}"
        then
            for index in "${!survivors[@]}"; do
                if (( current_heights[index] > initial_heights[index] )); then
                    continue
                fi

                if (( current_heights[index] == initial_heights[index] )) \
                    && (( current_rounds[index] > initial_rounds[index] ))
                then
                    continue
                fi

                reactor_advanced=0
                break
            done

            if (( reactor_advanced == 1 )); then
                advanced=1
                log "HVM_R8U_SURVIVOR_REACTOR_LIVENESS_OK target=$target consensus_height=${current_heights[0]} rounds=${current_rounds[*]}"
                break
            fi
        fi
    done

    if (( advanced != 1 )); then
        log "HVM_R8U_SURVIVOR_LIVENESS_NOT_PROVEN target=$target before=$before after=$after"
        return 1
    fi

    log "HVM_R8U_MIXED_FLEET_QUORUM_OK target=$target before=$before after=$after"
}

wait_target_rejoin() {
    local target="$1"
    local expected_runtime="$2"
    local attempt
    local installed_runtime

    for attempt in $(seq 1 120); do
        installed_runtime="$(
            remote "$target" \
                "sha256sum /opt/hashburst-hvm-mainnet/hashburst-mainnet | awk '{print \$1}'" \
                2>/dev/null || true
        )"

        if test "$installed_runtime" = "$expected_runtime" \
            && validator_health "$target" 0 >/dev/null 2>&1
        then
            log "HVM_R8U_TARGET_RESTORED_OK target=$target attempt=$attempt runtime_sha256=$installed_runtime"
            return 0
        fi

        sleep 2
    done

    return 1
}

validator_id() {
    case "$1" in
        77.90.188.153) printf '%s\n' validator-01 ;;
        77.90.188.154) printf '%s\n' validator-02 ;;
        77.90.188.155) printf '%s\n' validator-03 ;;
        77.90.188.157) printf '%s\n' validator-04 ;;
        *)
            echo "UNKNOWN_VALIDATOR_IP=$1" >&2
            return 1
            ;;
    esac
}

find_rollback_bundle() {
    local host="$1"
    local id
    local bundle

    id="$(validator_id "$host")"
    bundle="$ROLLBACK_ROOT/$id/rollback-bundle.tar"

    test -f "$bundle" || {
        echo "ROLLBACK_BUNDLE_MISSING host=$host path=$bundle" >&2
        return 1
    }

    printf '%s\n' "$bundle"
}

preflight_all_rollback_bundles() {
    local id
    local host
    local bundle
    local expected_bundle_sha
    local actual_bundle_sha
    local verified
    local tmp
    local prefix
    local runtime
    local runtime_sha

    test -r "$ROLLBACK_INDEX" || {
        echo "ROLLBACK_INDEX_NOT_READABLE=$ROLLBACK_INDEX" >&2
        return 1
    }

    for host in "${VALIDATORS[@]}"; do
        id="$(validator_id "$host")"
        bundle="$ROLLBACK_ROOT/$id/rollback-bundle.tar"

        test -r "$bundle" || {
            echo "ROLLBACK_BUNDLE_NOT_READABLE=$bundle" >&2
            return 1
        }

        readarray -t metadata < <(
            python3 - "$ROLLBACK_INDEX" "$id" "$host" <<'PYINDEX'
import json
import sys

index_path, validator_id, host = sys.argv[1:]

with open(index_path, "r", encoding="utf-8") as f:
    d = json.load(f)

matches = [
    v for v in d["validators"]
    if v.get("validator") == validator_id
    and v.get("ip") == host
]

if len(matches) != 1:
    raise SystemExit(1)

v = matches[0]

print(v["rollback_bundle_sha256"])
print(
    "true"
    if v.get("central_copy_verified") is True
    else "false"
)
PYINDEX
        )

        test "${#metadata[@]}" -eq 2 || return 1

        expected_bundle_sha="${metadata[0]}"
        verified="${metadata[1]}"

        test "$verified" = true || {
            echo "ROLLBACK_CENTRAL_COPY_NOT_VERIFIED=$id" >&2
            return 1
        }

        actual_bundle_sha="$(
            sha256sum "$bundle" |
                awk '{print $1}'
        )"

        test "$actual_bundle_sha" = "$expected_bundle_sha" || {
            echo "ROLLBACK_BUNDLE_SHA_MISMATCH=$id" >&2
            return 1
        }

        #
        # Fail closed on unsafe archive member names before extraction.
        #
        python3 - "$bundle" <<'PYTAR'
import sys
import tarfile
from pathlib import PurePosixPath

bundle = sys.argv[1]

with tarfile.open(bundle, "r:*") as tf:
    members = tf.getmembers()

if not members:
    raise SystemExit(1)

for m in members:
    p = PurePosixPath(m.name)

    if p.is_absolute() or ".." in p.parts:
        raise SystemExit(1)

    if m.issym() or m.islnk():
        raise SystemExit(1)
PYTAR

        #
        # Explicit tar inventory is part of the fail-closed preflight.
        #
        tar -tf "$bundle" >/dev/null

        tmp="$(mktemp -d)"
        chmod 0700 "$tmp"

        if ! tar -xf "$bundle" -C "$tmp"; then
            rm -rf -- "$tmp"
            return 1
        fi

        mapfile -t prefixes < <(
            find "$tmp" \
                -mindepth 1 \
                -maxdepth 1 \
                -type d \
                -printf '%f\n'
        )

        if test "${#prefixes[@]}" -ne 1; then
            rm -rf -- "$tmp"
            echo "ROLLBACK_ARCHIVE_PREFIX_INVALID=$id" >&2
            return 1
        fi

        prefix="${prefixes[0]}"

        test -s "$tmp/$prefix/SHA256SUMS" || {
            rm -rf -- "$tmp"
            echo "ROLLBACK_INTERNAL_SUMS_MISSING=$id" >&2
            return 1
        }

        if ! (
            cd "$tmp/$prefix"
            sha256sum -c SHA256SUMS >/dev/null
        ); then
            rm -rf -- "$tmp"
            echo "ROLLBACK_INTERNAL_SUMS_FAILED=$id" >&2
            return 1
        fi

        runtime="$tmp/$prefix/runtime/hashburst-mainnet"

        test -s "$runtime" || {
            rm -rf -- "$tmp"
            echo "ROLLBACK_RUNTIME_MISSING=$id" >&2
            return 1
        }

        runtime_sha="$(
            sha256sum "$runtime" |
                awk '{print $1}'
        )"

        test "$runtime_sha" = "$EXPECTED_OLD_RUNTIME" || {
            rm -rf -- "$tmp"
            echo "ROLLBACK_RUNTIME_SHA_MISMATCH=$id" >&2
            return 1
        }

        rm -rf -- "$tmp"

        log "HVM_R8U_ROLLBACK_PREFLIGHT_OK validator=$id host=$host bundle_sha256=$actual_bundle_sha runtime_sha256=$runtime_sha"
    done

    log "HVM_R8U_ROLLBACK_PREFLIGHT_4OF4_OK"
}

rollback_target() {
    local host="$1"
    local bundle="$2"
    local id
    local expected_bundle_sha
    local actual_bundle_sha
    local tmp
    local prefix
    local runtime
    local runtime_sha
    local remote_tmp

    id="$(validator_id "$host")"

    expected_bundle_sha="$(
        python3 - "$ROLLBACK_INDEX" "$id" "$host" <<'PYROLLBACKINDEX'
import json
import sys

index_path, validator_id, host = sys.argv[1:]

with open(index_path, "r", encoding="utf-8") as f:
    d = json.load(f)

matches = [
    v for v in d["validators"]
    if v["validator"] == validator_id and v["ip"] == host
]

assert len(matches) == 1
assert matches[0]["central_copy_verified"] is True

print(matches[0]["rollback_bundle_sha256"])
PYROLLBACKINDEX
    )"

    actual_bundle_sha="$(sha256sum "$bundle" | awk '{print $1}')"

    test "$actual_bundle_sha" = "$expected_bundle_sha" || {
        echo "ROLLBACK_BUNDLE_SHA_MISMATCH host=$host" >&2
        return 1
    }

    tmp="$(mktemp -d)"

    if ! tar -xf "$bundle" -C "$tmp"; then
        rm -rf "$tmp"
        return 1
    fi

    mapfile -t prefixes < <(
        find "$tmp" \
            -mindepth 1 \
            -maxdepth 1 \
            -type d \
            -printf '%f\n'
    )

    test "${#prefixes[@]}" -eq 1 || {
        rm -rf "$tmp"
        echo "ROLLBACK_ARCHIVE_PREFIX_INVALID host=$host" >&2
        return 1
    }

    prefix="${prefixes[0]}"
    runtime="$tmp/$prefix/runtime/hashburst-mainnet"

    test -f "$runtime" || {
        rm -rf "$tmp"
        echo "ROLLBACK_RUNTIME_MISSING host=$host" >&2
        return 1
    }

    runtime_sha="$(sha256sum "$runtime" | awk '{print $1}')"

    test "$runtime_sha" = "$EXPECTED_OLD_RUNTIME" || {
        rm -rf "$tmp"
        echo "ROLLBACK_RUNTIME_SHA_MISMATCH host=$host" >&2
        return 1
    }

    remote_tmp="/opt/hashburst-hvm-mainnet/.hashburst-mainnet.rollback.$$"

    echo "HVM_R8U_VALIDATOR_ROLLBACK host=$host"

    remote "$host" "systemctl stop '$VSERVICE'"

    copy "$runtime" "$host" "$remote_tmp"

    remote "$host" "
        set -euo pipefail

        test \"\$(sha256sum '$remote_tmp' | awk '{print \$1}')\" = \
            '$EXPECTED_OLD_RUNTIME'

        install -o root -g root -m 0755 \
            '$remote_tmp' \
            /opt/hashburst-hvm-mainnet/hashburst-mainnet.rollback

        rm -f '$remote_tmp'

        mv -f \
            /opt/hashburst-hvm-mainnet/hashburst-mainnet.rollback \
            /opt/hashburst-hvm-mainnet/hashburst-mainnet

        systemctl start '$VSERVICE'

        test \"\$(systemctl is-active '$VSERVICE')\" = active
    "

    rm -rf "$tmp"

    wait_target_rejoin "$host" "$EXPECTED_OLD_RUNTIME"

    echo "HVM_R8U_TARGET_RESTORED_OK host=$host"
}

upgrade_validator() {
    local target="$1"
    local rollback_bundle="$2"

    log "HVM_R8U_ROLLING_VALIDATOR_UPGRADE target=$target"

    test -s "$rollback_bundle" ||
        fail "sealed rollback image missing for $target"

    #
    # Baseline must be healthy before removing one validator.
    #
    verify_four_of_four
    verify_testnet_sentinel

    #
    # Stage signed runtime before stopping the target.
    #
    copy "$SIGNED_RUNTIME" "$target" /root/hashburst-mainnet.r8u.new

    remote "$target" \
        "set -euo pipefail;
         test \"\$(sha256sum /root/hashburst-mainnet.r8u.new | awk '{print \$1}')\" = '$SIGNED_RUNTIME_SHA';
         chmod 0700 /root/hashburst-mainnet.r8u.new"

    #
    # Remove exactly one validator.
    #
    remote "$target" "systemctl stop '$VSERVICE'"

    #
    # Prove the remaining 3/4 federation is still finalizing before
    # replacing the target runtime.
    #
    if ! verify_survivor_finality "$target"; then
        rollback_target "$target" "$rollback_bundle"
        fail "surviving federation verification failed; target restored: $target"
    fi

    if ! remote "$target" \
        "set -euo pipefail;
         install -o root -g root -m 0755 \
             /root/hashburst-mainnet.r8u.new \
             /opt/hashburst-hvm-mainnet/hashburst-mainnet;
         rm -f /root/hashburst-mainnet.r8u.new;
         test \"\$(sha256sum /opt/hashburst-hvm-mainnet/hashburst-mainnet | awk '{print \$1}')\" = '$SIGNED_RUNTIME_SHA';
         systemctl start '$VSERVICE'"; then

        rollback_target "$target" "$rollback_bundle"
    fi

    if ! wait_target_rejoin "$target" "$SIGNED_RUNTIME_SHA"; then
        rollback_target "$target" "$rollback_bundle"
    fi

    remote "$target" \
        "test \"\$(sha256sum /opt/hashburst-hvm-mainnet/hashburst-mainnet | awk '{print \$1}')\" = '$SIGNED_RUNTIME_SHA'"

    verify_four_of_four
    verify_testnet_sentinel

    log "HVM_R8U_SIGNED_RUNTIME_BOUND target=$target sha256=$SIGNED_RUNTIME_SHA"
}

echo "============================================================"
echo "=== HASHBURST R8U SIGNED ROLLING PRODUCTION RELEASE ==="
date -u '+UTC=%Y-%m-%dT%H:%M:%SZ'
hostname
echo "============================================================"

#
# Local execution boundary.
#
test "$(hostname -I | tr ' ' '\n' | grep -Fx '77.90.188.180')" = 77.90.188.180
test -S "$SOCK"
test -r "$KNOWN"

test -d "$PACKAGE"
test -d "$ROLLBACK_ROOT"

#
# G5 trust and package verification MUST happen before the first
# fleet mutation.
#
bash "$TRUST_VERIFY"

bash "$RELEASE_VERIFY" \
    "$PACKAGE" \
    "$TRUST_PUB"

ARTIFACT_SOURCE_COMMIT="$(
    tr -d '\r\n' < "$PACKAGE/SOURCE_COMMIT"
)"

test "$ARTIFACT_SOURCE_COMMIT" = "$EXPECTED_ARTIFACT_SOURCE_COMMIT"

PRODUCTION_SIGNATURE_SHA256="$(
    sha256sum "$PACKAGE/SHA256SUMS.sig" |
    awk '{print $1}'
)"

test "$PRODUCTION_SIGNATURE_SHA256" = \
    "$EXPECTED_PRODUCTION_SIGNATURE_SHA256"

SIGNED_RUNTIME_SHA="$(
    awk '$2=="hashburst-mainnet"{print $1}' \
        "$PACKAGE/SHA256SUMS"
)"

SIGNED_GATEWAY_SHA="$(
    awk '$2=="hvm-mainnet-evm-gateway"{print $1}' \
        "$PACKAGE/SHA256SUMS"
)"

test -n "$SIGNED_RUNTIME_SHA"
test -n "$SIGNED_GATEWAY_SHA"

SIGNED_RUNTIME="$PACKAGE/hashburst-mainnet"
SIGNED_GATEWAY="$PACKAGE/hvm-mainnet-evm-gateway"

test "$(
    sha256sum "$SIGNED_RUNTIME" |
    awk '{print $1}'
)" = "$SIGNED_RUNTIME_SHA"

test "$(
    sha256sum "$SIGNED_GATEWAY" |
    awk '{print $1}'
)" = "$SIGNED_GATEWAY_SHA"

ORCHESTRATOR_COMMIT="${HASHBURST_ORCHESTRATOR_COMMIT:-}"

test -n "$ORCHESTRATOR_COMMIT"
test "${#ORCHESTRATOR_COMMIT}" -eq 40
case "$ORCHESTRATOR_COMMIT" in
    *[!0-9a-f]*)
        fail "invalid HASHBURST_ORCHESTRATOR_COMMIT"
        ;;
esac

log "ARTIFACT_SOURCE_COMMIT=$ARTIFACT_SOURCE_COMMIT"
log "ORCHESTRATOR_COMMIT=$ORCHESTRATOR_COMMIT"
log "PRODUCTION_SIGNATURE_SHA256=$PRODUCTION_SIGNATURE_SHA256"

log "HVM_R8U_SIGNED_RUNTIME_BOUND sha256=$SIGNED_RUNTIME_SHA"
log "HVM_R8U_SIGNED_GATEWAY_BOUND sha256=$SIGNED_GATEWAY_SHA"

#
# No founder or validator signing key is read or exported by this
# rolling runtime upgrade.
#
log "FOUNDER_PRIVATE_KEY_NOT_USED"
log "VALIDATOR_PRIVATE_KEYS_NOT_EXPORTED"

#
# A3-R5 / rollback-only recovery authority boundary.
#
case "$RELEASE_MODE" in
    rollback-only)
        test "$RECOVERY_TARGET" = 77.90.188.153 ||
            fail "rollback-only target must be 77.90.188.153"

        RECOVERY_TARGET=77.90.188.153
        bundle="$(find_rollback_bundle "$RECOVERY_TARGET")"

        test -n "$bundle" ||
            fail "cannot bind rollback image for $RECOVERY_TARGET"

        preflight_all_rollback_bundles
        log "HVM_R8U_ROLLBACK_PREFLIGHT_GATE_PASSED"

        verify_testnet_sentinel
        verify_survivor_finality "$RECOVERY_TARGET"

        log "HVM_R8U_ROLLBACK_ONLY_TARGET_BOUND_OK target=$RECOVERY_TARGET"

        rollback_target "$RECOVERY_TARGET" "$bundle"

        verify_four_of_four
        verify_testnet_sentinel

        log "HVM_A3_R5_VALIDATOR153_RECOVERY_OK target=$RECOVERY_TARGET runtime_sha256=$EXPECTED_OLD_RUNTIME"
        echo "HVM_R8U_ROLLBACK_ONLY_COMPLETE"
        exit 0
        ;;
    canary)
        test -z "$RECOVERY_TARGET" ||
            fail "HASHBURST_RECOVERY_TARGET is invalid in canary mode"
        ;;
    *)
        fail "invalid HASHBURST_RELEASE_MODE"
        ;;
esac

#
# Initial federation and testnet preservation gate for forward canary only.
#
verify_four_of_four
verify_testnet_sentinel

#
# C / canary authority boundary.
# Validate all four rollback images before the first mutation, then upgrade
# ONLY validator-01 (.153). A separate reviewed gate is required before any
# additional production validator may be mutated.
#
preflight_all_rollback_bundles
log "HVM_R8U_ROLLBACK_PREFLIGHT_GATE_PASSED"

CANARY_TARGET=77.90.188.153
bundle="$(find_rollback_bundle "$CANARY_TARGET")"

test -n "$bundle" ||
    fail "cannot bind rollback image for $CANARY_TARGET"

upgrade_validator "$CANARY_TARGET" "$bundle"

#
# Canary acceptance is complete only after upgrade_validator() has already
# proved survivor liveness, target rejoin, federation 4/4 and testnet
# preservation. Stop here deliberately: .154/.155/.157 remain untouched.
#
log "HVM_R8U_SIGNED_RUNTIME_1OF4_OK target=$CANARY_TARGET sha256=$SIGNED_RUNTIME_SHA"
log "HVM_R8U_CANARY_MANDATORY_STOP_OK remaining=77.90.188.154,77.90.188.155,77.90.188.157"

echo "============================================================"
echo "HVM_R8U_CANARY_1OF4_COMPLETE"
echo "============================================================"
