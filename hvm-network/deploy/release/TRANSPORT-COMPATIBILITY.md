# Transport dependency compatibility

Issue #33 concerned dependency selection, not a demonstrated production outage
or a security advisory. WebTransport 0.11.1 requires QUIC 0.60.0; updating only
WebTransport while pinning QUIC 0.59.1 was inconsistent.

The coordinated candidate uses go-libp2p 0.48.0, quic-go 0.60.0 and
webtransport-go 0.11.1. Local full Go regression passes. The race-enabled
TestTransportDependencyCompatibility exercises real loopback TCP, QUIC and
WebTransport hosts, encrypted stream delivery, peer disconnection and a second
connection. It is included in the checkpoint CI race gate.

Production listener configuration is unchanged: the current HVM constructor
explicitly listens on TCP. This dependency change does not enable UDP listeners,
change TEP identities, or authorize a network migration. Loopback tests do not
measure WAN performance or prove production finality.

Mainnet activation still requires production public validator registrations,
approved protocol parameters, a complete checkpoint and runtime acceptance.
A dependency update does not supply these artifacts.
