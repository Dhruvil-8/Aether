# Contributing

## Scope

This repo is a protocol + reference node project. Changes should preserve:

- protocol minimalism
- no protocol-level identity fields
- Tor-first production posture
- append-only behavior

## Workflow

1. Open an issue describing the problem and expected behavior.
2. Keep changes narrow and test-backed.
3. Run `go test ./...` before submitting.
4. Update `PROTOCOL.md` and `PROJECT_STATUS.md` when behavior changes.

## Code standards

- Keep network parsing defensive.
- Keep wire/storage compatibility stable unless versioning is explicitly changed.
- Prefer explicit limits and timeouts over unbounded behavior.
- Avoid adding content-moderation logic into protocol/node acceptance path.

## Pull requests

PRs should include:

- summary of behavior change
- risk assessment
- test coverage notes
- migration/operational notes if env vars or deployment behavior changed
