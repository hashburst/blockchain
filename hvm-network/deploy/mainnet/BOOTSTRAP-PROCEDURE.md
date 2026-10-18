# Offline mainnet validator checkpoint candidate

This procedure does not authorize production activation. It requires reviewed
protocol parameters and actual signed production enrollments. No testnet key,
ledger, journal or funding transaction may be used. The accepted legacy freeze
report remains unchanged; the legacy rollout is not repeated.

## Protocol and custody prerequisites

Set mainnet_bootstrap_end explicitly in the complete protocol JSON. Protocol V2
starts at height 1, node registrations and founder funding occupy block 1,
validator registrations occupy block 2, and subsequent bootstrap blocks are
empty apart from a zero-value system entry. Consensus starts at
mainnet_bootstrap_end + 1. The end must be between 3 and 4096 and accommodate the
validator activation delay. EVM and APoW activate after the bootstrap interval.
These constraints are enforced by consensus validation and replay, not just the
installer. The field is omitted for existing legacy/testnet configurations.

The exact protocol must be frozen before generating enrollments or signatures:
changing it changes their commitment and the economic genesis. Values for bond,
fees, gas, timing, difficulty and reward policy must be reviewed separately.
The code does not silently copy testnet values.

## Build on the administration host

From hvm-network in the reviewed source commit:

```
go test -race ./internal/mainnetidentity ./cmd/hvm-mainnet-identity
go test -race ./blockchain -run 'TestMainnetBootstrap|TestMainnetImport' -count=1
go build -trimpath -o hvm-mainnet-identity ./cmd/hvm-mainnet-identity
go build -trimpath -o hvm-mainnet-bootstrap ./cmd/hvm-mainnet-bootstrap
```

Use the identity enrollment procedure on each selected validator host. Export
only public.json. Private keys stay on their owning hosts. Independently verify
TEP federation membership; these tools prove P2P/consensus possession, not TEP
federation admission.

## Prepare the economic checkpoint

Use the exact approved freeze report and terminal generation with the same
complete protocol that was used to generate public enrollments:

```
./hvm-mainnet-bootstrap --protocol protocol.json \
  --freeze-report FLEET-FREEZE-HA-1791220850167915000.json \
  --source legacy-terminal-generation --out "$PWD/economic-checkpoint"
```

The output must be new. The source is an offline terminal copy, not a mutable
node directory. The command verifies terminal integrity and freeze-report hash.

## Plan and sign founder transfers

```
./hvm-mainnet-identity funding-plan --protocol protocol.json \
  v1.public.json v2.public.json v3.public.json v4.public.json > funding-plan.json
./hvm-mainnet-identity sign-funding --protocol protocol.json \
  --key "$FOUNDER_KEY_FILE" \
  v1.public.json v2.public.json v3.public.json v4.public.json > founder-transfers.json
./hvm-mainnet-identity verify-funding --protocol protocol.json \
  --signed-funding founder-transfers.json \
  v1.public.json v2.public.json v3.public.json v4.public.json
```

FOUNDER_KEY_FILE is the existing canonical mode-0600 raw wallet.key file on the
founder's signing computer, not a keystore directory. The command checks that
its address matches the approved founder. Signing authorizes the specific
transfers; it is not the earlier no-transfer custody challenge. Never upload
this key or its contents. The signed transfer JSON is public.

## Execute and reconcile offline

```
./hvm-mainnet-identity assemble --protocol protocol.json \
  --economic-ledger "$PWD/economic-checkpoint/ledger" \
  --signed-funding founder-transfers.json --out "$PWD/validator-checkpoint" \
  v1.public.json v2.public.json v3.public.json v4.public.json
```

The assembler exclusively creates its output, executes the signed transfers and
registrations using the core executor, checks receipts, waits the configured
activation delay, and reconciles founder funds, escrow balances, fees and total
supply. The 450 imported HBT remain part of the initial balance; bootstrap adds
zero issuance. It writes a binary ledger generation and reopens it with full
replay to compare native and validator commitments. CHECKPOINT.json is produced
only after those checks. Partial output is retained on failure and must not be
provisioned. No existing directory is overwritten. Independent builds may have
different block timestamps/nonces: distribute one accepted generation and its
hashes, rather than generating a separate checkpoint on each node.

## Production gate

CHECKPOINT.json retains activation_allowed=false. This artifact is not evidence
of a running mainnet. Provisioning still requires per-host mainnet configs,
verified TEP membership, custody-preserving installation, common-height finality,
restart/signing-journal checks and public RPC acceptance. Do not substitute
legacy/testnet services or flip the flag to bypass those checks. Final release
and website activation claims must reference the accepted source and runtime
reports. No automatic production installer is supplied by this candidate.

## Runtime bootstrap boundary

The initial validator checkpoint ends at mainnet_bootstrap_end. The first BFT
block is at mainnet_bootstrap_end + 1. The runtime accepts this exact boundary,
not an earlier economic checkpoint. After replay it requires 4..6 funded active
validators; a validator host must belong to that active set. This additional
initial-state check also runs on an observer. Existing post-activation recovery
continues to use the normal dynamic validator rules.

Before any service start, install one accepted ledger generation into each
host's dedicated mainnet data directory and use a reviewed per-host configuration.
Run the following commands as the future runtime user, with the production binary:

```
hashburst-mainnet --config /etc/hashburst-hvm-mainnet/config.json --provision
hashburst-mainnet --config /etc/hashburst-hvm-mainnet/config.json --check
```

Provision is a one-time operation: it creates the config/identity pin and signing
journals. A retry after successful provision uses --check, never deletes journals
or replaces the ledger. The observer uses its separate mainnet-ingress directory
and config. Both commands replay and validate state but do not open listeners.
A successful check is not fleet acceptance.

## Fleet acceptance record

Record the source commit and binary SHA256, protocol commitment, checkpoint hash,
state root, validator-set root and public identity for every host. After starting
only the dedicated mainnet services, verify chain ID 4735489 and compare block
hashes and native/validator commitments at the same finalized height on all nodes.
Equal heights alone are insufficient. Verify each quorum certificate against the
registered production validator set and record later finalized-height advancement.
Repeat after a controlled observer restart and then one validator restart, with
the voting-power quorum available throughout. Check journal prefix preservation
and new finality after each restart. Never restore an old signing journal.

Reconcile the founder balance, funded operators, bond escrows and fees against
funding-plan.json at the bootstrap checkpoint, and verify the consumed legacy
import is still present after recovery. At later heights account separately for
legitimate transaction fees and protocol rewards; the initial allocation must not
be compared blindly against a later circulating supply. A repeated bootstrap or
import must be rejected without changing balances or the import commitment.

Final publication needs these signed-input and runtime reports. A shell command
that changes an activation flag or marks a deployment complete cannot produce
missing evidence. Website content must describe the observed network status.
