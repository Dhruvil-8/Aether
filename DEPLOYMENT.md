# Deployment Guide

This guide covers practical deployment for `aetherd`.

## Modes

1. Tor-managed mode (`AETHER_MANAGE_TOR=true`)
2. External Tor mode (`AETHER_MANAGE_TOR=false`)
3. Dev clearnet mode (`AETHER_DEV_CLEARNET=true`, testing only)

## 1) Linux systemd (recommended)

1. Build and place binary at `/opt/aether/aetherd`.
2. Create runtime user:
   ```bash
   sudo useradd --system --home /var/lib/aether --shell /usr/sbin/nologin aether
   sudo mkdir -p /var/lib/aether /etc/aether
   sudo chown -R aether:aether /var/lib/aether
   ```
3. Copy:
   - `deploy/systemd/aetherd.service` -> `/etc/systemd/system/aetherd.service`
   - `deploy/aetherd.env.example` -> `/etc/aether/aetherd.env` (edit values)
4. Enable/start:
   ```bash
   sudo systemctl daemon-reload
   sudo systemctl enable --now aetherd
   sudo systemctl status aetherd
   ```

## 2) Managed Tor mode

Set in env file:

```text
AETHER_REQUIRE_TOR=true
AETHER_DEV_CLEARNET=false
AETHER_MANAGE_TOR=true
AETHER_TOR_BINARY=tor
```

Notes:

- Tor binary must exist on PATH or at `AETHER_TOR_BINARY`.
- `aetherd` supervises Tor process lifecycle for configured SOCKS/control ports.
- Control password is not used in managed mode.

## 3) External Tor mode

Set:

```text
AETHER_REQUIRE_TOR=true
AETHER_DEV_CLEARNET=false
AETHER_MANAGE_TOR=false
AETHER_TOR_SOCKS=127.0.0.1:9050
AETHER_TOR_CONTROL=127.0.0.1:9051
```

`aetherd` expects Tor to already be running and reachable.

## 4) Preflight and health checks

Run:

```bash
aetherd doctor
```

`doctor` validates Tor/runtime reachability under current env config.

## 5) Docker dev mesh

For local multi-node testing:

```bash
docker compose -f deploy/docker-compose.dev.yml up --build
```

This compose file intentionally runs in dev clearnet mode.

## 6) Security baseline

- Keep `AETHER_REQUIRE_TOR=true` in production.
- Keep `AETHER_DEV_CLEARNET=false` in production.
- Store data under a dedicated non-root directory.
- Restrict firewall inbound to chosen listen port.
- Keep system clock in sync (NTP).
