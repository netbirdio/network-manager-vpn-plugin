#!/usr/bin/env sh
set -eu

DESTDIR=${DESTDIR:-}
if [ -z "$DESTDIR" ] && [ "$(id -u)" -ne 0 ]; then
  echo "error: run this installer as root" >&2
  exit 1
fi

SCRIPT_DIR=$(CDPATH= cd "$(dirname "$0")" && pwd)
SOURCE_ROOT=$SCRIPT_DIR
if [ -d "$SCRIPT_DIR/../packaging" ]; then
  SOURCE_ROOT=$(CDPATH= cd "$SCRIPT_DIR/.." && pwd)
fi

LIBEXEC_DIR=${LIBEXEC_DIR:-/usr/libexec}
NM_VPN_DIR=${NM_VPN_DIR:-/usr/share/NetworkManager/VPN}
DBUS_POLICY_DIR=${DBUS_POLICY_DIR:-/etc/dbus-1/system.d}
NM_CONF_DIR=${NM_CONF_DIR:-/etc/NetworkManager/conf.d}

SERVICE_SRC=${SERVICE_SRC:-$SCRIPT_DIR/nm-netbird-service}
if [ ! -f "$SERVICE_SRC" ] && [ -f "$SCRIPT_DIR/bin/nm-netbird-service" ]; then
  SERVICE_SRC=$SCRIPT_DIR/bin/nm-netbird-service
fi
if [ ! -f "$SERVICE_SRC" ] && [ -f "$SOURCE_ROOT/bin/nm-netbird-service" ]; then
  SERVICE_SRC=$SOURCE_ROOT/bin/nm-netbird-service
fi

AUTH_DIALOG_SRC=${AUTH_DIALOG_SRC:-$SCRIPT_DIR/nm-netbird-auth-dialog}
if [ ! -f "$AUTH_DIALOG_SRC" ] && [ -f "$SCRIPT_DIR/bin/nm-netbird-auth-dialog" ]; then
  AUTH_DIALOG_SRC=$SCRIPT_DIR/bin/nm-netbird-auth-dialog
fi
if [ ! -f "$AUTH_DIALOG_SRC" ] && [ -f "$SOURCE_ROOT/bin/nm-netbird-auth-dialog" ]; then
  AUTH_DIALOG_SRC=$SOURCE_ROOT/bin/nm-netbird-auth-dialog
fi

VPN_NAME_SRC=${VPN_NAME_SRC:-$SOURCE_ROOT/packaging/NetworkManager/VPN/nm-netbird-service.name}
DBUS_POLICY_SRC=${DBUS_POLICY_SRC:-$SOURCE_ROOT/packaging/dbus-1/system.d/nm-netbird-service.conf}
NM_UNMANAGED_SRC=${NM_UNMANAGED_SRC:-$SOURCE_ROOT/packaging/NetworkManager/conf.d/90-netbird-unmanaged.conf}

require_file() {
  path=$1
  if [ ! -f "$path" ]; then
    echo "error: missing required file: $path" >&2
    exit 1
  fi
}

install_file() {
  src=$1
  dst=$2
  mode=$3
  target=$DESTDIR$dst

  mkdir -p "$(dirname "$target")"
  install -m "$mode" "$src" "$target"
  echo "installed $dst"
}

install_config_noreplace() {
  src=$1
  dst=$2
  mode=$3
  target=$DESTDIR$dst

  mkdir -p "$(dirname "$target")"
  if [ ! -e "$target" ]; then
    install -m "$mode" "$src" "$target"
    echo "installed $dst"
    return
  fi

  if cmp -s "$src" "$target"; then
    echo "kept existing $dst"
    return
  fi

  install -m "$mode" "$src" "$target.new"
  echo "warning: kept existing $dst; installed updated sample at $dst.new" >&2
}

reload_dbus_policy() {
  if command -v dbus-send >/dev/null 2>&1; then
    if dbus-send --system --type=method_call --dest=org.freedesktop.DBus \
      /org/freedesktop/DBus org.freedesktop.DBus.ReloadConfig >/dev/null 2>&1; then
      echo "reloaded D-Bus system policy"
      return
    fi
  fi

  if command -v busctl >/dev/null 2>&1; then
    if busctl call org.freedesktop.DBus /org/freedesktop/DBus \
      org.freedesktop.DBus ReloadConfig >/dev/null 2>&1; then
      echo "reloaded D-Bus system policy"
      return
    fi
  fi

  if command -v systemctl >/dev/null 2>&1; then
    if systemctl reload dbus >/dev/null 2>&1; then
      echo "reloaded D-Bus system policy"
      return
    fi
  fi

  echo "warning: could not reload D-Bus policy automatically; restart the system bus if direct plugin access fails" >&2
}

reload_networkmanager() {
  if command -v systemctl >/dev/null 2>&1 && systemctl is-active --quiet NetworkManager >/dev/null 2>&1; then
    if systemctl reload NetworkManager >/dev/null 2>&1; then
      echo "reloaded NetworkManager"
      return
    fi
  fi

  if command -v nmcli >/dev/null 2>&1; then
    if nmcli general reload >/dev/null 2>&1; then
      echo "reloaded NetworkManager"
      return
    fi
  fi

  echo "warning: could not reload NetworkManager automatically; restart NetworkManager if vpn-type netbird is not visible" >&2
}

require_file "$SERVICE_SRC"
require_file "$AUTH_DIALOG_SRC"
require_file "$VPN_NAME_SRC"
require_file "$DBUS_POLICY_SRC"
require_file "$NM_UNMANAGED_SRC"

install_file "$SERVICE_SRC" "$LIBEXEC_DIR/nm-netbird-service" 0755
install_file "$AUTH_DIALOG_SRC" "$LIBEXEC_DIR/nm-netbird-auth-dialog" 0755
install_file "$VPN_NAME_SRC" "$NM_VPN_DIR/nm-netbird-service.name" 0644
install_file "$DBUS_POLICY_SRC" "$DBUS_POLICY_DIR/nm-netbird-service.conf" 0644
install_config_noreplace "$NM_UNMANAGED_SRC" "$NM_CONF_DIR/90-netbird-unmanaged.conf" 0644

if [ -z "$DESTDIR" ]; then
  reload_dbus_policy
  reload_networkmanager
else
  echo "staged install under DESTDIR=$DESTDIR; skipped service reloads"
fi

cat <<'EOF'

NetworkManager NetBird plugin installed.

Next steps:
  1. Make sure the NetBird daemon/runtime is installed and logged in, or configure setup-key/SSO data.
  2. Create a NetworkManager profile, for example:
       nmcli connection add type vpn con-name NetBird vpn-type netbird ifname --
  3. Activate it:
       nmcli connection up NetBird
EOF
