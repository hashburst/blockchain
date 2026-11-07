# Security Policy

## Supported versions

Security fixes are applied to the current HVM development line and to explicitly supported release branches/tags. Historical proof-of-concept and superseded test artifacts are not assumed to receive security fixes unless a release notice states otherwise.

## Reporting a vulnerability

Please do not open a public issue for an undisclosed vulnerability.

Preferred channel: GitHub Private Vulnerability Reporting / Security Advisories for this repository. If private vulnerability reporting is temporarily unavailable, contact the repository owner through a private channel and provide only enough information to establish contact; do not publish exploit details, private keys, credentials, validator material or live infrastructure secrets.

Include the affected commit/tag and component, impact and preconditions, minimal reproduction steps, whether mainnet/testnet/validator/wallet/EVM/APoW/RPC behavior is affected, and a suggested remediation if known.

## Sensitive material

Never submit founder wallet keys, validator private identities, fleet SSH private keys, API tokens, seed phrases or production credentials in issues, pull requests, Actions logs or advisory comments.

## Disclosure

We will validate reports, coordinate remediation and publish an advisory when appropriate. Mainnet-impacting fixes may require coordinated release and validator rollout before public technical disclosure.
