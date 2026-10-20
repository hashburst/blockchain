# Native Windows full-node development

This branch is a portability candidate, not an operational Windows release.

The EVM store and runtime use nonblocking Windows LockFileEx locks. The runtime
opens the lock without following reparse points and rejects directories. Closing
the handle releases the lock. Unix flock behavior remains unchanged.
CI tests exclusion between processes and reopening after release on both systems.

Native production support remains blocked on:
- private-key and state-directory ACL creation and verification;
- durable checkpoint and signing-journal publication and crash recovery;
- owner/ACL-preserving offline migrations (currently rejected on Windows);
- native runtime provisioning, finality, shutdown and restart acceptance;
- Windows installation, upgrade and service lifecycle acceptance.

Do not remove Unix permission checks or ignore failed directory flushes merely
to pass Windows tests. A successful cross-build is not runtime acceptance.
Existing Windows wallet/auditor packages retain their published scope.

Mainnet protocol and signed registrations are separate production inputs.
No production protocol, validator identity or founder signature is synthesized.
