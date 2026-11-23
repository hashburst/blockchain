#!/usr/bin/env python3

from pathlib import Path

here = Path(__file__).resolve().parent
g = (here / "run-production-release.sh").read_text()

#
# G5 production package / trust boundary.
#
required = {
    "production package":
        "production-signed-release",

    "production trust verifier":
        "verify-production-trust-anchor.sh",

    "release package verifier":
        "verify-release.sh",

    "production trust public key":
        "trust/release-ed25519.pub.pem",

    "signed checksum metadata":
        "SHA256SUMS",

    "signed runtime binding":
        "HVM_R8U_SIGNED_RUNTIME_BOUND",

    "signed gateway binding":
        "HVM_R8U_SIGNED_GATEWAY_BOUND",

    #
    # Dual provenance.
    #
    "artifact source commit":
        "ARTIFACT_SOURCE_COMMIT",

    "orchestrator commit":
        "ORCHESTRATOR_COMMIT",

    "production signature digest":
        "PRODUCTION_SIGNATURE_SHA256",

    #
    # Rolling federation safety.
    #
    "rolling upgrade":
        "HVM_R8U_ROLLING_VALIDATOR_UPGRADE",

    "surviving quorum":
        "HVM_R8U_MIXED_FLEET_QUORUM_OK",

    "target restored":
        "HVM_R8U_TARGET_RESTORED_OK",

    "federation restored":
        "HVM_R8U_FEDERATION_RESTORED_4OF4_OK",

    #
    # Per-validator rollback.
    #
    "target rollback":
        "HVM_R8U_VALIDATOR_ROLLBACK",

    "rollback bundle":
        "ROLLBACK_ROOT",

    #
    # Preservation boundaries.
    #
    "testnet preservation":
        "HVM_R8U_TESTNET_PRESERVATION_OK",

    "founder boundary":
        "FOUNDER_PRIVATE_KEY_NOT_USED",

    "validator key boundary":
        "VALIDATOR_PRIVATE_KEYS_NOT_EXPORTED",
}

for name, needle in required.items():
    assert needle in g, (
        f"missing R8U production-upgrade contract: "
        f"{name}: {needle}"
    )

#
# These legacy bootstrap contracts are forbidden in the rolling-upgrade
# orchestrator.
#
forbidden = {
    "runtime copied from validator-01":
        "root@77.90.188.153:/opt/hashburst-hvm-mainnet/hashburst-mainnet",

    "validator-at-rest bootstrap requirement":
        "HVM_R8B_VALIDATOR_AT_REST_AND_TESTNET_OBSERVER_PRESERVED_OK",

    "stale Go 1.25.7 requirement":
        'test "$(go env GOVERSION)" = go1.25.7',

    "ad-hoc production gateway build":
        'go build -trimpath -o "$REPO/hvm-mainnet-evm-gateway"',

    "global validator shutdown rollback":
        'for ip in "${VALIDATORS[@]}";do remote "$ip" "systemctl disable --now',
}

for name, needle in forbidden.items():
    assert needle not in g, (
        f"forbidden legacy production contract remains: "
        f"{name}: {needle}"
    )

#
# The package must be verified before the first fleet mutation.
#
trust = g.index("verify-production-trust-anchor.sh")
verify = g.index("verify-release.sh")

mutation_candidates = [
    x for x in (
        g.find("systemctl stop"),
        g.find("systemctl restart"),
        g.find("systemctl disable"),
        g.find("systemctl enable"),
    )
    if x >= 0
]

assert mutation_candidates, "no controlled service mutation found"

first_mutation = min(mutation_candidates)

assert trust < first_mutation, (
    "production trust anchor verification occurs after fleet mutation"
)

assert verify < first_mutation, (
    "signed package verification occurs after fleet mutation"
)

#
# Runtime authority must derive from signed metadata, never from a live node.
#
assert "hashburst-mainnet" in g
assert "SHA256SUMS" in g

#
# Testnet is an independent preservation boundary.
#
assert "4735490" in g

print("HB_R8U_2A_R3_SIGNED_ROLLING_RELEASE_REGRESSION_OK")

