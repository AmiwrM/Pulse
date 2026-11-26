#!/usr/bin/env bash
# =============================================================================
# Pulse Server Agent Installer — Debian / Ubuntu (amd64 / arm64)
#
# Installs the Pulse binary in agent mode on a monitored VPS. The binary is
# downloaded from the official GitHub release of the same repository.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/AmiwrM/Pulse/main/deploy/install-agent.sh -o /tmp/install-agent.sh
#   sudo bash /tmp/install-agent.sh \
#     --server https://status.example.com \
#     --token TOKEN \
#     --name vps-1
#
# Optional:
#   --release v1.6.0      Release tag (default: latest)
#   --binary PATH         Use a local pulse-linux-ARCH binary
#   --binary-url URL      Download the binary from a custom URL
#   --services xray,wg    systemd services to report
#   --no-wg               Disable WireGuard collection
#   --xray-version VER    Xray release for remote proxy checks (default: v26.11.9)
#   --no-xray             Do not install Xray on this agent
#
# Run as root on Debian/Ubuntu.
# =============================================================================
set -Eeuo pipefail

INSTALLER_VERSION="1.6.0"
REPO_SLUG="${PULSE_REPO:-AmiwrM/Pulse}"
REPO_URL="https://github.com/${REPO_SLUG}"

SERVER=""
TOKEN=""
NAME="$(hostname)"
BINARY=""
BINARY_URL="${PULSE_AGENT_BINARY_URL:-}"
RELEASE="${PULSE_VERSION:-latest}"
SERVICES=""
WG=1
AGENT_INTERVAL="1s"
INSTALL_XRAY=1
FORCE_XRAY=0
XRAY_VERSION="${XRAY_VERSION:-v26.11.9}"
XRAY_DOWNLOAD_URL="${XRAY_DOWNLOAD_URL:-}"
XRAY_OK=0
XRAY_PATH=""
XRAY_ASSET=""
PULSE_XRAY_PATH="${PULSE_XRAY_PATH:-/usr/local/lib/pulse/xray}"

log()  { printf '\033[1;36m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m[!]\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[1;31m[x]\033[0m %s\n' "$*" >&2; exit 1; }

usage() {
  cat <<HELP
Pulse Server Agent Installer ${INSTALLER_VERSION}

Usage:
  sudo bash install-agent.sh --server https://status.example.com --token TOKEN --name vps-1

Options:
  --server URL          Pulse public URL (required)
  --token TOKEN         Agent token from the panel (required)
  --name NAME           Display name (default: hostname)
  --release TAG         GitHub release tag (default: latest)
  --binary PATH         Use a local pulse-linux-ARCH binary
  --binary-url URL      Download the binary from a custom URL
  --services LIST       Comma-separated systemd services (e.g. xray,wireguard)
  --no-wg               Disable WireGuard collection
  --interval DURATION   Report interval (default: 1s)
  --xray-version VER    Xray release for remote proxy checks (default: v26.11.9)
  --no-xray             Do not install Xray on this agent
  --force-xray          Reinstall the Pulse-owned Xray copy (system Xray untouched)
  -h, --help            Show this help
HELP
  exit 0
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --server) SERVER="$2"; shift 2 ;;
    --token) TOKEN="$2"; shift 2 ;;
    --name) NAME="$2"; shift 2 ;;
    --release) RELEASE="$2"; shift 2 ;;
    --binary) BINARY="$2"; shift 2 ;;
    --binary-url) BINARY_URL="$2"; shift 2 ;;
    --services) SERVICES="$2"; shift 2 ;;
    --no-wg) WG=0; shift ;;
    --interval) AGENT_INTERVAL="$2"; shift 2 ;;
    --xray-version) XRAY_VERSION="$2"; FORCE_XRAY=1; shift 2 ;;
    --no-xray) INSTALL_XRAY=0; shift ;;
    --force-xray) FORCE_XRAY=1; shift ;;
    -h|--help) usage ;;
    *) die "Unknown option: $1 (try --help)" ;;
  esac
done

