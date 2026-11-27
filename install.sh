#!/usr/bin/env bash
# =============================================================================
# Pulse VPS Installer — Debian / Ubuntu (amd64 / arm64)
#
# One command:
#   curl -fsSL https://raw.githubusercontent.com/AmiwrM/Pulse/main/install.sh | sudo bash
#
# Interactive SSL:
#   curl -fsSL .../install.sh -o /tmp/pulse-install.sh
#   sudo bash /tmp/pulse-install.sh
#
# Sources (all downloaded on the server):
#   Pulse binary .... GitHub Releases (AmiwrM/Pulse); source build fallback
#   Go toolchain .... https://go.dev/dl
#   Xray core ....... https://github.com/XTLS/Xray-core/releases
#   Caddy ........... official Caddy apt repository
#
# Supported: Debian 11 / 12, Ubuntu 20.04 / 22.04 / 24.04. Run as root.
# =============================================================================
set -Eeuo pipefail

INSTALLER_VERSION="1.7.0"
REPO_SLUG="${PULSE_REPO:-AmiwrM/Pulse}"
BRANCH="${PULSE_BRANCH:-main}"
REPO_URL="https://github.com/${REPO_SLUG}"

PORT="${PULSE_PORT:-8080}"
DOMAIN="${DOMAIN:-}"
EMAIL="${EMAIL:-}"
USE_SSL="ask"
ASSUME_YES=0
BEHIND_PROXY_URL="${BEHIND_PROXY_URL:-}"
BINARY_PATH="${PULSE_BINARY_PATH:-}"
BINARY_URL="${PULSE_BINARY_URL:-}"
PULSE_RELEASE="${PULSE_VERSION:-latest}"
FORCE_SOURCE=0
ADMIN_USER="${PULSE_ADMIN_USER:-admin}"
ADMIN_PASS="${PULSE_ADMIN_PASSWORD:-}"
NO_ADMIN=0
INSTALL_XRAY=1
FORCE_XRAY=0
XRAY_VERSION="${XRAY_VERSION:-v26.11.9}"
XRAY_DOWNLOAD_URL="${XRAY_DOWNLOAD_URL:-}"
XRAY_OK=0
XRAY_PATH=""
PULSE_XRAY_PATH="${PULSE_XRAY_PATH:-/usr/local/lib/pulse/xray}"
SSL_OK=0
DNS_OK=1
PUBLIC_BASE_URL=""
PULSE_SRC=""
PULSE_SRC_KIND=""

log()  { printf '\033[1;36m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m[!]\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[1;31m[x]\033[0m %s\n' "$*" >&2; exit 1; }

# apt/dpkg can be busy with unattended-upgrades, cloud-init or another install.
# Wait for the lock instead of failing with "Could not get lock".
apt_get() {
  local attempt
  for attempt in 1 2 3; do
    if DEBIAN_FRONTEND=noninteractive apt-get \
      -o DPkg::Lock::Timeout=180 \
      -o Acquire::Retries=3 \
      "$@"; then
      return 0
    fi
    warn "apt-get $* failed (attempt ${attempt}/3); another package manager may be running. Retrying in 5s..."
    sleep 5
  done
  return 1
}

usage() {
  cat <<HELP
Pulse VPS Installer ${INSTALLER_VERSION}

Usage:
  sudo bash install.sh
  sudo bash install.sh --domain status.example.com --email you@example.com --yes
  sudo bash install.sh --no-ssl --yes

Options:
  --domain DOMAIN        Enable automatic HTTPS with Caddy for this domain
  --email EMAIL          Let's Encrypt account email (optional)
  --no-ssl               Install without HTTPS (direct HTTP on --port)
  --yes, -y              Non-interactive; use provided flags and defaults
  --release VERSION      Install a specific GitHub release tag (default: latest)
  --source               Always build Pulse from source instead of release binary
  --branch NAME          Source branch when building (default: main)
  --binary PATH          Use a local pulse-linux-ARCH binary
  --binary-url URL       Download the Pulse binary from a URL
  --admin-user NAME      Admin username (default: admin)
  --admin-pass PASS      Admin password (default: generated random password)
  --no-admin             Do not create the admin account automatically
  --port PORT            Pulse HTTP port (default: 8080)
--behind-proxy URL     Bind on loopback and use an existing reverse proxy (no Caddy/443)
  --xray-version VERSION Xray version, or "latest" (default: v26.11.9)
  --no-xray              Do not install Xray
  --force-xray           Reinstall the Pulse-owned Xray copy (system Xray untouched)
  -h, --help             Show this help
  --installer-version    Show installer version

Environment overrides:
  PULSE_REPO, PULSE_BRANCH, PULSE_VERSION, PULSE_PORT,
  PULSE_BINARY_URL, PULSE_BINARY_PATH, XRAY_VERSION, XRAY_DOWNLOAD_URL,
  GOPROXY, GO_BIN, GO_DOWNLOAD_URL

Pulse release assets (built by .github/workflows/release.yml):
  pulse-linux-amd64, pulse-linux-arm64, SHA256SUMS
HELP
  exit 0
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --domain) DOMAIN="$2"; USE_SSL="yes"; shift 2 ;;
    --email) EMAIL="$2"; shift 2 ;;
    --no-ssl) USE_SSL="no"; shift ;;
    --yes|-y) ASSUME_YES=1; shift ;;
    --release) PULSE_RELEASE="$2"; shift 2 ;;
    --source) FORCE_SOURCE=1; shift ;;
    --branch) BRANCH="$2"; shift 2 ;;
    --binary) BINARY_PATH="$2"; shift 2 ;;
    --binary-url) BINARY_URL="$2"; shift 2 ;;
    --admin-user) ADMIN_USER="$2"; shift 2 ;;
    --admin-pass) ADMIN_PASS="$2"; shift 2 ;;
    --no-admin) NO_ADMIN=1; shift ;;
    --port) PORT="$2"; shift 2 ;;
