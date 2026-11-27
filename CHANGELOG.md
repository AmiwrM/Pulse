# Changelog

## 1.6.4

- Pass the VLESS `encryption` value through to the generated Xray outbound.
  Newer subscriptions use `encryption=mlkem768x25519plus...`; Pulse previously
  always sent `encryption: none`, so those handshakes could not succeed.
- HTTP monitors can now search the keyword in response headers, the body, or
  both (`keyword_scope`: `body`, `headers`, `both`). This is useful for
  subscription endpoints that expose the profile name in headers.
- Bumped the PWA cache and asset query versions so the new monitor form is
  loaded after an update.

## 1.6.3

- Fixed `pulse update` rejecting a freshly downloaded release. The binary run
  from the updater's temporary file printed that file name (for example
  `.pulse-update-123 1.6.2`) instead of `pulse 1.6.2`, so the version check
  failed. `--version` now always reports `pulse` (or `pulse-agent` when invoked
  through the agent applet), independent of the file name.
- Added a regression test for the updater's version output.

## 1.6.2

- Fixed a proxy monitoring bug where one dead endpoint in a subscription could
  consume the entire fast TCP probe phase. All real Xray tests then started
  with an expired context and were skipped, leaving every node on TCP. Each
  fast probe is now capped at 6 seconds.
- Proxy check timeouts are now sized from `max_real_nodes` (up to 3 minutes)
  instead of a fixed 45 seconds, and the scheduler/agent allow the same window.
  This prevents the last nodes of a large subscription from being skipped.
- Added XHTTP/SplitHTTP transport support to real Xray tests
  (`type=xhttp`/`splithttp`, with path, host and mode).

## 1.6.1

- The installer (panel and agent) now waits for the apt/dpkg lock instead of
  aborting when `unattended-upgrades`, `cloud-init` or another install is
  running. Locks are waited on for up to 10 minutes and failed package
  operations are retried; a clear error is printed only if apt is really broken.
- The panel installer verifies the Caddy version and installs Caddy 2.6+ from
  the official repository when the distribution package is too old. If the
  installed Caddy still rejects the `h2c` transport, the installer now writes a
  WebSocket-compatible site automatically, so automatic HTTPS keeps working and
  agents use the WebSocket/REST fallbacks.
- Added Ubuntu 20.04 and Debian 11 to the supported install targets; the older
  distro Caddy package no longer blocks the installation.

## 1.6.0

- Added a full **English** UI with an EN/FA toggle, automatic LTR layout,
  English dates/numbers, and a CSP-safe translation layer. The choice is stored
  in the browser.
- Added a dedicated **Logs** page and a persistent SQLite log store. Logs are
  split into panel, scheduler, monitor, Xray, Cloudflare, notifications, agent,
  security and API categories, with level/search filters and automatic refresh.
  `pulse logs` can read the same database with category/level/follow filters.
- Unified the panel and agent CLI: `start`, `stop`, `restart`, `status`, `logs`,
  `update`, `uninstall` (aliases `remove`/`delete`), `version`, and
  `pulse port` for changing the panel listen port.
- Added `--port` and `PULSE_PORT` to the panel binary, plus `--behind-proxy` in
  the installer for nodes that already run Xray/VPN panels and a reverse proxy.
- Isolated Xray completely: the installer keeps a Pulse-owned copy at
  `/usr/local/lib/pulse/xray` and never overwrites `/usr/local/bin/xray` or
  restarts an existing Xray service. Temporary test cores use a random loopback
  port and their own working directory, and only their own process is stopped.
- Made mobile modals lighter and safer: shorter sheet animation, no heavy
  shadow/blur, scroll containment, and no backdrop-click close on phones.
- Added store/CLI tests for application logs and port rewriting.

## 1.5.0

- Agents now keep a persistent bidirectional link: gRPC first, WebSocket as a
  reverse-proxy fallback, and REST as the final fallback. Hardware telemetry is
  pushed every second and the link state is reflected immediately in the UI.
- Added **test from agent** for every monitor type. A monitor can select an
  agent as its probe location, so proxy handshakes, TCP, ICMP, HTTP, DNS and
  SSL checks run from that server's network instead of the panel host.
- A missing or offline probe agent is reported as a degraded probe failure
  instead of opening a false incident for the target.
- Added `probe_agent_id` with an automatic database migration, agent-aware
  monitor cards/details, and a probe selector in the monitor form.
- Agents advertise check capability and Xray availability over the live link;
  the UI shows gRPC/WebSocket/REST transport and warns when an agent cannot run
  proxy handshakes.
- The agent installer now installs the official Xray release by default for
  remote proxy tests (`--no-xray` to skip it).
- The Caddy installer exposes the gRPC path over h2c while keeping normal
  UI/SSE traffic and the WebSocket fallback on HTTP/1.1.
- Agent deletion clears monitors that referenced that probe; agent token
  rotation drops the old live link immediately.
- Added gRPC, WebSocket and remote-check integration tests.
- Fixed the monitor form's type picker overflowing its modal at narrow tablet
  widths (the grid items now shrink and wrap correctly).

## 1.4.4

- Header is now full-bleed: page gutters moved from the main column to the
  header/view/footer, so no empty strips remain between the header, sidebar
  and viewport edges on any page.
- The dashboard gauge glow no longer has a rectangular edge. The drop shadow
  moved from the clipped SVG shape to the SVG element, and the ambient halo
  is a wide radial gradient that reaches transparent before its box ends.
- Replaced the generic "node" wording with context-appropriate terms:
  proxy **configs** in the UI, API messages and README.
- Slightly enlarged small icons across navigation, buttons, chips, stat cards
  and detail panels for better legibility.
- Fixed the 24-hour trend panel growing without bound: the canvas wrapper now
  has a definite height, breaking the ResizeObserver/canvas feedback loop.
- The dashboard server-health panel now refreshes when an agent comes online
  or a new agent is added while the dashboard is open, so it no longer
  disagrees with the "servers online" summary chip.
- Version bump so `pulse update` picks up the release.

## 1.4.3

- Fixed the root cause of transparent-looking modals: `#modal-root` was
  missing its `modal-root` class, so it had no stacking layer above the app.
  Page cards were painting over modal dialogs. The root now has the proper
  z-index and pointer-event handling.
- Removed the whole-dialog opacity fade and backdrop blur that made the
  problem much more visible in screenshots; dialogs now have an opaque
  surface and only a short position animation.
- Version bump so the VPS can pull the fix with `pulse update`.

## 1.4.2

- Full UI audit and bug-fix pass: oversized icons, light-theme hero/tooltips,
  tablet hero overlap, mobile delete actions and DNS columns, and light-theme
  danger/soft buttons.
- Fixed route/detail refresh races, stale detail modals, modal/confirm cleanup,
  focus handling, keyboard card activation, and SSE reconnect refreshes.
- Cloudflare disabled state can now be re-enabled without re-entering the token;
  zones/records failures and out-of-order zone changes show correct state.
- Preserved unsaved Telegram/Webhook/Settings drafts while saving sibling forms;
  test notifications refresh history.
- Badge counts now load outside the dashboard; degraded monitors are counted
  correctly; active incident badges update immediately.
- Live agent detail modal updates over SSE; agent cards/gauges continue to patch
  in place at 1-second intervals.
- Service worker caching is safer and cache names are aligned with the app version.

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