[[ ${EUID} -eq 0 ]] || die "Please run as root: sudo bash $0"
[[ -n "$SERVER" ]] || die "--server is required."
[[ -n "$TOKEN" ]] || die "--token is required."
[[ "$SERVER" == http://* || "$SERVER" == https://* ]] || die "--server must start with http:// or https://"
SERVER="${SERVER%/}"
[[ -n "$NAME" && ${#NAME} -le 80 && "$NAME" != *$'\n'* && "$NAME" != *$'\r'* ]] || die "Invalid --name."
[[ "$AGENT_INTERVAL" =~ ^[0-9]+(s|m|h)$ ]] || die "Invalid --interval (e.g. 1s, 1m)."
[[ "$RELEASE" == "latest" || "$RELEASE" == v* ]] || RELEASE="v$RELEASE"
[[ "$XRAY_VERSION" == "latest" || "$XRAY_VERSION" == v* ]] || XRAY_VERSION="v$XRAY_VERSION"

ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64)  ARCH="amd64"; XRAY_ASSET="Xray-linux-64.zip" ;;
  aarch64|arm64) ARCH="arm64"; XRAY_ASSET="Xray-linux-arm64-v8a.zip" ;;
  *) die "Unsupported architecture: $ARCH (amd64/arm64 only)" ;;
esac

command -v apt-get >/dev/null 2>&1 || die "This installer supports Debian/Ubuntu (apt-get not found)."
log "Installing system dependencies..."
apt-get update -qq
apt-get install -y -qq ca-certificates curl unzip tar gzip coreutils iputils-ping >/dev/null

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

RELEASE_ASSET=""
release_url() {
  local asset="$1"
  if [[ "$RELEASE" == "latest" ]]; then
    printf '%s/releases/latest/download/%s' "$REPO_URL" "$asset"
  else
    printf '%s/releases/download/%s/%s' "$REPO_URL" "$RELEASE" "$asset"
  fi
}

if [[ -z "$BINARY" ]]; then
  if [[ -z "$BINARY_URL" ]]; then
    RELEASE_ASSET="pulse-linux-$ARCH"
    BINARY_URL="$(release_url "$RELEASE_ASSET")"
  fi
  download_path="$TMP/pulse"
  [[ -n "$RELEASE_ASSET" ]] && download_path="$TMP/$RELEASE_ASSET"
  log "Downloading agent binary: $BINARY_URL"
  command -v curl >/dev/null 2>&1 || die "curl is required."
  curl -fsSL --retry 4 --retry-delay 2 "$BINARY_URL" -o "$download_path" || die "Could not download the agent binary. Use --binary or --binary-url."

  if [[ -n "$RELEASE_ASSET" ]]; then
    sum_url="$(release_url SHA256SUMS)"
    if curl -fsSL --retry 2 "$sum_url" -o "$TMP/SHA256SUMS" 2>/dev/null; then
      sum_line="$(grep " $RELEASE_ASSET\$" "$TMP/SHA256SUMS" || true)"
      if [[ -n "$sum_line" ]]; then
        ( cd "$TMP" && printf '%s\n' "$sum_line" | sha256sum -c - >/dev/null ) \
          || die "Agent binary checksum verification failed."
        log "Release checksum verified."
      fi
    fi
  fi
  BINARY="$download_path"
fi

[[ -f "$BINARY" ]] || die "Binary not found: $BINARY"
chmod 0755 "$BINARY"
VERSION_OUT="$("$BINARY" --version 2>/dev/null | sed -n '1p' || true)"
[[ "$VERSION_OUT" == pulse* ]] || die "The selected file is not a runnable Pulse Linux binary for $ARCH."

install -m 0755 "$BINARY" /usr/local/bin/pulse
ln -sfn /usr/local/bin/pulse /usr/local/bin/pulse-agent
ln -sfn /usr/local/bin/pulse /usr/local/bin/pulse-a
install -d -m 0750 /etc/pulse

install_xray() {
  use_xray() {
    XRAY_PATH="$1"
    if [[ -x "$XRAY_PATH" ]] && "$XRAY_PATH" version >/dev/null 2>&1; then
      XRAY_OK=1
      log "Using Xray $("$XRAY_PATH" version 2>/dev/null | sed -n '1p') at $XRAY_PATH"
      return 0
    fi
    return 1
  }

  if [[ -n "${PULSE_XRAY_BIN:-}" ]]; then
    if use_xray "$PULSE_XRAY_BIN"; then
      return 0
    fi
    warn "PULSE_XRAY_BIN=$PULSE_XRAY_BIN is not a runnable Xray binary; continuing with auto-detection."
  fi

  if [[ "$INSTALL_XRAY" -eq 0 ]]; then
    if use_xray "$PULSE_XRAY_PATH"; then
      return 0
    fi
    if command -v xray >/dev/null 2>&1 && use_xray "$(command -v xray)"; then
      warn "Xray installation disabled; using the existing system Xray for temporary loopback checks."
    fi
    return 0
  fi

  if [[ "$FORCE_XRAY" -eq 0 ]] && use_xray "$PULSE_XRAY_PATH"; then
    return 0
  fi

  local url
  if [[ "$XRAY_VERSION" == "latest" ]]; then
    url="${XRAY_DOWNLOAD_URL:-https://github.com/XTLS/Xray-core/releases/latest/download/${XRAY_ASSET}}"
  else
    url="${XRAY_DOWNLOAD_URL:-https://github.com/XTLS/Xray-core/releases/download/${XRAY_VERSION}/${XRAY_ASSET}}"
  fi
  log "Installing a Pulse-owned Xray ${XRAY_VERSION} copy (system Xray stays untouched)..."
  if ! curl -fsSL --retry 4 --retry-delay 2 "$url" -o "$TMP/xray.zip"; then
    if command -v xray >/dev/null 2>&1 && use_xray "$(command -v xray)"; then
      warn "Xray download failed; using the existing system Xray for temporary loopback checks."
      return 0
    fi
    warn "Could not download Xray. The agent will still report hardware metrics, but proxy handshakes from this server will be skipped."
    return 0
  fi
  if ! unzip -q "$TMP/xray.zip" xray -d "$TMP" 2>/dev/null; then
    unzip -q "$TMP/xray.zip" -d "$TMP" || return 0
  fi
  [[ -f "$TMP/xray" ]] || { warn "Xray archive did not contain the xray binary."; return 0; }
  install -d -m 0755 /usr/local/lib/pulse
  install -m 0755 "$TMP/xray" "$PULSE_XRAY_PATH"
  if ! use_xray "$PULSE_XRAY_PATH"; then
    warn "The Pulse Xray copy is not runnable on this server."
    XRAY_PATH=""
  fi
}
install_xray

env_escape() {
  local s="$1"
  s="${s//\\/\\\\}"
  s="${s//\"/\\\"}"
  s="${s//$'\n'/}"
  s="${s//$'\r'/}"
  printf '%s' "$s"
}

umask 077
{
  printf 'PULSE_SERVER="%s"\n' "$(env_escape "$SERVER")"
  printf 'PULSE_TOKEN="%s"\n' "$(env_escape "$TOKEN")"
  printf 'PULSE_AGENT_NAME="%s"\n' "$(env_escape "$NAME")"
  if [[ -n "$SERVICES" ]]; then
    printf 'PULSE_AGENT_SERVICES="%s"\n' "$(env_escape "$SERVICES")"
  fi
  if [[ "$XRAY_OK" -eq 1 && -n "$XRAY_PATH" ]]; then
    printf 'PULSE_XRAY_BIN="%s"\n' "$(env_escape "$XRAY_PATH")"
  fi
} >/etc/pulse/agent.env
chmod 0600 /etc/pulse/agent.env
umask 022

WG_FLAG=""
[[ "$WG" -eq 0 ]] && WG_FLAG=" --wg=false"

if command -v curl >/dev/null 2>&1; then
  hello="$(curl -fsS --max-time 10 -H "X-Agent-Token: $TOKEN" -H "Accept: application/json" \
    "$SERVER/api/agent/hello" 2>/dev/null || true)"
  if [[ "$hello" == *'"ok":true'* ]]; then
    log "Server handshake verified."
  else
    warn "The server handshake could not be verified now; the agent will retry automatically."
  fi
fi

cat >/etc/systemd/system/pulse-agent.service <<UNIT
[Unit]
Description=Pulse server agent
Documentation=${REPO_URL}
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=root
EnvironmentFile=/etc/pulse/agent.env
ExecStart=/usr/local/bin/pulse --agent --interval $AGENT_INTERVAL$WG_FLAG
Restart=always
RestartSec=5
TimeoutStopSec=20
LimitNOFILE=65536
NoNewPrivileges=true
ProtectSystem=full
ProtectHome=true

[Install]
WantedBy=multi-user.target
UNIT
chmod 0600 /etc/systemd/system/pulse-agent.service

systemctl daemon-reload
systemctl enable pulse-agent >/dev/null 2>&1 || true
systemctl restart pulse-agent || true
sleep 1
if ! systemctl is-active pulse-agent >/dev/null 2>&1; then
  journalctl -u pulse-agent -n 80 --no-pager || true
  die "pulse-agent did not start. Check: journalctl -u pulse-agent -n 100 --no-pager"
fi

echo
echo "============================================================"
echo " Pulse agent installed"
echo "============================================================"
echo " Version : ${VERSION_OUT#pulse }"
echo " Server  : $SERVER"
echo " Name    : $NAME"
echo " Xray    : ${XRAY_PATH:-not installed (proxy handshakes disabled)}"
echo " Config  : /etc/pulse/agent.env (mode 600)"
echo " Service : systemctl status pulse-agent --no-pager"
echo " Logs    : journalctl -u pulse-agent -f"
echo
echo " Manage  :"
echo "   pulse-agent status"
echo "   pulse-agent restart"
echo "   pulse-agent logs"
echo "   pulse-agent update"
echo "   (pulse-a is an alias for pulse-agent)"
echo "============================================================"
