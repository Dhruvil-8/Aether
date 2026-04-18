# Aether Protocol

Status: implementation-aligned working document

This file describes the protocol as implemented in this repository.

## Goals

- No protocol-level identity
- No mandatory central server
- Append-only local history on honest nodes
- Free posting constrained only by protocol validity and abuse controls
- Tor-first transport for production nodes
- Small wire and storage surface

## Message

Logical object:

```json
{
  "v": 1,
  "t": 1710000000,
  "c": "hello",
  "h": "hex sha256",
  "p": 0
}
```

Rules:

- `v` must be `1`
- `t` is `uint64` Unix time in seconds
- `c` is UTF-8 text, NFC-normalized, max 320 bytes
- `h` is lowercase hex SHA-256 of canonical body
- `p` is `uint32` PoW nonce

Canonical body:

```text
v      : 1 byte
t      : 8 bytes, big-endian
len(c) : 2 bytes, big-endian
c      : UTF-8 NFC-normalized bytes
```

Hash:

```text
h = SHA256(MESSAGE_BODY)
```

## Proof Of Work

PoW input:

```text
SHA256(raw_h_bytes || nonce_u32_be)
```

Acceptance rule:

- PoW hash must satisfy configured leading-zero-bit difficulty

Current constant:

- `DIFFICULTY = 20`

## Wire Protocol

Transport:

- TCP
- Tor SOCKS5 default for outbound
- Tor onion service for inbound in production mode

Frame layout:

```text
frame_len  : 4 bytes unsigned big-endian
frame_type : 1 byte
payload    : frame_len - 1 bytes
```

Current frame cap:

- `8 MiB`

Frame types:

| Type | Name | Purpose |
|---|---|---|
| `0x01` | `HELLO` | handshake |
| `0x02` | `GETPEERS` | request peers |
| `0x03` | `PEERS` | peer response |
| `0x04` | `MESSAGE` | relay one message |
| `0x05` | `SYNCREQ` | request message batch |
| `0x06` | `SYNCDATA` | sync batch response |
| `0x07` | `PING` | keepalive |
| `0x08` | `PONG` | keepalive response |
| `0x09` | `ACK` | acceptance acknowledgment |
| `0x0A` | `SYNCMETA` | cursor validation metadata |

Payload encoding:

- JSON for wire simplicity
- canonical identity still comes from binary message body hashing

## Handshake

Each side exchanges `HELLO`:

```json
{
  "network_id": "...",
  "node_version": "aetherd/0.1.0",
  "capabilities": ["relay", "tor-required", "archive"],
  "listen_addr": "example.onion:9000"
}
```

Rules:

- disconnect on `network_id` mismatch
- no persistent peer ID
- `listen_addr` may be omitted for outbound-only nodes

Network identifier:

```text
NETWORK_ID = SHA256("aether-protocol-v1")
```

## Relay Rules

For a `MESSAGE`, node flow is:

1. validate normalization/format
2. recompute `h`
3. verify PoW
4. dedup check
5. append locally
6. relay to connected peers
7. ACK sender after local acceptance

The protocol includes no author/account/signature/reply/moderation fields.

## Peer Discovery

Sources:

- configured bootstrap peers
- `GETPEERS` / `PEERS`
- persisted peers in `peers.json`

Current protections:

- peer-list response caps
- peer-announcement caps before persistence
- connection caps
- control-plane throttling
- weighted reputation penalties
- temporary bans
- persistent peer state with decay, eviction, and cooldown

Peer files:

- `peers.json`: known candidate addresses
- `peer_state.json`: score/backoff/ban/eviction state

## Sync

Current sync is batch-based, append-only, and resumable.

Main pieces:

- `SYNCREQ` requests message batches from `offset`
- `SYNCDATA` returns `messages`, `next_offset`, and `has_more`
- `SYNCMETA` returns:
  - `total_messages`
  - `tip_hash`
  - checkpoint hashes for requested offsets
  - accumulator digests for requested offsets
  - fixed chunk digests for requested chunk indices
  - chunk-Merkle root and inclusion proofs for requested chunks
  - rolling window digests for requested window ends/sizes

Persisted sync state:

```json
{
  "peer.onion:9000": {
    "offset": 123,
    "last_hash": "...",
    "checkpoints": [...],
    "accumulators": [...],
    "chunk_digests": [...],
    "chunk_merkle_root": "...",
    "chunk_merkle_leaves": 42,
    "chunk_merkle_proofs": [...],
    "window_digests": [...]
  }
}
```

Resume behavior:

- syncer asks peer for metadata before trusting saved cursor
- if direct `last_hash` check fails, syncer validates saved checkpoints
- if checkpoint validation fails, syncer validates saved accumulators
- if accumulator validation fails, syncer validates saved chunk digests using chunk-Merkle proofs
- if chunk validation fails, syncer falls back to last matching window digest
- if nothing matches, sync resumes from zero safely

What sync is today:

- practical for MVP
- better than full known-hash exchange
- supports checkpoint, accumulator, chunk-Merkle proof, and rolling-window fallback recovery

What sync is not yet:

- full cross-peer trust model and sparse archival proofs beyond current chunk-Merkle recovery proofs

## Storage

Local layout under data dir:

```text
.aether/
  peers.json
  peer_state.json
  sync_state.json
  segments/
    seg_000000.log
    seg_000001.log.gz
  tor/
    ...
```

Segment records:

```text
record_len : 4 bytes unsigned little-endian
record     : JSON-encoded logical message
```

Storage properties:

- append-only active segments
- sealed segment compression with gzip
- replay from plain and compressed segments

## Tor

Production posture:

- outbound via Tor SOCKS5 by default
- inbound via onion service
- clearnet is development mode

Defaults:

- SOCKS: `127.0.0.1:9050`
- control: `127.0.0.1:9051`

Current behavior:

- `aetherd post` runs SOCKS preflight in Tor-required mode
- `aetherd serve` runs SOCKS + control preflight in Tor-required mode
- Tor dials use explicit timeouts
- runtime watchdog checks Tor health and re-establishes managed onion service after recovery
- optional managed-Tor mode can start and supervise a local Tor process for the configured SOCKS/control ports

Managed-Tor mode still requires a valid Tor binary on host (`AETHER_TOR_BINARY`).

## Current Limits And Hardening

Current enforcement includes:

- max frame size
- per-connection message rate limits
- control-plane rate limits
- sync batch caps
- sync metadata request caps
- peer-list caps
- connection caps
- JSON payload caps before decode
- sync response byte caps
- idle read deadlines on established peer connections
- per-connection read-byte and frame budgets
- global inbound read-byte budget per second
- persistent peer scoring
- temporary bans and eviction cooldown
- fallback to healthy peers on peer failures
- non-archive recent-history serving window

These are abuse/transport controls, not content moderation.

## Metrics

Optional metrics endpoint:

- enable with `AETHER_METRICS_ENABLED=true`
- listen on `AETHER_METRICS_LISTEN`
- expose path `AETHER_METRICS_PATH`

Current counters include:

- accepted and relayed messages
- sync batches, applied messages, and failures
- peer dial failures and abuse events
- Tor recovery events
- connection and resource-limit rejects
- open connection gauge

## Current Commands

- `aetherd version`
- `aetherd doctor`
- `aetherd mine <message>`
- `aetherd post <message>`
- `aetherd post <peer> <message>`
- `aetherd serve`

## Next Protocol Work

- stronger archival sync trust model (cross-peer proof agreement, sparse archival proofs)
- longer-running partition/churn soak validation
- longer-horizon reputation tuning
