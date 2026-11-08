# HVM-R6 founder offline signing and signed checkpoint gate

HVM-R6 consumes a founder authorization without ever placing the founder private
key on GitHub, the control runner, a validator, an observer, an API or a dashboard.
The private key remains on the founder's offline signing computer.

This procedure does not authorize production deployment. The resulting validator
checkpoint retains `activation_allowed=false` and
`runtime_acceptance_required=true`. The `deploy` and `acceptance` workflow
phases remain fail-closed.

## 1. Public signing package

Use the public files produced by the accepted `phase=checkpoint` run:

- `mainnet-funding-plan.json`
- `mainnet-founder-signing-protocol.json`
- the four `mainnet-founder-*.public.json` validator enrollments
- `mainnet-founder-signing-SHA256SUMS.log`

Transfer only these public files and the reviewed source/binary needed to run
`hvm-mainnet-identity` to the offline signing computer. Verify the published
SHA256 values before signing.

The funding plan is advisory evidence. `sign-funding` independently recomputes
the deterministic plan from the frozen protocol and four enrollments. It does
not sign the JSON file as an opaque blob.

## 2. Sign only on the founder computer

The existing founder key must be the canonical owner-only raw `wallet.key`.
Do not copy it into the signing package.

Example, using local filenames:

```
./hvm-mainnet-identity sign-funding \
  --protocol mainnet-founder-signing-protocol.json \
  --key "$FOUNDER_KEY_FILE" \
  mainnet-founder-77.90.188.153.public.json \
  mainnet-founder-77.90.188.154.public.json \
  mainnet-founder-77.90.188.155.public.json \
  mainnet-founder-77.90.188.157.public.json \
  > founder-transfers.json
```

Immediately verify on the offline computer:

```
./hvm-mainnet-identity verify-funding \
  --protocol mainnet-founder-signing-protocol.json \
  --signed-funding founder-transfers.json \
  mainnet-founder-77.90.188.153.public.json \
  mainnet-founder-77.90.188.154.public.json \
  mainnet-founder-77.90.188.155.public.json \
  mainnet-founder-77.90.188.157.public.json
sha256sum founder-transfers.json
```

The required verifier marker is:

```
FUNDING_SIGNATURES_AND_PLAN_VERIFIED_NO_STATE_CHANGED
```

Record the SHA256 separately. `founder-transfers.json` is public signed data;
the private key and its contents never leave the offline computer.

## 3. Stage the public signed JSON on the control runner

Create the fixed evidence directory on 77.90.188.180 and copy only the public
signed JSON into it:

```
/var/lib/hashburst-actions/mainnet-evidence/founder-signed-funding/founder-transfers.json
```

The gate refuses a symlink, a non-canonical path, an empty/oversized file, and a
group/world-writable file. Compare its SHA256 with the value recorded on the
offline computer before dispatch.

Do not pass the signed JSON through a GitHub secret, environment variable, URL,
or private-key-bearing archive. The workflow reads only the fixed local evidence
path above.

## 4. Controlled HVM-R6 dispatch

After the reviewed HVM-R6 implementation is merged, dispatch:

```
phase=founder-checkpoint
```

The gate performs, in order:

1. coordinator and frozen protocol/economics checks;
2. a fresh R5 economic checkpoint preparation from the pinned legacy evidence
   and reviewed re-attestation;
3. collection and verification of the four current public validator identities;
4. `verify-funding` against the staged signed JSON;
5. creation of a new per-run output under
   `/var/lib/hashburst-actions/mainnet-evidence/founder-signed-checkpoints/run-$GITHUB_RUN_ID`;
6. offline `assemble` using the freshly prepared economic ledger;
7. replay/economic/validator reconciliation performed by the Go assembler;
8. explicit policy checks that the validator checkpoint is not activatable and
   still requires runtime acceptance;
9. public evidence copies and SHA256 logs for the GitHub artifact.

The successful terminal markers are:

```
FUNDING_SIGNATURES_AND_PLAN_VERIFIED_NO_STATE_CHANGED
HVM_R6_FOUNDER_FUNDING_VERIFIED_NO_STATE_CHANGED
OFFLINE_VALIDATOR_CHECKPOINT_REPLAY_VERIFIED_RUNTIME_ACCEPTANCE_REQUIRED
HVM_R6_VALIDATOR_CHECKPOINT_POLICY_VERIFIED_NOT_ACTIVATABLE
NO_SERVICE_STARTED
NO_PRODUCTION_ACTIVATION
HVM_R6_FOUNDER_SIGNED_FUNDING_AND_OFFLINE_VALIDATOR_CHECKPOINT_OK
```

The artifact contains public evidence only. The complete validator ledger remains
on the controlled runner under the per-run evidence directory; its
`blockchain.dat` and `blockchain.idx` SHA256 values are included in the
artifact log.

## 5. Boundary after HVM-R6

A successful HVM-R6 proves that the exact founder-authorized transfers, validator
registrations, zero-issuance bootstrap and replay reconciliation form one
internally consistent offline validator checkpoint.

It does not prove a running network. No service is started, no validator data
directory is provisioned, and no production activation is authorized. A later
reviewed gate must implement controlled provisioning and runtime acceptance.
Until then, `deploy` and `acceptance` intentionally return fail-closed.
