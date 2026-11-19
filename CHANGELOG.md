# Changelog

## 1.3.0

- Agent transport upgraded to a resilient REST/JSON client with a typed
  `/api/agent/hello` handshake, keep-alive, request timeouts, jittered
  exponential backoff and automatic reconnect after panel/network outages.
  Reports now default to a 5-second interval and server cards update live over SSE.
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
- UI polish: persistent light/dark theme toggle, consistent line-icon set, softer
  global halos, layered shadows, agent connection-quality cards, REST status,
  one-line install panel, proxy node health/latency meters and improved busy/loading states.
- Release/install documentation updated for v1.3.0.

## 1.2.0

- Full Xray real-handshake engine and proxy-node testing.
- Xray settings, installer and Docker integration.
- One-script VPS installer with optional Caddy/Let's Encrypt and Cloudflare
  DNS failover support.

## 1.1.0

- Initial Pulse release: monitors, incidents, rules, notifications, agents,
  Cloudflare automation, PWA and the Persian RTL interface.
