# APoW testnet rollout: immutable staging

This is the first live deployment step, not APoW activation. Chain 4735490 only.
Use stage-all.py on the Mac with the unmodified Linux amd64 release archive:

    python3 stage-all.py --release /path/HashBurst-HVM-v0.4.0-rc.1-linux-amd64.tar.gz

The archive SHA256 is pinned in the script. Only the two named regular binaries
are extracted; archive paths and symlinks are not extracted. Password prompts
belong to SSH/scp. Host key verification is retained. No password is stored.

Before changes, all four validators and the observer must agree at one finalized
height and show progress. Certificates must be present; this Python comparison
does not independently verify QC signatures. The runtime remains responsible for
chain validation. A failure retains a report without relaxing the gate.

On each node, the script stages a root-owned immutable candidate runtime under
/opt/hashburst-apow-candidate/<sha256>/hashburst-testnet. It does not change the
running node's ExecStart or configuration. On each validator only, miner-service.py
creates or reuses an isolated miner key and stages a stopped service, CPUQuota=25%.
The miner's own native address is its reward beneficiary. This is distinct from
the validator and founder keys; no reward is created by staging. Miner keys stay
on the VPS, are not included in reports, and need private operator backups.
The public report contains node IDs, paths, prefix hashes and miner addresses.

Configuration and identity pin bytes must remain identical. Existing signing
journal prefixes must remain intact; legitimate appended votes are allowed.
Runtime binary identity is checked before and after staging. The observer never
gets a miner or signing key. Failures retain staged files; no automatic rollback,
node restart, migration retry or journal truncation occurs. A repeat stage may
reuse exact matching staged files, never a conflicting release/unit/key.

Expected final marker:

    FIVE_CANDIDATES_FOUR_MINERS_STAGED_NO_ACTIVATION

Keep the generated apow-stage-results-*.tar.gz for the coordinated migration.
Next requirements: miner work measurement; explicit common APoW parameters;
future activation height; coordinated offline migration with runtime locks and
journal preservation; resumed finality and reward reconciliation; single-node
restart; EVM and MetaMask regression. These are not certified by STAGED.json.
Do not independently run miner-service.py start as an activation procedure.

Mainnet remains inactive. The approved economic policy is a verified legacy
import with canonical legacy spending freeze, plus an additional 1 billion HBT
founder allocation to 0xd1Da8D04D767685e53440DbC56803aF350A65333. Faucet allocation
is a subset of that founder budget. Neither a snapshot nor that freeze has been
implemented by this staging operation. Legacy 1337 and active testnet 4735490
are not rewritten here.