#
# R8U-2A-R3B-3 semantic regression.
#
# Do not accept verifier filenames in variable assignments as proof
# that verification actually executes before the rolling mutation.
#
trust_call = g.index('bash "$TRUST_VERIFY"')
release_call = g.index('bash "$RELEASE_VERIFY"')
rolling_call = g.index('upgrade_validator "$target" "$bundle"')

assert trust_call < rolling_call, (
    "production trust verifier is not CALLED before rolling mutation"
)

assert release_call < rolling_call, (
    "production package verifier is not CALLED before rolling mutation"
)

#
# Exact forensic bundle layout discovered in R8U-2A-R3B-2R.
#
assert '$ROLLBACK_ROOT/$id/rollback-bundle.tar' in g, (
    "rollback bundle does not use exact sealed forensic layout"
)

#
# Ordinary rollback must restore only the binary changed by the
# forward upgrade. Point-in-time ledger/consensus/recovery state
# belongs to disaster recovery and must not be copied automatically.
#
rb_start = g.index("rollback_target()")
rb_end = g.index("upgrade_validator()", rb_start)
rb = g[rb_start:rb_end]

assert "runtime/hashburst-mainnet" in rb
assert "EXPECTED_OLD_RUNTIME" in rb
assert "rollback_bundle_sha256" in rb
assert "central_copy_verified" in rb

for forbidden_state_restore in (
    "state/blockchain.dat",
    "state/blockchain.idx",
    "state/runtime.pin",
    "state/consensus-votes.jsonl",
    "state/consensus-bft-signatures.jsonl",
    "state/consensus-recovery.json",
    "state/recovery-checkpoint.key",
):
    assert forbidden_state_restore not in rb, (
        "ordinary rollback attempts point-in-time state restore: "
        + forbidden_state_restore
    )

print("HB_R8U_2A_R3B3_SEMANTIC_ROLLBACK_REGRESSION_OK")

#
# R8U-2A-R3B-4R executable symbol contract.
#
# Static marker presence is insufficient: every helper called by the
# production rollback path must actually have exactly one definition.
#
import re

required_function_definitions = (
    "remote",
    "copy",
    "validator_id",
    "find_rollback_bundle",
    "rollback_target",
    "upgrade_validator",
    "wait_target_rejoin",
)

for fn in required_function_definitions:
    pattern = re.compile(
        rf'(?m)^[ \t]*{re.escape(fn)}\(\)[ \t]*\{{'
    )
    count = len(pattern.findall(g))
    assert count == 1, (
        f"required executable function definition invalid: "
        f"{fn}: definitions={count}"
    )

required_variable_definitions = (
    "ROLLBACK_ROOT",
    "ROLLBACK_INDEX",
    "EXPECTED_OLD_RUNTIME",
)

for var in required_variable_definitions:
    pattern = re.compile(
        rf'(?m)^[ \t]*(?:export[ \t]+)?'
        rf'{re.escape(var)}='
    )
    count = len(pattern.findall(g))
    assert count == 1, (
        f"required executable variable definition invalid: "
        f"{var}: definitions={count}"
    )

#
# Use the orchestrator's real transport/rejoin interfaces.
#
rb_start = g.index("rollback_target()")
rb_end = g.index("upgrade_validator()", rb_start)
rb = g[rb_start:rb_end]

assert 'copy "$runtime" "$host" "$remote_tmp"' in rb, (
    "rollback does not use canonical copy() transport helper"
)

assert 'wait_target_rejoin "$host" "$EXPECTED_OLD_RUNTIME"' in rb, (
    "rollback does not use canonical wait_target_rejoin() helper"
)

assert "scp_to " not in rb, (
    "undefined scp_to helper remains in rollback path"
)

assert "wait_target_health " not in rb, (
    "undefined wait_target_health helper remains in rollback path"
)

print("HB_R8U_2A_R3B4R_EXECUTABLE_SYMBOL_REGRESSION_OK")

#
# R8U-2A-R3B-4F-R1:
# successful rejoin must bind both health and the expected runtime SHA.
#
rejoin_start = g.index("wait_target_rejoin()")
rejoin_end = g.index("\n}", rejoin_start) + 2
rejoin = g[rejoin_start:rejoin_end]

