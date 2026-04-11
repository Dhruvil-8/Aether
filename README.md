# Aether

**An append-only stream of human thought, without identity.**

Aether is a protocol. It defines a global, append-only stream of messages with no authors, no accounts, no servers, and no deletions. Anyone can write to it. Anyone can read it. No one owns it.

## Development: AI-Generated

> The entire Aether codebase was written by LLMs.
>
> **Protocol & core node**: OpenAI GPT Codex 5.4 and 5.3
> **CLI & Web UI**: Anthropic Claude Opus 4.6

## What this repo provides

- `aetherd` — operator daemon (`serve`, `post`, `mine`, `doctor`, `version`)
- `aether` — user-friendly CLI (`timeline`, `post`, `status`, `peers`, `search`, `export`, `count`)
- `aether-web` — single-binary web UI with embedded node (timeline, broadcast, status, peers)
- Custom wire protocol (`HELLO`, `MESSAGE`, `SYNC*`, `ACK`, `PEERS`)
- PoW-gated message acceptance
- Append-only segmented storage with sealed-segment compression
- Peer reputation and abuse controls
- Tor-first transport, managed onion services, optional managed Tor process mode

Protocol details: `PROTOCOL.md`
Implementation status: `PROJECT_STATUS.md`
Deployment runbook: `DEPLOYMENT.md`

## Quick start (local dev)

Prerequisites:

- Go 1.23+

Build all three binaries:

```powershell
go build -o aetherd.exe ./cmd/aetherd       # Core daemon (operator)
go build -o aether.exe ./cmd/aether         # User CLI (timeline, post, search)
go build -o aether-web.exe ./cmd/aether-web # Web UI + embedded node (single binary)
```


Run one node (dev clearnet mode):

```powershell
$env:AETHER_DEV_CLEARNET="true"
$env:AETHER_REQUIRE_TOR="false"
$env:AETHER_LISTEN_ADDR="127.0.0.1:9000"
$env:AETHER_ADVERTISE_ADDR="127.0.0.1:9000"
.\dist\aetherd.exe serve
```

Post message:

```powershell
$env:AETHER_DEV_CLEARNET="true"
$env:AETHER_REQUIRE_TOR="false"
$env:AETHER_BOOTSTRAP_PEERS="127.0.0.1:9000"
.\dist\aetherd.exe post "hello"
```

## Production profile

Use Tor-required mode:

- `AETHER_REQUIRE_TOR=true`
- `AETHER_DEV_CLEARNET=false`

Optional managed Tor process:

- `AETHER_MANAGE_TOR=true`
- `AETHER_TOR_BINARY=tor`

If `AETHER_MANAGE_TOR=false`, Tor must already be running and reachable at:

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

## Docker (dev mesh)

`deploy/docker-compose.dev.yml` starts a 3-node clearnet test mesh:

```powershell
docker compose -f .\deploy\docker-compose.dev.yml up --build
```

## CI and tests

Run all tests:

```powershell
go test ./...
```

## Safety and claims

This project improves privacy posture but does not guarantee perfect anonymity or permanent retention without archival participation. Read `PROTOCOL.md` and `PROJECT_STATUS.md` before production use.
