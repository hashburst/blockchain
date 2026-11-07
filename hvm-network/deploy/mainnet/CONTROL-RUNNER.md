# Founder-controlled GitHub Actions coordinator

The HVM mainnet control plane is intentionally outside the validator fleet.

Coordinator:
- public IP: 77.90.188.180
- DNS: mlmultiservices.com
- GitHub runner labels: self-hosted, linux, x64, hashburst-control, founder-180
- production Environment: hvm-mainnet-production

Security boundary:
1. Fleet SSH private key is generated on the coordinator and remains only at /etc/hashburst-control/fleet_ed25519 (0600).
2. GitHub Actions receives no fleet private key and no founder wallet key.
3. /etc/hashburst-control/fleet_known_hosts is pinned; StrictHostKeyChecking=yes.
4. The runner account must not be root. sudo is allowed only through reviewed wrapper commands, not arbitrary shell.
5. The founder HBT wallet key never exists on this coordinator. Founder funding signatures are imported as public signed JSON.
6. Validator consensus/private identities are generated on their destination validator and are never copied to the coordinator.
7. preflight and benchmark are read-only. identities/checkpoint/deploy/acceptance fail closed until protocol.freeze.json and economics.freeze.json exist.
8. The coordinator is not a validator and has no voting power.

GitHub Environment hvm-mainnet-production should require the founder as reviewer and restrict deployment branches to reviewed mainnet branches/master. Environment approval controls job dispatch; it is not a secret store for fleet SSH credentials.

Bootstrap is intentionally not automated from GitHub because registering a self-hosted runner requires a short-lived GitHub registration token and local root/admin access on 77.90.188.180. After registration, normal fleet operations are driven by GitHub Actions.