assert 'local target="$1"' in rejoin, (
    "wait_target_rejoin must bind target from argument 1"
)

assert 'local expected_runtime="$2"' in rejoin, (
    "wait_target_rejoin must bind expected runtime from argument 2"
)

assert "validator_health" in rejoin, (
    "wait_target_rejoin must require validator health"
)

assert "sha256sum" in rejoin, (
    "wait_target_rejoin must verify installed runtime SHA"
)

assert "expected_runtime" in rejoin

print("HB_R8U_2A_R3B4FR1_REJOIN_RUNTIME_REGRESSION_OK")

#
# R8U-2A-R3B-4F-R2R:
# exact forward authority is SIGNED_RUNTIME_SHA, derived from the
# hashburst-mainnet entry of the verified package SHA256SUMS.
#
assert 'SIGNED_RUNTIME_SHA="$(' in g
assert '$2=="hashburst-mainnet"' in g
assert '"$PACKAGE/SHA256SUMS"' in g
assert 'SIGNED_RUNTIME="$PACKAGE/hashburst-mainnet"' in g

forward_start = g.index("upgrade_validator()")
forward_end = g.index("\n}", forward_start) + 2
forward = g[forward_start:forward_end]

assert (
    'wait_target_rejoin "$target" "$SIGNED_RUNTIME_SHA"'
    in forward
), "forward rejoin is not bound to SIGNED_RUNTIME_SHA"

print("HB_R8U_2A_R3B4FR2R_EXACT_SIGNED_RUNTIME_REGRESSION_OK")

#
# R8U-2A-R3B-4F-R3 / REQ22:
# all four sealed rollback bundles must be validated locally before
# the first production validator mutation.
#
assert "preflight_all_rollback_bundles() {" in g, (
    "missing global rollback preflight"
)

preflight_start = g.index("preflight_all_rollback_bundles()")
preflight_end = g.index("\n}", preflight_start) + 2
preflight = g[preflight_start:preflight_end]

assert (
    'ROLLBACK_INDEX="$ROLLBACK_ROOT/FORENSIC-INDEX.json"'
    in g
), "global rollback index is not bound to FORENSIC-INDEX.json"

for required in (
    "$ROLLBACK_INDEX",
    "central_copy_verified",
    "rollback_bundle_sha256",
    "sha256sum",
    "tar -tf",
    "SHA256SUMS",
    "runtime/hashburst-mainnet",
    "EXPECTED_OLD_RUNTIME",
):
    assert required in preflight, (
        "global rollback preflight missing: " + required
    )

preflight_call = g.index("preflight_all_rollback_bundles")
rolling_call = g.index(
    'upgrade_validator "$target" "$bundle"'
)

assert preflight_call < rolling_call, (
    "global rollback preflight must precede rolling mutation"
)

print("HB_R8U_2A_R3B4FR3_GLOBAL_ROLLBACK_PREFLIGHT_REGRESSION_OK")

# Trusted production execution is a root-owned exported tree without .git.
# Provenance must therefore be supplied explicitly by the privileged
# execution capability rather than inferred from repository metadata.
assert 'ORCHESTRATOR_COMMIT="${HASHBURST_ORCHESTRATOR_COMMIT:-}"' in g, (
    "orchestrator provenance is not bound to privileged explicit input"
)
assert 'git -C "$REPO" rev-parse HEAD' not in g, (
    "trusted production execution still depends on .git metadata"
)
assert 'test "${#ORCHESTRATOR_COMMIT}" -eq 40' in g, (
    "orchestrator commit provenance is not length validated"
)
assert '*[!0-9a-f]*)' in g, (
    "orchestrator commit provenance is not lowercase-hex validated"
)

print("HB_R8U_2A_R3B4FR4_TRUSTED_PROVENANCE_REGRESSION_OK")


#
# R8U-2A-R3B-4F-R5:
# validate_health() consumes sys.argv[1] and sys.argv[2].
# expected_role and minimum_peers must therefore be supplied explicitly
# after the python3 -c program.
#
validate_start = g.index("validate_health() {")
validate_end = g.index(
    "\n}\n\nvalidator_health()",
    validate_start,
)
validate = g[validate_start:validate_end]

