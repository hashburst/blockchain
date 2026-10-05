# Reviewed HA transition during archive rollout

The observed master PID change on .153 is supported by HA fencing/start logs
and primary acquisition with 3/3 grants. HA configuration names the legacy node
as a prerequisite for primary-only hashburst-master.service. This is not a
claim that the master remained unchanged.

`accept-ha-archive.py --deployer-root PATH` performs a fresh, read-only audit
of all five archives and accepts ONLY the documented master PID transition on
.153. It verifies pinned installer/binary identity, terminal ledger commitments,
balance and rejected writes, unchanged other protected services, historical HA
journal evidence and current HA eligibility/primary lease in two samples.
A further PID change is not silently accepted. Secrets are not exported.

It does not modify the installer, protected.json or original coordinator state,
and never stops/starts services. The local original state still records the old
rejected invariant. The new timestamped FLEET-FREEZE-HA report is the successor
acceptance evidence; do not rerun the old deployment/audit to clear that state.
The result is scoped to managed services at their observation times, not all
possible ledger copies. Mainnet import and activation remain false.

Tests cover the exact exception and rejection of missing services, new PID,
other host, or additional protected-service changes. The existing rollback-free
installer regression tests remain enabled. A production run, not these tests,
produces fleet acceptance.
