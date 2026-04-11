# Aether Project Status

## Current State

The project is in active MVP development.

It is no longer just a protocol idea. A working Go implementation exists with:

- canonical message encoding and hashing
- Unicode NFC normalization in the message path
- proof-of-work mining and verification
- custom framed wire protocol
- Tor SOCKS outbound dialing by default
- onion service provisioning through the Tor control port
- peer handshake and peer discovery
- peer persistence across restarts
- append-only segment storage
- sealed-segment compression
- offset-based resumable history sync
- ACK-based message acceptance
- periodic bootstrap and sync maintenance
- persistent outbound peer connector loop
- periodic peer exchange over live connections
- basic per-connection rate limiting
- bounded frame parsing
- server-side caps for sync batches and peer exchange payloads
- connection caps and control-plane throttling
- weighted peer scoring and temporary bans for repeat offenders
- sync metadata with checkpoint validation for persisted cursors
- sync metadata with rolling window digests and tip-hash reporting
- sync metadata with accumulator digests for stronger recovery checkpoints
- fallback to healthy peers after failed send/sync/bootstrap attempts
- persistent peer reputation with score decay and eviction thresholds
- eviction cooldowns that block immediate peer reintroduction
- periodic peer-state sweep cleanup and broader churn fallback coverage
- Tor SOCKS/control preflight validation and timeout-aware Tor dials
- runtime Tor health watchdog with managed onion re-establishment on recovery
- optional managed-Tor process startup and supervision in `serve`
- structured leveled logging (`debug`/`info`/`warn`/`error`) across serve/post and maintenance loops
- optional Prometheus-style metrics endpoint with runtime counters
- archive-mode signaling with bounded non-archive history serving policy
- JSON payload size caps and idle-connection read deadlines
- operational packaging assets (`Dockerfile`, compose, PowerShell build/smoke scripts)
- open-source readiness assets (`README`, `LICENSE`, `CONTRIBUTING`, `SECURITY`, CI workflow)

## Achieved

### Protocol and format

- Fixed message schema with deterministic hashing
- Unicode NFC normalization during validation and hashing
- Deterministic network ID
- Push-based message relay
- Cursor-based sync request and sync response frames
- Sync metadata frames for cursor validation
- Sync metadata frames with rolling window digests
- Sync metadata frames with accumulator digests
- ACK-based message acceptance
- Local caps on sync and peer-exchange payload sizes regardless of remote request size
- Local caps on sync-metadata request cardinality
- Control-plane request throttling on established peers
- Weighted peer penalties with temporary bans after repeated abuse
- Failure accounting on sync/bootstrap paths with fallback to healthy peers
- Persistent peer reputation with decay across restarts and eviction of heavily penalized peers
- Eviction cooldowns that quarantine penalized peers before they can re-enter candidate rotation
- Periodic peer-state cleanup plus repeated fallback and mesh-churn coverage
- Sparse sync checkpoints carried in persisted cursors with fallback to the last matching checkpoint
- Rolling window digests in persisted cursors with fallback to the last matching digest window
- Accumulator digests in persisted cursors with fallback before window-based recovery

### Node behavior

- `aetherd serve`
- `aetherd post`
- `aetherd mine`
- Peer book persisted in `peers.json`
- Peer reputation persisted in `peer_state.json`
- Startup restore of seen hashes from local storage
- Persisted per-peer sync offsets in `sync_state.json`
- Persisted per-peer sync cursors with last-hash validation
- Persisted sparse sync checkpoints for partial cursor recovery
- Persisted window digests for stronger partial cursor recovery
- Persisted accumulator digests for stronger partial cursor recovery
- Background bootstrap loop
- Background peer exchange loop
- Background sync loop
- Background outbound peer connector loop
- Basic peer rate limiting and invalid-message disconnect behavior
- JSON payload cap enforcement before JSON decode
- Idle read deadlines on established peer connections
- Peer announcement clamping before persistence
- Connection-cap rejection and backoff for excess peers
- Repeat-offender scoring with temporary bans
- Non-archive recent-history serving window for sync responses

### Storage

- Append-only local segment files
- Automatic segment sealing
- Compression of sealed segments
- Replay of both active and compressed segments

### Transport

