# HBT strict profile v0.1

HBT-XXXX is a HashBurst deployment label for the corresponding ERC contract
standard, not a changed wire protocol. Original ABI, selectors, events and
standard-defined interface identifiers are retained. Do not substitute HBT names
inside Ethereum function/event signatures. ERC-165 IDs are not invented for
standards that do not define them.

The checked-in example contracts import OpenZeppelin Contracts 5.4.0, compiled
with Solidity 0.8.30, optimizer 200 runs, Cancun. The lockfile pins dependencies.
They are test templates, not founder allocation or production asset contracts.
Their symbols are STA/STN/STV to distinguish assets from native HBT used for gas.
Native HBT has no ERC-20 contract address; asset identity includes chain ID and
contract address, not ticker alone. Wrapped native HBT would need an explicit
reserve-backed wrapper, not a metadata rename.

| Profile | Preserved standard | Current implementation/check |
|---|---|---|
| HBT-20 | ERC-20 | OZ token, transfers/balances/Transfer log tested |
| HBT-2612 | ERC-2612 | OZ permit inherited; dedicated signature/replay conformance pending |
| HBT-5267 | ERC-5267 | OZ EIP-712 domain introspection inherited; no renamed domain protocol |
| HBT-721 | ERC-721 plus metadata | ownership transfer and original ERC-165 IDs tested |
| HBT-4906 | ERC-4906 | OZ URI-storage extension and interface ID retained |
| HBT-2981 | ERC-2981 | royalty interface preserved; marketplace payment is not enforced |
| HBT-1155 | ERC-1155 plus metadata | transfer/balance and original interface IDs tested |
| HBT-165 | ERC-165 | original IDs, including rejection of 0xffffffff |
| HBT-4626 | ERC-4626 | OZ vault, deposit/withdraw/accounting smoke test |
| HBT-1271 | ERC-1271 | immutable-owner signature verifier, valid/invalid signatures tested |

Run `npm ci --ignore-scripts && npm run build`, then from `../execution` run
`go test -v ./...`. The latter executes the compiled artifacts in the actual HVM
execution engine using signed type-2 transactions. This is not a full formal ERC
conformance suite, independent audit or live deployment certification.

Measured constructor gas in this build:

| Template | Gas |
|---|---:|
| HBT20Strict | 973741 |
| HBT721Strict | 1160693 |
| HBT1155Strict | 1156001 |
| HBT1271Strict | 238890 |
| HBT4626Strict | 1069136 |

All exceed the previously configured 200000 block gas limit. Therefore these
full templates MUST NOT be advertised as deployable on that configuration.
An explicitly reviewed higher gas limit (and block load tests) is a rollout gate,
not an automatic change to running nodes. Runtime maximum remains 30000000;
this document does not select a production value.

Additional ERCs enter a versioned registry only after dependency, ABI and
behavior tests. There is no universal promise to support every future ERC.
ERC-4337/EntryPoint/bundlers/paymasters and dedicated account-abstraction RPC are
excluded. Core changes such as EIP-7702 or blob transactions are not token
standards and are not activated by an HBT label. Cross-chain bridges, oracle,
indexer and marketplace services are not implied by an ABI-compatible contract.

MetaMask remains an Ethereum-compatible wallet: accounts, signing, gas estimates,
transactions and supported token displays. It does not run APoW/PoH/BFT. DApps
invoke vaults, permits, royalties and other contract functions using unchanged
ABIs. A standard being executable does not guarantee built-in MetaMask UI support.
IT: nessuna modifica del wallet richiesta per il consenso; EN: consensus runs
on nodes, not in the wallet.

Sources: https://github.com/OpenZeppelin/openzeppelin-contracts/tree/v5.4.0
and https://eips.ethereum.org/ (individual ERC specifications).
