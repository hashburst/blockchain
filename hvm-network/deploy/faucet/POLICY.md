# HVM Public Faucet Policy

Status: owner-approved production policy candidate.

Public faucets are required for HVM testnet (chain 4735490) and HVM mainnet (chain 4735489). HBT is the native coin used to pay HashBurst network fees; the faucets reduce onboarding friction, support wallet/Web3 testing and permit promotional distribution.

## Monetary boundary

Mainnet faucet transfers are not issuance, mining rewards or protocol subsidies. Every mainnet grant MUST debit a dedicated faucet account funded from the approved 1,000,000,000 HBT founder allocation. The imported 450 HBT legacy balance is excluded from faucet funding. The exact reserve is frozen before founder funding is signed.

Testnet HBT is economically separate and has no mainnet value.

## Distribution and abuse policy

- valid HVM recipient address required;
- configurable amount per claim, cooldown, daily cap and global reserve;
- idempotency for accepted requests;
- reject malformed/zero addresses and protected founder, validator escrow and internal service destinations;
- durable append-only audit of decisions and resulting transaction hashes;
- rate limiting and abuse controls fail closed;
- signed transfers use the normal HVM transaction path;
- faucet signing key stays outside web root and API process behind a dedicated signer/secret boundary;
- APIs never accept, return or log founder/faucet private keys.

## Publication

HVM Network pages publish both network profiles, MetaMask-compatible chain/RPC metadata, native wallet tooling, faucet status and endpoints. A faucet is advertised as live only after runtime health and funding are independently verified.

Marker: HVM_PUBLIC_FAUCET_POLICY_APPROVED_NETWORKS_4735489_4735490
