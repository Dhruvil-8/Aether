# Aether

An append-only stream of human thought, without identity.

Aether is a protocol for a global append-only stream of messages with no authors, no accounts, no central server, and no deletions. Anyone can write. Anyone can read.

## Development

This codebase was generated and iterated with LLM-assisted(OpenAI-Codex) development.

## What this repo provides

- `aetherd` - operator daemon (`serve`, `post`, `mine`, `doctor`, `version`)
- `aether` - user CLI (`timeline`, `post`, `status`, `peers`, `search`, `export`, `count`)
- `aether-web` - web UI binary with embedded node
- custom wire protocol (`HELLO`, `MESSAGE`, `SYNC*`, `ACK`, `PEERS`)
- PoW-gated message acceptance
- append-only segmented storage with sealed-segment compression
- peer reputation and abuse controls
- Tor-first transport with onion-service support and optional managed Tor mode

Protocol details: `PROTOCOL.md`  
Implementation status: `PROJECT_STATUS.md`  
Deployment runbook: `DEPLOYMENT.md`

## Quick start (local dev)

Prerequisite: Go 1.23+

Build:

```powershell
go build -o aetherd.exe ./cmd/aetherd
go build -o aether.exe ./cmd/aether
go build -o aether-web.exe ./cmd/aether-web
```

Run one node in dev-clearnet mode:

```powershell
$env:AETHER_DEV_CLEARNET="true"
$env:AETHER_REQUIRE_TOR="false"
$env:AETHER_LISTEN_ADDR="127.0.0.1:9000"
$env:AETHER_ADVERTISE_ADDR="127.0.0.1:9000"
.\aetherd.exe serve
```

Post:

```powershell
$env:AETHER_DEV_CLEARNET="true"
$env:AETHER_REQUIRE_TOR="false"
$env:AETHER_BOOTSTRAP_PEERS="127.0.0.1:9000"
.\aetherd.exe post "hello"
```

## Production profile

Use Tor-required mode:

- `AETHER_REQUIRE_TOR=true`
- `AETHER_DEV_CLEARNET=false`
- `AETHER_UI_ALLOWED_ORIGINS` can be set to a comma-separated list of trusted external web origins if the UI/API is hosted across origins. Same-origin UI requests and non-browser local tooling do not need this.
- `AETHER_SYNC_TRUST_QUORUM=2` or higher enables cross-peer agreement checks for sync metadata before trusting cursor recovery.

Optional managed Tor process:

- `AETHER_MANAGE_TOR=true`
- `AETHER_TOR_BINARY=tor`

If managed Tor is disabled, Tor must already be reachable at:

- `AETHER_TOR_SOCKS` (default `127.0.0.1:9050`)
- `AETHER_TOR_CONTROL` (default `127.0.0.1:9051`)

## Useful commands

```powershell
aetherd version
aetherd doctor
aetherd mine "message"
aetherd post "message"
aetherd post "<peer:port>" "message"
aetherd serve
```

## Docker dev mesh

```powershell
docker compose -f .\deploy\docker-compose.dev.yml up --build
```

## CI and release

- CI test workflow: `.github/workflows/ci.yml`
- Tag-triggered release workflow with multi-platform artifacts, `SHA256SUMS`, `PROVENANCE.json`, and keyless Sigstore bundles: `.github/workflows/release.yml`

## Safety and claims

Privacy and permanence are goals, not absolute guarantees. Retention depends on archival participation. Review `PROTOCOL.md` and `PROJECT_STATUS.md` before production use.
