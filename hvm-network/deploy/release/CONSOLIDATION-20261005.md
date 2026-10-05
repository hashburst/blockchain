# Repository consolidation, 2026-10-05

PR 31 documents the networks and prepares the RC3 draft and website sources.
PR 32 integrates the economic genesis receipt into Go state, replay and recovery.
Both changes are merged; neither activates a VPS mainnet service.

Dependency consolidation covers QUIC v0.59.1, DTLS v3.1.4, STUN v3.1.5 and
Gorilla WebSocket v1.5.3 in the execution module. The required transitive updates
are transport/v4 v4.0.2 and x/time v0.14.0. HVM and EVM execution tests pass
locally on the combined source.

WebTransport v0.11.1 requires QUIC v0.60.0 and is excluded from this patch
consolidation. Issue 33 tracks the coordinated transport upgrade.

Documentation changes remove conversational introductions, decorative prose
punctuation and outdated active-protocol claims from maintained Markdown files.
Third-party vendor documentation, signed data, protocol strings and code examples
are not rewritten for editorial style. Historical examples remain marked as
such and do not define current HVM consensus.

RC3 is a draft built from its recorded SOURCE_COMMIT. It is not a binary of every
subsequent master change. A final release requires a fresh build from the accepted
final commit, all platform checksums and mainnet activation evidence. Do not
promote old candidate binaries by changing only a release title.
