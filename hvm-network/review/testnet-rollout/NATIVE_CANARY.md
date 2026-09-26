# Native funded application acceptance

Operator output dated 2026-09-26, Canary v1.0.1:

- fund: 0x3245e15f40d7274f242e57e5cf6a2291cb8be14548fcd6a6d111c8efbd7d61eb, fee 101 native units
- deploy: 0x9470f581083ccdb3b7d006b583faf1f891f98c09e7191d47e8fcc0f4a478a817, fee 125
- profile: 0x8a30b53bab88a79b12eb1c4e85a5f55716aaa86547c4d3daf1c1ddc50221e7f3, fee 108
- HVM_NATIVE_FUNDED_CANARY_OK
- PUBLIC_NATIVE_RECEIPTS_AGREEMENT_OK
- PUBLIC_NATIVE_FINALIZED_COMMITMENT_OK height=43438
- HVM_NATIVE_FUNDED_APPLICATION_TEST_OK

The HTTPS verifier ran from validator v1 after Mac-specific directory commands
failed in the SSH session. Its successful result proves agreement through the
public HTTPS endpoint from that host; it is not an independent external-vantage
network test. Earlier ingress verification was executed from the Mac.

This is native HVM acceptance, not Ethereum/MetaMask or mainnet certification.
The pasted output is the evidence source; no raw public-proof JSON is archived
in this change. Keys and validator journals are not included.
