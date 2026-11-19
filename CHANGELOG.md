# Changelog

## 1.4.1

- Agent live reports now default to **1 second**; server cards update in place
  over SSE with no full page re-render or visible flicker.
- Metric history is sampled every 10 seconds to keep the database light.
- UI bug fixes: stable agent-card patching, live page-summary counts, light-theme
  primary buttons, softer/larger halos, no halo clipping behind agent cards,
  theme-aware charts and better mobile/topbar spacing.
- Version bump so `pulse update` on 1.4.0 can install the fresh binary.

## 1.4.0

- Version bump and new release so `pulse update` on 1.3.x installs a fresh binary.
- Agent reports default to a 5-second interval; server cards update live over SSE.
- Persistent light/dark theme toggle (stored in the browser).
- Refined icon set, softer larger halos, layered shadows, removed halo clipping
  behind cards, improved light-theme contrast and gauges/charts.
- Kept all 1.3.0 REST agent, one-line installer, CLI and proxy-test fixes.

## 1.3.0

- Agent transport upgraded to a resilient REST/JSON client with a typed
  `/api/agent/hello` handshake, keep-alive, request timeouts, jittered
  exponential backoff and automatic reconnect after panel/network outages.
- Agent installation is now a single command generated directly in the panel.
  The token is stored in `/etc/pulse/agent.env`; it is no longer embedded in
  the systemd unit.
- Added `pulse update`, `pulse restart`, `pulse status`, `pulse logs` and the
  matching `pulse-agent` / `pulse-a` service commands.
- `pulse update` downloads the official Linux release, verifies `SHA256SUMS`,
  replaces the binary atomically and restarts the selected service.
- Proxy checks now use the real Xray tunnel as the source of truth even when
  the preliminary TCP/TLS probe fails (fixes false negatives on Reality, CDN
  and WebSocket nodes). Proxy check timeouts and the manual check wait were
  increased to match real-handshake cost.
- The **Check now** button now waits for the result and refreshes the monitor
  instead of only queueing a background check.
- UI polish: consistent line-icon set, agent connection-quality cards, REST
  status, one-line install panel, proxy node health/latency meters and improved
  busy/loading states.
- Release/install documentation updated for v1.3.0.

## 1.2.0

- Full Xray real-handshake engine and proxy-node testing.
- Xray settings, installer and Docker integration.
- One-script VPS installer with optional Caddy/Let's Encrypt and Cloudflare
  DNS failover support.

## 1.1.0

- Initial Pulse release: monitors, incidents, rules, notifications, agents,
  Cloudflare automation, PWA and the Persian RTL interface.