- Custom TCP frame protocol
- Tor SOCKS5 outbound transport
- Tor control-port onion service creation
- Onion address persistence under `.aether/tor/`
- Tor SOCKS/control startup preflight checks and timeout-aware control/proxy dialing
- Runtime Tor health checks and onion service re-establishment after control/proxy recovery
- Managed-Tor process startup and lifecycle supervision support (`AETHER_MANAGE_TOR=true`)

### Observability

- Structured leveled logger configurable via `AETHER_LOG_LEVEL`
- Log coverage for `serve`, `post`, bootstrap failures, sync failures, connector/peer-exchange debug ticks, and Tor lifecycle events
- Optional metrics endpoint (`AETHER_METRICS_ENABLED`) with counters for relay/sync/peer/Tor recovery behavior

### Testing

- Unit tests for message encoding, PoW, wire framing, config, peer persistence, storage, and Tor control parsing
- Integration tests for relay, back-to-back delivery, live peer learning, multi-hop relay, and sync across multiple nodes in dev-clearnet mode
- Resume-sync test coverage for persisted offsets
- Sync-metadata tests for cursor reset on mismatch and legacy state migration
- Focused tests for sync batch clamping and peer announcement limits
- Focused tests for control-frame throttling and connection-cap enforcement
- Higher-volume long-chain relay coverage
- Peer-book tests for score decay and temporary ban behavior
- Failure-mode integration tests for post/sync fallback when one peer is dead
- Peer-book tests for reputation persistence and eviction behavior
- Peer-book tests for eviction cooldown and blocked reintroduction
- Repeated fallback-post coverage and mesh relay churn coverage
- Integration coverage for checkpoint-based sync fallback and bridge-node recovery after restart
- Integration coverage for window-digest-based sync fallback
- Integration coverage for accumulator-digest-based sync fallback
- Sustained sync convergence coverage under repeated dead-peer fallback loops
- Partition/bridge flap convergence coverage with explicit sync-based catch-up
- Focused coverage for non-archive recent-history sync serving policy
- Focused coverage for sync metadata window digest generation and tip hash
- Focused coverage for oversized JSON payload disconnect/backoff behavior

## Pending

### High priority

- Longer-horizon reputation tuning beyond persistent decay, cooldown, and threshold eviction
- Broader long-duration soak and multi-partition integration campaigns beyond current bridge-flap coverage
- Stronger archival sync metadata beyond checkpoint/accumulator/window-digest recovery

### Production hardening

- Rich observability (metrics/tracing) beyond structured logs
- Release packaging automation (signing/versioned artifacts) beyond current Docker/PowerShell assets
- Archive-node policy expansion beyond current non-archive history window behavior
- Global resource accounting and stress limits beyond current payload/idle/read guards

### Not done yet

- Browser client
- Desktop app
- Moderation/client-layer filtering
- Advanced archival verification such as Merkle-based sync

## Honest Risks

- Privacy claims are still weaker than the final vision
- The current sync design is materially better than hash-list sync, but it is still not archival-grade
- Reliability is better than before because of ACKs, but still not equivalent to a mature messaging network
- Tor integration now fails faster and more clearly at startup, but still depends on an existing Tor installation and compatible control-port configuration
- Managed-Tor startup/supervision requires a Tor binary on host and does not auto-install dependencies
- The node now bounds several remote-controlled paths, but it still needs more global resource accounting under sustained load
- Defensive parsing and resource limits are stronger (JSON caps, sync-meta caps, idle deadlines), but sustained-load accounting is still incomplete
- Peer penalties now persist, decay, and quarantine evicted peers across restarts, but long-horizon tuning is still simple
- Sync metadata now supports sparse checkpoint recovery, but it is still not a full archival proof structure such as sparse Merkle verification
- Sync metadata now supports checkpoint/accumulator/window fallback recovery, but it is still not a full archival proof structure such as sparse Merkle verification
- Failure fallback now covers repeated dead-peer, checkpoint/accumulator recovery, and bridge-flap cases, but not yet long-duration soak or multi-partition campaigns

## Recommended Next Steps

1. Tune long-horizon reputation and eviction policy under real churn, repeated reintroduction, and long-lived peer histories.
2. Expand from dead-peer fallback and bridge-flap recovery tests to longer soak, churn, and multi-partition integration scenarios.
3. Improve sync metadata from checkpoint/accumulator/window recovery toward sparse archival proof verification.
4. Add release-grade packaging automation (signed versioned artifacts) and expanded deployment operator docs.