assert 'local expected_role="$1"' in validate
assert 'local minimum_peers="$2"' in validate

assert "role = sys.argv[1]" in validate
assert "minimum_peers = int(sys.argv[2])" in validate

argv_contract = chr(39) + ' "$expected_role" "$minimum_peers"'

assert argv_contract in validate, (
    "validate_health Python argv contract broken: "
    "expected_role and minimum_peers are not passed to python3 -c"
)

assert validate.count(argv_contract) == 1, (
    "validate_health Python argv contract must occur exactly once"
)

print(
    "HB_R8U_2A_R3B4FR5_VALIDATE_HEALTH_ARGV_REGRESSION_OK"
)

#
# R8U-R7B / survivor-finality failure recovery contract.
#
# Once the target validator has been stopped, failure of the surviving
# 3-of-4 finality proof is a post-mutation failure. It MUST therefore
# restore the stopped target from its sealed rollback bundle before
# terminating the production release.
#
# This regression intentionally fails on the pre-remediation source.
#
forward_start = g.index("upgrade_validator() {")
forward_end = g.index("\n}\n", forward_start) + 2
forward = g[forward_start:forward_end]

stop_call = 'remote "$target" "systemctl stop \'$VSERVICE\'"'

survivor_guard = '''if ! verify_survivor_finality "$target"; then
        rollback_target "$target" "$rollback_bundle"
        fail "surviving federation verification failed; target restored: $target"
    fi'''

signed_install = '''install -o root -g root -m 0755 \\
             /root/hashburst-mainnet.r8u.new \\
             /opt/hashburst-hvm-mainnet/hashburst-mainnet'''

assert stop_call in forward, (
    "rolling upgrade no longer exposes the expected single-target stop"
)

assert survivor_guard in forward, (
    "survivor-finality failure after target stop is not rollback protected"
)

stop_pos = forward.index(stop_call)
guard_pos = forward.index(survivor_guard)
rollback_pos = forward.index(
    'rollback_target "$target" "$rollback_bundle"',
    guard_pos,
)
fail_pos = forward.index(
    'fail "surviving federation verification failed; '
    'target restored: $target"',
    guard_pos,
)
install_pos = forward.index(signed_install)

assert stop_pos < guard_pos, (
    "survivor-finality guard must execute after target stop"
)

assert guard_pos < rollback_pos < fail_pos, (
    "survivor-finality failure must rollback before fail-closed exit"
)

assert fail_pos < install_pos, (
    "survivor-finality failure path must precede signed runtime install"
)

guard_end = forward.index("\n    fi", guard_pos) + len("\n    fi")
guard_block = forward[guard_pos:guard_end]

assert guard_block.count(
    'rollback_target "$target" "$rollback_bundle"'
) == 1, (
    "survivor-finality failure path must restore target exactly once"
)

assert "SIGNED_RUNTIME_SHA" not in guard_block, (
    "survivor-finality failure path must not bind/install signed runtime"
)

assert "/root/hashburst-mainnet.r8u.new" not in guard_block, (
    "survivor-finality failure path must not perform forward installation"
)

print(
    "HB_R8U_R7B_SURVIVOR_FINALITY_ROLLBACK_REGRESSION_OK"
)


# R8U-R8E / APoW-aware survivor liveness contract.
#
# Production incident evidence:
# - four-validator federation already had finalized height 2430 before rolling;
# - after one target was stopped, the three survivors remained on the same
#   chain/config/consensus height and advanced consensus rounds;
# - production consensus may legitimately be unable to build a proposal while
#   APoW work is unavailable.
#
# Therefore the deployment availability gate must not define survivor
# availability solely as finalized_height advancing.
#
# This is a deployment-layer regression contract. It must not weaken quorum,
# alter proposer selection, bypass APoW, or modify consensus code.

r8e_source = g

r8e_verify_start = r8e_source.find("verify_survivor_finality()")
assert r8e_verify_start >= 0, "survivor verification function missing"

r8e_verify_end = r8e_source.find("\n}", r8e_verify_start)
assert r8e_verify_end > r8e_verify_start, (
    "survivor verification function boundary missing"
)

