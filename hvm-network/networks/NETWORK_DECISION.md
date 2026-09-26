# HashBurst HVM: network identity and testnet rollout, 2026-09-25

The project assigns HVM Mainnet chain ID 4735489 (0x484201), and HVM Testnet
chain ID 4735490 (0x484202). The hexadecimal prefix 4842 encodes HB, with network
suffix 01/02. Both IDs fit the RPC signed 64-bit representation and JavaScript
safe integer range. Legacy 1337 remains unchanged.

Registry check: ethereum-lists/chains at commit
4deb40dafb2675f0d120a79b30b20b90f363bb63, 2771 chain records. Neither ID nor a
HashBurst entry is present. A check of the titles/bodies of 68 open PRs found no
reference to either ID; that search does not inspect every pending PR diff.
This is a project assignment, NOT a public registry allocation or a claim of
global uniqueness. Recheck conflicts before public activation and submit accurate
metadata when supported endpoints exist. Do not publish invented RPC URLs or
claim full Ethereum wallet compatibility from these numeric identifiers.

This change is configuration/planning only: it changes neither public genesis
nor running chain ID, replay rules, consensus, or services. The bootstrap CLI
continues to require an explicit --chain-id. Use4735490 for the new persistent
testnet, not the isolated test ID 987654321 in unit tests. Do not relabel an
already-provisioned checkpoint or reset a validator journal to change IDs.

## Target topology, pending live checks

Coordinator hpcVM46 (77.90.188.158): builds one offline checkpoint; no signing
keys collected. Initial validator candidates are 77.90.188.153, 77.90.188.154,
77.90.188.155, 77.90.188.157, whose TEP traffic was previously observed. Their
current role/readiness is verified by collect-preflight.sh, not inferred from
historical traffic. A failed check does not silently substitute another host.
These are distinct hosts but not a claim of geographically independent failure
domains. Existing DePIN HA roles/services are preserved.

## Next executable step

Run deploy/testnet/collect-preflight.sh from the Mac or coordinator. It opens
four normal SSH connections and may ask for passwords/host-key verification.
It reads only TEP PUBLIC status, IP assignment, current units and path existence,
and briefly binds/closes the two intended TCP ports without listening. It does
not read key files or change any remote configuration. It writes local JSON
reports. Nonempty/new-state conflicts stop provisioning rather than deleting data.
This is a narrow bootstrap preflight, not a new full inventory or HVM Network gate.

After all checks pass, use the v1.0.0 bootstrap package with 4735490 and the four
verified public TEP keys, one fresh identity per host. Transfer public.json only,
assemble the shared checkpoint once, verify its digest, then provision each node
in observer mode. Stage the same 06d7c3bd runtime on hosts where it is absent.

The subsequent validator start must be coordinated: four observers alone cannot
advance finality. Before activation verify TCP 31307 reachability between the
selected hosts; the preflight bind test does not establish firewall reachability.
RPC 18009 stays loopback-only. Public ingress and Mainnet rollout remain later.
