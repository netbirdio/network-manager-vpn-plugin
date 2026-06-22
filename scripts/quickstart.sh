#!/usr/bin/env sh
set -eu

REPO=${REPO:-netbirdio/network-manager-vpn-plugin}
RELEASE_TAG=${RELEASE_TAG:-${VERSION:-latest}}

log() {
  printf '%s\n' "$*"
}

die() {
  printf 'error: %s\n' "$*" >&2
  exit 1
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "missing required command: $1"
}

as_root() {
  if [ "$(id -u)" -eq 0 ]; then
    "$@"
    return
  fi

  need_cmd sudo
  sudo "$@"
}

install_system_deps() {
  case "$1" in
    apt)
      as_root apt-get update
      as_root env DEBIAN_FRONTEND=noninteractive apt-get install -y \
        network-manager curl ca-certificates
      ;;
    dnf)
      as_root dnf install -y NetworkManager curl ca-certificates
      ;;
    yum)
      as_root yum install -y NetworkManager curl ca-certificates
      ;;
    *)
      die "unsupported package manager: $1"
      ;;
  esac
}

enable_networkmanager() {
  if command -v systemctl >/dev/null 2>&1; then
    if ! as_root systemctl enable --now NetworkManager; then
      log "warning: could not enable/start NetworkManager; start it manually if it is not running"
    fi
  else
    log "warning: systemctl not found; start NetworkManager manually if it is not running"
  fi
}

install_local_package() {
  manager=$1
  package=$2

  case "$manager" in
    apt)
      as_root env DEBIAN_FRONTEND=noninteractive apt-get install -y "$package"
      ;;
    dnf)
      as_root dnf install -y "$package"
      ;;
    yum)
      as_root yum install -y "$package"
      ;;
    *)
      die "unsupported package manager: $manager"
      ;;
  esac
}

case "$(uname -s)" in
  Linux) ;;
  *) die "this installer supports Linux only" ;;
esac

case "$(uname -m)" in
  x86_64|amd64) ;;
  *) die "quickstart packages are currently published for amd64/x86_64 only; use the tarball install from the README for this architecture" ;;
esac

if command -v apt-get >/dev/null 2>&1; then
  package_manager=apt
  package_ext=deb
elif command -v dnf >/dev/null 2>&1; then
  package_manager=dnf
  package_ext=rpm
elif command -v yum >/dev/null 2>&1; then
  package_manager=yum
  package_ext=rpm
else
  die "could not find apt-get, dnf, or yum"
fi

need_cmd grep
need_cmd cut
need_cmd head
need_cmd mktemp

if [ "$RELEASE_TAG" = latest ]; then
  release_api="https://api.github.com/repos/$REPO/releases/latest"
else
  release_api="https://api.github.com/repos/$REPO/releases/tags/$RELEASE_TAG"
fi

log "Installing NetworkManager prerequisites..."
install_system_deps "$package_manager"
need_cmd curl

tmpdir=$(mktemp -d "${TMPDIR:-/tmp}/network-manager-netbird.XXXXXX")
cleanup() {
  rm -rf "$tmpdir"
}
trap cleanup EXIT HUP INT TERM

log "Resolving $RELEASE_TAG network-manager-netbird .$package_ext package..."
asset_url=$(curl -fsSL "$release_api" |
  grep '"browser_download_url":' |
  grep -E "network-manager-netbird.*(_linux_amd64|_amd64|\\.x86_64)\\.$package_ext\"" |
  cut -d '"' -f 4 |
  head -n 1)

if [ -z "$asset_url" ]; then
  die "could not find an amd64 .$package_ext asset in $release_api"
fi

package_path="$tmpdir/network-manager-netbird.$package_ext"
log "Downloading $asset_url..."
curl -fL -o "$package_path" "$asset_url"

log "Enabling NetworkManager..."
enable_networkmanager

log "Installing network-manager-netbird..."
if ! install_local_package "$package_manager" "$package_path"; then
  die "package installation failed. If the error mentioned an unmet 'netbird' dependency, install the NetBird daemon/runtime first: https://docs.netbird.io/get-started/install/linux"
fi

cat <<'EOF'

NetworkManager NetBird plugin installed.

Next steps:
  1. Make sure the NetBird daemon/runtime is installed and authenticated.
  2. Create and activate a NetworkManager NetBird VPN profile.

See the README Quick start section for nmcli examples.
EOF
