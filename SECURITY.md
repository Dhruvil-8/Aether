# Security Policy

## Supported version

Security fixes are applied to the current `main` branch.

## Reporting vulnerabilities

Do not open public issues for vulnerabilities that could expose node operators or message history integrity risks.

Report privately to project maintainers with:

- affected commit/tag
- impact summary
- reproducible steps
- suggested mitigation if available

## Security-sensitive areas

- Tor transport and control integration
- wire parsing and frame boundaries
- sync and history validation logic
- peer reputation/abuse controls
- storage append/replay correctness

## Disclosure process

1. Acknowledge report.
2. Reproduce and triage severity.
3. Patch and validate with tests.
4. Publish advisory with fixed commit/version.
