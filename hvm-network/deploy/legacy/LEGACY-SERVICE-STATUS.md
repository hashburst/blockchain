# Legacy service status

Owner-authorized PR #30 service-status update, 2026-10-04.

On 77.90.188.155, `hashburst-node.service` was explicitly stopped at 09:27:16 UTC after a restart loop. The current application log is `/var/log/hashburst/node6.log`. The reported fatal error is `REWARD_ADDRESS not set`. `NODE_KEYSTORE` and `NODE_KEYSTORE_PASSWORD_FILE` are also unconfigured. The default P2P key file `/var/lib/hashburst/node_p2p.key` exists. No private key or credential is included here. No repair or restart is claimed completed.