--behind-proxy) BEHIND_PROXY_URL="$2"; shift 2 ;;
    --xray-version) XRAY_VERSION="$2"; FORCE_XRAY=1; shift 2 ;;
    --no-xray) INSTALL_XRAY=0; shift ;;
    --force-xray) FORCE_XRAY=1; shift ;;
    -h|--help) usage ;;
    --installer-version) echo "pulse-installer $INSTALLER_VERSION"; exit 0 ;;
    *) die "Unknown option: $1 (try --help)" ;;
  esac
done

[[ ${EUID} -eq 0 ]] || die "Please run as root: sudo bash $0"
command -v apt-get >/dev/null 2>&1 || die "This installer supports Debian/Ubuntu (apt-get not found)."
[[ -d /run/systemd/system ]] || die "systemd is required (this does not look like a booted VPS)."
if [[ ! "$PORT" =~ ^[0-9]+$ ]] || (( PORT <= 0 || PORT >= 65536 )); then
  die "Invalid --port"
fi
[[ "$ADMIN_USER" =~ ^[A-Za-z0-9_.-]{3,32}$ ]] || die "Admin username must be 3-32 chars: A-Z a-z 0-9 _ . -"
[[ -z "$ADMIN_PASS" || ${#ADMIN_PASS} -ge 8 ]] || die "--admin-pass must be at least 8 characters."

# TTY access is kept separately so `curl ... | sudo bash` can still prompt.
TTY_DEV=""
if [[ -r /dev/tty && -w /dev/tty ]]; then
  TTY_DEV="/dev/tty"
elif [[ -t 0 ]]; then
  TTY_DEV="/dev/stdin"
fi
ask() {
  local prompt="$1" answer=""
  if [[ -n "$TTY_DEV" ]]; then
    read -r -p "$prompt" answer <"$TTY_DEV" || true
  fi
  printf '%s' "$answer"
}

# ------------------------------------------------------------------ prompts
if [[ -n "$BEHIND_PROXY_URL" ]]; then
  [[ "$BEHIND_PROXY_URL" == http://* || "$BEHIND_PROXY_URL" == https://* ]] || die "--behind-proxy must be an http:// or https:// URL"
  USE_SSL="no"
  DOMAIN=""
fi
if [[ -n "$DOMAIN" && "$USE_SSL" == "ask" ]]; then
  USE_SSL="yes"
fi
if [[ "$USE_SSL" == "ask" ]]; then
  if [[ -n "$TTY_DEV" && "$ASSUME_YES" -eq 0 ]]; then
    answer="$(ask "Enable automatic HTTPS/SSL with a domain? [y/N] ")"
    if [[ "$answer" =~ ^[Yy]$ ]]; then
      USE_SSL="yes"
      DOMAIN="$(ask "Domain (e.g. status.example.com): ")"
      EMAIL="$(ask "Email for Let's Encrypt (optional): ")"
    else
      USE_SSL="no"
    fi
  else
    USE_SSL="no"
  fi
fi
[[ "$USE_SSL" == "yes" ]] && [[ -z "$DOMAIN" ]] && die "SSL requested but no domain was supplied."

DOMAIN="${DOMAIN#http://}"; DOMAIN="${DOMAIN#https://}"; DOMAIN="${DOMAIN%%/*}"
if [[ -n "$DOMAIN" && ! "$DOMAIN" =~ ^[A-Za-z0-9]([A-Za-z0-9.-]*[A-Za-z0-9])?\.[A-Za-z]{2,}$ ]]; then
  die "Invalid domain: $DOMAIN"
fi
if [[ -n "$EMAIL" && ! "$EMAIL" =~ ^[^@[:space:]]+@[^@[:space:]]+\.[^@[:space:]]+$ ]]; then
  warn "Invalid email '$EMAIL'; continuing without an ACME email."
  EMAIL=""
fi
[[ "$PULSE_RELEASE" == "latest" || "$PULSE_RELEASE" == v* ]] || PULSE_RELEASE="v$PULSE_RELEASE"
[[ "$XRAY_VERSION" == "latest" || "$XRAY_VERSION" == v* ]] || XRAY_VERSION="v$XRAY_VERSION"

export DEBIAN_FRONTEND=noninteractive
umask 022
TMPDIR_INSTALL="$(mktemp -d)"
trap 'rm -rf "$TMPDIR_INSTALL"' EXIT

# ------------------------------------------------------------------ base info
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64)  ARCH="amd64"; XRAY_ASSET="Xray-linux-64.zip" ;;
  aarch64|arm64) ARCH="arm64"; XRAY_ASSET="Xray-linux-arm64-v8a.zip" ;;
  *) die "Unsupported architecture: $ARCH (amd64/arm64 only)" ;;
esac

log "Installing system dependencies (waiting for any apt/dpkg lock)..."
apt_get update -qq \
  || die "apt-get update failed after retries. Run 'sudo apt-get update', fix the error it prints, then re-run this installer."
apt_get install -y -qq ca-certificates curl unzip tar gzip coreutils iputils-ping openssl >/dev/null \
  || die "Could not install system packages. Run 'sudo apt-get -f install', then re-run this installer."

get_public_ip() {
  local ip=""
  for u in https://api.ipify.org https://ifconfig.me/ip https://icanhazip.com; do
    ip="$(curl -4 -fsS --max-time 8 "$u" 2>/dev/null | tr -d '[:space:]' || true)"
    if [[ "$ip" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then printf '%s' "$ip"; return; fi
  done
  hostname -I 2>/dev/null | awk '{print $1}'
}
PUBLIC_IP="$(get_public_ip || true)"

open_firewall() {
  local status
  if command -v ufw >/dev/null 2>&1; then
    status="$(ufw status 2>/dev/null || true)"
    if [[ "$status" == *"Status: active"* ]]; then
      if [[ "$USE_SSL" == "yes" ]]; then
        ufw allow 80/tcp >/dev/null || true
        ufw allow 443/tcp >/dev/null || true
      else
        ufw allow "$PORT/tcp" >/dev/null || true
      fi
      log "UFW rules updated."
    fi
  fi
  if command -v firewall-cmd >/dev/null 2>&1 && firewall-cmd --state >/dev/null 2>&1; then
    if [[ "$USE_SSL" == "yes" ]]; then
      firewall-cmd --permanent --add-service=http >/dev/null || true
      firewall-cmd --permanent --add-service=https >/dev/null || true
    else
      firewall-cmd --permanent --add-port="$PORT/tcp" >/dev/null || true
    fi
    firewall-cmd --reload >/dev/null || true
  fi
}

# ------------------------------------------------------------------ DNS check
if [[ "$USE_SSL" == "yes" ]]; then
  log "Checking DNS for $DOMAIN..."
  mapfile -t RESOLVED < <(getent ahostsv4 "$DOMAIN" 2>/dev/null | awk '{print $1}' | sort -u || true)
  DNS_OK=1
  if (( ${#RESOLVED[@]} == 0 )); then
    warn "DNS for $DOMAIN is not resolved yet. Caddy will retry automatically."
    DNS_OK=0
  elif [[ -n "$PUBLIC_IP" ]] && ! printf '%s\n' "${RESOLVED[@]}" | grep -qx "$PUBLIC_IP"; then
    warn "DNS points to ${RESOLVED[*]}, but this server's public IP appears to be $PUBLIC_IP."
    DNS_OK=0
  fi
  if [[ "$DNS_OK" -eq 0 && "$ASSUME_YES" -eq 0 && -n "$TTY_DEV" ]]; then
    go="$(ask "Continue with HTTPS anyway? [Y/n] ")"
    [[ "$go" =~ ^[Nn]$ ]] && USE_SSL="no"
  fi
fi

# ------------------------------------------------------------------ Caddy
caddy_version_ok() {
  local raw ver major minor
  raw="$(caddy version 2>/dev/null | awk '{print $1}')"
  [[ "$raw" == v2.* ]] || return 1
  ver="${raw#v}"
  IFS=. read -r major minor _ <<<"$ver"
  [[ "$major" =~ ^[0-9]+$ && "$minor" =~ ^[0-9]+$ ]] || return 1
  (( major > 2 || (major == 2 && minor >= 6) ))
}

install_caddy() {
  local need_upgrade=0
  if command -v caddy >/dev/null 2>&1 && systemctl cat caddy.service >/dev/null 2>&1; then
    if caddy_version_ok; then
      return 0
    fi
    warn "Installed Caddy $(caddy version 2>/dev/null | awk '{print $1}') is older than 2.6; trying the official repository..."
    need_upgrade=1
  else
    log "Installing Caddy from the distribution repository..."
    if apt_get install -y -qq caddy >/dev/null 2>&1; then
      if caddy_version_ok; then
        return 0
      fi
      warn "The distribution Caddy is older than 2.6; trying the official repository..."
      need_upgrade=1
    else
      need_upgrade=1
    fi
  fi

  if [[ "$need_upgrade" -eq 1 ]]; then
    warn "Adding the official Caddy repository for Caddy 2.6+..."
    if apt_get install -y -qq debian-keyring debian-archive-keyring apt-transport-https gpg >/dev/null 2>&1 \
      && curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' \
        | gpg --dearmor --yes -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg \
      && curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' \
        > /etc/apt/sources.list.d/caddy-stable.list \
      && apt_get update -qq \
      && apt_get install -y -qq caddy >/dev/null 2>&1; then
      if caddy_version_ok; then
        return 0
      fi
      warn "Caddy was installed from the official repository but is still older than 2.6."
    else
      warn "Could not install Caddy 2.6+ from the official repository."
    fi
  fi

  # An older Caddy 2.x can still serve Pulse over HTTPS; the installer then
  # writes a WebSocket-compatible config and skips the h2c/gRPC transport.
  if command -v caddy >/dev/null 2>&1 \
    && caddy version 2>/dev/null | awk '{print $1}' | grep -q '^v2\.' \
    && systemctl cat caddy.service >/dev/null 2>&1; then
    warn "Continuing with Caddy $(caddy version 2>/dev/null | awk '{print $1}'); the agent link will use WebSocket/REST."
    return 0
  fi
  return 1
}

# Write the Pulse Caddy site. With h2c enabled, Caddy proxies the agent gRPC
# path to Pulse over cleartext HTTP/2; without it, the agent falls back to
# WebSocket and REST, which works on every Caddy 2.x release.
write_pulse_caddy() {
  local grpc_h2c="$1"
  {
    printf '%s {\n' "$DOMAIN"
    printf '\tencode gzip\n'
    if [[ "$grpc_h2c" -eq 1 ]]; then
      printf '\t@pulse_grpc {\n'
      printf '\t\tpath /pulse.agent.v1.AgentLink/*\n'
      printf '\t\theader Content-Type application/grpc*\n'
      printf '\t}\n'
      printf '\thandle @pulse_grpc {\n'
      printf '\t\treverse_proxy 127.0.0.1:%s {\n' "$PORT"
      printf '\t\t\ttransport http {\n\t\t\t\tversions h2c 2\n\t\t\t}\n'
      printf '\t\t}\n\t}\n'
    fi
    printf '\thandle {\n'
    printf '\t\treverse_proxy 127.0.0.1:%s {\n\t\t\tflush_interval -1\n\t\t}\n' "$PORT"
    printf '\t}\n'
    printf '\theader {\n\t\t-Server\n\t}\n'
    printf '}\n'
  } >"$PULSE_CADDY"
  chmod 0644 "$PULSE_CADDY"
}

if [[ "$USE_SSL" == "yes" ]]; then
  if ! install_caddy; then
    if [[ -n "$TTY_DEV" && "$ASSUME_YES" -eq 0 ]]; then
      answer="$(ask "Caddy installation failed. Continue without HTTPS? [y/N] ")"
      if [[ "$answer" =~ ^[Yy]$ ]]; then
        USE_SSL="no"
        warn "Continuing without HTTPS. Do not expose this panel to the internet without a TLS proxy."
      else
        die "Caddy installation failed. Fix the network/repositories or re-run with --no-ssl."
      fi
    else
      die "Caddy installation failed. Fix the network/repositories or re-run with --no-ssl."
    fi
  else
    log "Caddy is ready."
  fi
fi

# ------------------------------------------------------------------ Go / source build
version_ge() {
  [[ "$(printf '%s\n' "$2" "$1" | sort -V | sed -n '1p')" == "$2" ]]
}
go_version_of() {
  "$1" version 2>/dev/null | awk '{print $3}' | sed 's/^go//'
}
go_is_usable() {
  local bin="$1" ver
  [[ -n "$bin" && -x "$bin" ]] || return 1
  ver="$(go_version_of "$bin" || true)"
  [[ -n "$ver" ]] && version_ge "$ver" "1.26.0"
}

get_go() {
  local candidate="${GO_BIN:-}"
  if go_is_usable "$candidate"; then printf '%s' "$candidate"; return 0; fi
  candidate="$(command -v go 2>/dev/null || true)"
  if go_is_usable "$candidate"; then printf '%s' "$candidate"; return 0; fi

  local latest url tarball
  log "Go 1.26+ was not found; downloading the official Go toolchain..." >&2
  latest="$(curl -fsSL --retry 3 --max-time 30 'https://go.dev/VERSION?m=text' 2>/dev/null | sed -n '1p' || true)"
  [[ "$latest" =~ ^go[0-9]+\.[0-9]+(\.[0-9]+)?$ ]] || return 1
  version_ge "${latest#go}" "1.26.0" || return 1
  tarball="${latest}.linux-${ARCH}.tar.gz"
  url="${GO_DOWNLOAD_URL:-https://go.dev/dl/${tarball}}"
  curl -fsSL --retry 5 --retry-delay 2 --max-time 600 "$url" -o "$TMPDIR_INSTALL/go.tar.gz" || return 1
  mkdir -p "$TMPDIR_INSTALL/go-toolchain"
  tar -xzf "$TMPDIR_INSTALL/go.tar.gz" -C "$TMPDIR_INSTALL/go-toolchain" || return 1
  candidate="$TMPDIR_INSTALL/go-toolchain/go/bin/go"
  go_is_usable "$candidate" || return 1
  printf '%s' "$candidate"
}

source_archive_url() {
  if [[ "$PULSE_RELEASE" != "latest" ]]; then
    printf '%s/archive/refs/tags/%s.tar.gz' "$REPO_URL" "$PULSE_RELEASE"
  else
    printf '%s/archive/refs/heads/%s.tar.gz' "$REPO_URL" "$BRANCH"
  fi
}

build_from_source() {
  local go_bin src_dir archive build_version ldflags
  go_bin="$(get_go)" || die "Could not find or download Go 1.26+. Use --release/--binary-url or install Go manually."
  log "Building Pulse from source with $("$go_bin" version | sed -n '1p')..." >&2

  if [[ "$PULSE_RELEASE" == "latest" && -f "$SCRIPT_DIR/cmd/pulse/main.go" && -f "$SCRIPT_DIR/go.mod" ]]; then
    src_dir="$SCRIPT_DIR"
    log "Using local source tree: $src_dir" >&2
  else
    archive="$(source_archive_url)"
    log "Downloading source: $archive" >&2
    curl -fsSL --retry 4 --retry-delay 2 --max-time 300 "$archive" -o "$TMPDIR_INSTALL/source.tar.gz" \
      || die "Could not download Pulse source from $archive"
    mkdir -p "$TMPDIR_INSTALL/source"
    tar -xzf "$TMPDIR_INSTALL/source.tar.gz" -C "$TMPDIR_INSTALL/source" \
      || die "Could not extract Pulse source archive."
    src_dir="$(find "$TMPDIR_INSTALL/source" -mindepth 1 -maxdepth 1 -type d -print -quit)"
    [[ -n "$src_dir" && -f "$src_dir/go.mod" ]] || die "Pulse source archive did not contain go.mod."
  fi

  build_version=""
  if [[ "$PULSE_RELEASE" != "latest" ]]; then
    build_version="${PULSE_RELEASE#v}"
  elif [[ -n "${PULSE_BUILD_VERSION:-}" ]]; then
    build_version="$PULSE_BUILD_VERSION"
  fi
  ldflags="-s -w"
  if [[ -n "$build_version" ]]; then
    ldflags="$ldflags -X main.version=$build_version"
  fi

  ( cd "$src_dir" && \
    CGO_ENABLED=0 GOOS=linux GOARCH="$ARCH" \
    GOPROXY="${GOPROXY:-https://proxy.golang.org,direct}" \
    "$go_bin" build -trimpath -ldflags "$ldflags" -o "$TMPDIR_INSTALL/pulse" ./cmd/pulse ) \
    || die "Pulse source build failed. Check network/GOPROXY and the build log above."
  PULSE_SRC="$TMPDIR_INSTALL/pulse"
  PULSE_SRC_KIND="source"
}

# ------------------------------------------------------------------ Pulse binary
SCRIPT_SOURCE="${BASH_SOURCE[0]:-}"
SCRIPT_DIR="$PWD"
if [[ -n "$SCRIPT_SOURCE" && -f "$SCRIPT_SOURCE" ]]; then
  SCRIPT_DIR="$(cd "$(dirname "$SCRIPT_SOURCE")" 2>/dev/null && pwd)" || SCRIPT_DIR="$PWD"
fi

release_asset_url() {
  local asset="$1"
  if [[ "$PULSE_RELEASE" == "latest" ]]; then
    printf '%s/releases/latest/download/%s' "$REPO_URL" "$asset"
  else
    printf '%s/releases/download/%s/%s' "$REPO_URL" "$PULSE_RELEASE" "$asset"
  fi
}

download_release_binary() {
  local asset="pulse-linux-$ARCH" url sum_url sum_line
  url="$(release_asset_url "$asset")"
  log "Downloading Pulse release binary: $url"
  if ! curl -fsSL --retry 3 --retry-delay 2 "$url" -o "$TMPDIR_INSTALL/$asset"; then
    return 1
  fi
  sum_url="$(release_asset_url "SHA256SUMS")"
  if curl -fsSL --retry 2 "$sum_url" -o "$TMPDIR_INSTALL/SHA256SUMS" 2>/dev/null; then
    sum_line="$(grep " $asset\$" "$TMPDIR_INSTALL/SHA256SUMS" || true)"
    if [[ -n "$sum_line" ]]; then
      ( cd "$TMPDIR_INSTALL" && printf '%s\n' "$sum_line" | sha256sum -c - >/dev/null ) \
        || { warn "Pulse release checksum verification failed."; return 1; }
      log "Release checksum verified."
    else
      warn "No checksum entry for $asset in SHA256SUMS; continuing."
    fi
  else
    warn "Release checksum file not found; continuing without checksum verification."
  fi
  if ! "$TMPDIR_INSTALL/$asset" --version >/dev/null 2>&1; then
    warn "Downloaded release binary is not runnable on this server; falling back to source build."
    return 1
  fi
  PULSE_SRC="$TMPDIR_INSTALL/$asset"
  PULSE_SRC_KIND="release"
}

if [[ -n "$BINARY_PATH" ]]; then
  [[ -f "$BINARY_PATH" ]] || die "Binary not found: $BINARY_PATH"
  PULSE_SRC="$BINARY_PATH"
  PULSE_SRC_KIND="local"
elif [[ -n "$BINARY_URL" ]]; then
  log "Downloading Pulse binary from custom URL..."
  curl -fsSL --retry 4 --retry-delay 2 "$BINARY_URL" -o "$TMPDIR_INSTALL/pulse-custom" \
    || die "Could not download the Pulse binary."
  PULSE_SRC="$TMPDIR_INSTALL/pulse-custom"
  PULSE_SRC_KIND="custom"
elif [[ "$FORCE_SOURCE" -eq 1 ]]; then
  build_from_source
elif ! download_release_binary; then
  warn "No usable Pulse release asset was found for $ARCH; building from source."
  build_from_source
fi

chmod 0755 "$PULSE_SRC"
PULSE_BIN_VERSION="$("$PULSE_SRC" --version 2>/dev/null | sed -n '1p' || true)"
[[ "$PULSE_BIN_VERSION" == pulse* ]] || die "The selected file is not a runnable Pulse Linux binary for $ARCH."
log "Prepared $PULSE_BIN_VERSION ($PULSE_SRC_KIND)."

# ------------------------------------------------------------------ Xray
#
# Pulse only ever needs a short-lived, loopback-only core for proxy checks.
# Install a Pulse-owned copy under /usr/local/lib/pulse and never overwrite or
# restart an existing Xray binary/service that a VPN panel may be using.
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
      warn "Xray installation disabled; using the existing system Xray. Pulse will only run it with temporary loopback configs."
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
  if ! curl -fsSL --retry 4 --retry-delay 2 "$url" -o "$TMPDIR_INSTALL/xray.zip"; then
    if command -v xray >/dev/null 2>&1 && use_xray "$(command -v xray)"; then
      warn "Xray download failed; using the existing system Xray for temporary loopback checks."
      return 0
    fi
    warn "Could not download Xray from $url. Pulse will work, but real proxy tests will be disabled."
    return 0
  fi
  if ! unzip -q "$TMPDIR_INSTALL/xray.zip" xray -d "$TMPDIR_INSTALL" 2>/dev/null; then
    unzip -q "$TMPDIR_INSTALL/xray.zip" -d "$TMPDIR_INSTALL" || return 0
  fi
  if [[ ! -f "$TMPDIR_INSTALL/xray" ]]; then
    warn "Xray archive did not contain the xray binary."
    return 0
  fi
  install -d -m 0755 /usr/local/lib/pulse
  install -m 0755 "$TMPDIR_INSTALL/xray" "$PULSE_XRAY_PATH"
  if ! use_xray "$PULSE_XRAY_PATH"; then
    warn "The Pulse Xray copy is not runnable on this server."
    XRAY_OK=0
    XRAY_PATH=""
    return 0
  fi
}
install_xray

# ------------------------------------------------------------------ install Pulse
if systemctl cat pulse.service >/dev/null 2>&1; then
  systemctl stop pulse >/dev/null 2>&1 || true
fi
if [[ -f /var/lib/pulse/pulse.db ]]; then
  mkdir -p /var/backups
  log "Backing up existing Pulse data..."
  tar czf "/var/backups/pulse-data-$(date +%Y%m%d-%H%M%S).tar.gz" -C /var/lib pulse 2>/dev/null || true
  chmod 600 /var/backups/pulse-data-*.tar.gz 2>/dev/null || true
fi
if ! id pulse >/dev/null 2>&1; then
  useradd --system --home-dir /var/lib/pulse --create-home --shell /usr/sbin/nologin pulse
fi
install -d -m 0750 -o pulse -g pulse /var/lib/pulse
install -d -m 0755 -o root -g root /etc/pulse
install -m 0755 "$PULSE_SRC" /usr/local/bin/pulse
ln -sfn /usr/local/bin/pulse /usr/local/bin/pulse-agent
ln -sfn /usr/local/bin/pulse /usr/local/bin/pulse-a
INSTALLED_VERSION="$(/usr/local/bin/pulse --version 2>/dev/null | sed -n '1p' || echo 'pulse')"

if [[ -n "$BEHIND_PROXY_URL" ]]; then
  FINAL_BIND="127.0.0.1:$PORT"
  PUBLIC_BASE_URL="${BEHIND_PROXY_URL%/}"
elif [[ "$USE_SSL" == "yes" ]]; then
  FINAL_BIND="127.0.0.1:$PORT"
  PUBLIC_BASE_URL="https://$DOMAIN"
else
  FINAL_BIND="0.0.0.0:$PORT"
  PUBLIC_HOST="${PUBLIC_IP:-$(hostname -f 2>/dev/null || hostname)}"
  [[ "$PUBLIC_HOST" == *:* && "$PUBLIC_HOST" != \[* ]] && PUBLIC_HOST="[$PUBLIC_HOST]"
  PUBLIC_BASE_URL="http://$PUBLIC_HOST:$PORT"
fi

write_unit() {
  local bind="$1"
  cat >/etc/systemd/system/pulse.service <<UNIT
[Unit]
Description=Pulse - infrastructure monitoring panel
Documentation=${REPO_URL}
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=pulse
Group=pulse
WorkingDirectory=/var/lib/pulse
StateDirectory=pulse
EnvironmentFile=-/etc/pulse/pulse.env
Environment=HOME=/var/lib/pulse
Environment=PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
ExecStart=/usr/local/bin/pulse --addr $bind --db /var/lib/pulse/pulse.db
Restart=always
RestartSec=5
TimeoutStopSec=20
LimitNOFILE=65536

AmbientCapabilities=CAP_NET_RAW
CapabilityBoundingSet=CAP_NET_RAW
NoNewPrivileges=true

PrivateTmp=true
PrivateDevices=true
ProtectSystem=strict
ProtectHome=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
ReadWritePaths=/var/lib/pulse
RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX
LockPersonality=true

[Install]
WantedBy=multi-user.target
UNIT
  chmod 0644 /etc/systemd/system/pulse.service
}

# Start on loopback first; create the admin account before exposing the panel.
write_unit "127.0.0.1:$PORT"
touch /etc/pulse/pulse.env
if grep -qE '^PULSE_(PUBLIC_BASE_URL|XRAY_BIN)=' /etc/pulse/pulse.env 2>/dev/null; then
  sed -E '/^PULSE_(PUBLIC_BASE_URL|XRAY_BIN)=/d' /etc/pulse/pulse.env >"$TMPDIR_INSTALL/pulse.env"
  cat "$TMPDIR_INSTALL/pulse.env" >/etc/pulse/pulse.env
fi
printf 'PULSE_PUBLIC_BASE_URL=%s\n' "$PUBLIC_BASE_URL" >>/etc/pulse/pulse.env
if [[ "$XRAY_OK" -eq 1 ]]; then
  printf 'PULSE_XRAY_BIN=%s\n' "$XRAY_PATH" >>/etc/pulse/pulse.env
fi
chmod 0600 /etc/pulse/pulse.env

systemctl daemon-reload
systemctl enable pulse >/dev/null 2>&1 || true
systemctl restart pulse

HEALTH=""
for _ in $(seq 1 40); do
  HEALTH="$(curl -fsS --max-time 2 "http://127.0.0.1:$PORT/api/health" 2>/dev/null || true)"
  [[ "$HEALTH" == *'"ok":true'* ]] && break
  sleep 1
done
if [[ "$HEALTH" != *'"ok":true'* ]]; then
  journalctl -u pulse -n 100 --no-pager || true
  die "Pulse did not start. See: journalctl -u pulse -n 100 --no-pager"
fi
log "Pulse is running on 127.0.0.1:$PORT."

# ------------------------------------------------------------------ admin account
ADMIN_CREATED=0
ADMIN_EXISTING=0
ADMIN_FAILED=0
ADMIN_PASSWORD=""
if [[ "$NO_ADMIN" -eq 0 ]]; then
  AUTH_STATE="$(curl -fsS "http://127.0.0.1:$PORT/api/auth/state" 2>/dev/null || true)"
  if [[ -z "$AUTH_STATE" ]]; then
    ADMIN_FAILED=1
    warn "Could not read auth state; finish setup from the panel."
  elif [[ "$AUTH_STATE" == *'"needs_setup":true'* ]]; then
    ADMIN_PASSWORD="${ADMIN_PASS:-$(openssl rand -hex 16)}"
    json_escape() {
      local s="$1"
      s="${s//\\/\\\\}"; s="${s//\"/\\\"}"; s="${s//$'\n'/}"; s="${s//$'\r'/}"
      printf '%s' "$s"
    }
    printf '{"username":"%s","password":"%s"}' \
      "$(json_escape "$ADMIN_USER")" "$(json_escape "$ADMIN_PASSWORD")" >"$TMPDIR_INSTALL/admin.json"
    chmod 0600 "$TMPDIR_INSTALL/admin.json"
    SETUP_RESP="$(curl -fsS -X POST "http://127.0.0.1:$PORT/api/auth/setup" \
      -H 'Content-Type: application/json' --data-binary "@$TMPDIR_INSTALL/admin.json" 2>/dev/null || true)"
    if [[ "$SETUP_RESP" == *'"ok":true'* ]]; then
      ADMIN_CREATED=1
      umask 077
      {
        echo "Pulse admin credentials"
        echo "URL: $PUBLIC_BASE_URL"
        echo "Username: $ADMIN_USER"
        echo "Password: $ADMIN_PASSWORD"
        echo "Created: $(date -Iseconds)"
        echo "Change the password after the first login, then delete this file."
      } >/root/pulse-admin-credentials.txt
      umask 022
      chmod 0600 /root/pulse-admin-credentials.txt
      log "Admin account created."
    else
      ADMIN_FAILED=1
      warn "Could not create the admin account automatically; use the panel setup screen."
    fi
  else
    ADMIN_EXISTING=1
    log "Admin account already exists; keeping it unchanged."
  fi
fi

# ------------------------------------------------------------------ expose panel
if [[ -n "$BEHIND_PROXY_URL" ]]; then
  log "Behind-proxy mode: Pulse stays on 127.0.0.1:$PORT; Caddy, ports 80/443 and firewall entries are left untouched."
  write_unit "$FINAL_BIND"
  systemctl daemon-reload
  systemctl restart pulse
  for _ in $(seq 1 20); do
    curl -fsS --max-time 2 "http://127.0.0.1:$PORT/api/health" >/dev/null 2>&1 && break
    sleep 1
  done
elif [[ "$USE_SSL" == "yes" ]]; then
  log "Configuring Caddy for https://$DOMAIN ..."
  install -d -m 0755 /etc/caddy /etc/caddy/conf.d
  PULSE_CADDY="/etc/caddy/conf.d/pulse.caddy"
  # Prefer h2c for the gRPC agent link. If this Caddy is too old to understand
  # the transport block, validation below rewrites the site without it and the
  # agent uses its WebSocket/REST fallbacks instead.
  write_pulse_caddy 1

  if [[ -f /etc/caddy/Caddyfile ]]; then
    cp -a /etc/caddy/Caddyfile "/etc/caddy/Caddyfile.bak-$(date +%Y%m%d-%H%M%S)" || true
    if ! grep -Eq '^[[:space:]]*import[[:space:]]+.*conf\.d/' /etc/caddy/Caddyfile; then
      printf '\n# Pulse site (managed block)\nimport conf.d/*.caddy\n' >>/etc/caddy/Caddyfile
    fi
  else
    {
      if [[ -n "$EMAIL" ]]; then
        printf '{\n\temail %s\n}\n\n' "$EMAIL"
      fi
      printf 'import conf.d/*.caddy\n'
    } >/etc/caddy/Caddyfile
  fi
  chmod 0644 /etc/caddy/Caddyfile

  if ! caddy validate --config /etc/caddy/Caddyfile >/dev/null 2>&1 && grep -q 'versions h2c' "$PULSE_CADDY"; then
    warn "This Caddy rejected the h2c transport block; retrying with the WebSocket-compatible config."
    write_pulse_caddy 0
  fi

  if caddy validate --config /etc/caddy/Caddyfile >/dev/null 2>&1; then
    open_firewall
    systemctl enable caddy >/dev/null 2>&1 || true
    if systemctl restart caddy; then
      SSL_OK=1
      log "Caddy is running. The certificate may take a moment to issue."
    else
      warn "Caddy failed to start. Check: journalctl -u caddy -n 100 --no-pager"
    fi
  else
    warn "Caddyfile validation failed. Check: caddy validate --config /etc/caddy/Caddyfile"
  fi
else
  log "No SSL requested; exposing the panel directly on port $PORT."
  open_firewall
  write_unit "$FINAL_BIND"
  systemctl daemon-reload
  systemctl restart pulse
  for _ in $(seq 1 20); do
    curl -fsS --max-time 2 "http://127.0.0.1:$PORT/api/health" >/dev/null 2>&1 && break
    sleep 1
  done
fi

# ------------------------------------------------------------------ summary
echo
echo "============================================================"
echo " Pulse installation summary"
echo "============================================================"
echo " Version    : ${INSTALLED_VERSION#pulse }"
echo " Source     : $PULSE_SRC_KIND (${PULSE_RELEASE})"
echo " Panel URL  : $PUBLIC_BASE_URL"
if [[ -n "$BEHIND_PROXY_URL" ]]; then
  echo " Proxy      : behind existing reverse proxy at $PUBLIC_BASE_URL"
  echo " Bind       : 127.0.0.1:$PORT (Caddy/443 untouched)"
elif [[ "$USE_SSL" == "yes" ]]; then
  if [[ "$SSL_OK" -eq 1 ]]; then
    echo " HTTPS      : enabled (Caddy, automatic Let's Encrypt)"
    if [[ "$DNS_OK" -eq 0 ]]; then
      echo " DNS        : not fully verified; certificate may stay pending until DNS propagates"
    fi
  else
    echo " HTTPS      : requested, but Caddy is not healthy"
    echo "              Diagnostics: journalctl -u caddy -n 100 --no-pager"
    echo "              Temporary SSH tunnel: ssh -L $PORT:127.0.0.1:$PORT root@SERVER"
  fi
else
  echo " HTTPS      : disabled (direct HTTP on port $PORT)"
  echo " WARNING    : Put this panel behind HTTPS before using it on the internet."
fi
if [[ "$XRAY_OK" -eq 1 ]]; then
  echo " Xray       : $("$XRAY_PATH" version 2>/dev/null | sed -n '1p')"
else
  echo " Xray       : not installed; real proxy tests will degrade with a warning"
fi
if [[ "$ADMIN_CREATED" -eq 1 ]]; then
  echo " Admin      : $ADMIN_USER"
  echo " Password   : $ADMIN_PASSWORD"
  echo " Credentials: /root/pulse-admin-credentials.txt (mode 600)"
elif [[ "$ADMIN_EXISTING" -eq 1 ]]; then
  echo " Admin      : existing account preserved"
elif [[ "$ADMIN_FAILED" -eq 1 ]]; then
  echo " Admin      : not created automatically; open the panel setup screen"
else
  echo " Admin      : not created (--no-admin); open the panel and finish setup"
fi
echo
echo " Next steps:"
echo "   1. Open $PUBLIC_BASE_URL and change the admin password."
echo "   2. Settings -> Xray Core -> Check again."
echo "   3. Add your first monitor and configure Telegram / Cloudflare."
echo
echo " Useful commands:"
echo "   pulse status"
echo "   pulse restart"
echo "   pulse logs"
echo "   pulse update"
echo "   pulse-agent status"
echo "   pulse-agent update"
if [[ "$USE_SSL" == "yes" ]]; then echo "   journalctl -u caddy -f"; fi
echo "   Source: $REPO_URL"
echo "   Open ports 80/443 in your provider firewall when using HTTPS."
echo "============================================================"