r8e_verify = r8e_source[r8e_verify_start:r8e_verify_end + 2]

# RED contract 1:
# finalized_height advancement cannot remain the sole success criterion.
assert (
    "reactor" in r8e_verify
    and "round" in r8e_verify
), (
    "survivor gate is finality-only and does not verify reactor/round liveness"
)

# RED contract 2:
# survivor verification must establish expected chain/config through
# the health/consensus helpers it actually invokes. Do not require
# duplicated literal validation inside verify_survivor_finality().
assert (
    "validator_health" in r8e_verify
    and "consensus_state" in r8e_verify
    and 'd["chain_id"] == 4735489' in r8e_source
    and 'd["config_digest"]' in r8e_source
), (
    "survivor gate does not bind liveness to expected chain/config"
)

# RED contract 3:
# the gate must observe consensus progress independently of block production.
assert "consensus_state" in r8e_verify, (
    "survivor gate has no consensus-liveness observation"
)

assert "reactor_advanced" in r8e_verify, (
    "survivor gate does not prove reactor progress"
)

assert "current_rounds" in r8e_verify, (
    "survivor gate does not observe consensus round progression"
)

assert (
    "after > before" in r8e_verify
    and "reactor_advanced == 1" in r8e_verify
), (
    "survivor gate does not accept finality-or-reactor liveness"
)

# Preserve the R7B fail-closed rollback ordering.
r8e_stop = r8e_source.find('remote "$target" "systemctl stop')
assert r8e_stop >= 0, "target stop site missing"

r8e_install = r8e_source.find(
    'install -m 0755 "$SIGNED_RUNTIME"',
    r8e_stop,
)

r8e_rollback = r8e_source.find(
    'rollback_target "$target" "$rollback_bundle"',
    r8e_stop,
)

assert r8e_rollback > r8e_stop, (
    "survivor failure is not rollback protected"
)

if r8e_install >= 0:
    assert r8e_rollback < r8e_install, (
        "rollback protection occurs after forward runtime installation"
    )

# Rollback success must remain operationally verified.
r8e_rb_start = r8e_source.find("rollback_target()")
assert r8e_rb_start >= 0, "rollback_target function missing"

r8e_rb_end = r8e_source.find("\n}", r8e_rb_start)
assert r8e_rb_end > r8e_rb_start, "rollback_target boundary missing"

r8e_rb = r8e_source[r8e_rb_start:r8e_rb_end + 2]

assert "systemctl start" in r8e_rb, (
    "rollback does not restart restored validator"
)

assert (
    "wait_target_rejoin" in r8e_rb
    or "validate_health" in r8e_rb
), (
    "rollback does not verify operational restoration"
)

print("HB_R8U_R8E_APOW_AWARE_SURVIVOR_LIVENESS_REGRESSION_OK")


# R8U-R8E-F2 / rollback reachability contract.
#
# A survivor verification function used as:
#
#     if ! verify_survivor_*; then
#         rollback_target ...
#     fi
#
# MUST report verification failure with a non-zero return.
# It MUST NOT terminate the orchestrator itself via fail()/exit,
# otherwise rollback_target is unreachable after target mutation.

r8e_f2_source = g

r8e_f2_verify_start = r8e_f2_source.find("verify_survivor_finality()")
assert r8e_f2_verify_start >= 0, (
    "survivor verification function missing"
)

r8e_f2_verify_end = r8e_f2_source.find(
    "\n}",
    r8e_f2_verify_start,
)
assert r8e_f2_verify_end > r8e_f2_verify_start, (
    "survivor verification function boundary missing"
)

r8e_f2_verify = r8e_f2_source[
    r8e_f2_verify_start:r8e_f2_verify_end + 2
]

assert (
    'fail "surviving 3-of-4 federation did not advance finality' not in
    r8e_f2_verify
), (
    "survivor predicate terminates orchestrator before rollback is reachable"
)

assert (
    "return 1" in r8e_f2_verify
), (
    "survivor predicate does not return failure to rollback caller"
)

print(
    "HB_R8U_R8E_F2_ROLLBACK_REACHABILITY_REGRESSION_OK"
)
