#!/usr/bin/env sh
set -eu

DESTDIR=${DESTDIR:-}
if [ -z "$DESTDIR" ] && [ "$(id -u)" -ne 0 ]; then
  echo "error: run this uninstaller as root" >&2
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
NM_UNMANAGED_SRC=${NM_UNMANAGED_SRC:-$SOURCE_ROOT/packaging/NetworkManager/conf.d/90-netbird-unmanaged.conf}

remove_file() {
  dst=$1
  target=$DESTDIR$dst

  if [ -e "$target" ]; then
    rm -f "$target"
    echo "removed $dst"
  fi
}

remove_config_if_matching() {
  src=$1
  dst=$2
  target=$DESTDIR$dst

  if [ ! -e "$target" ]; then
    return
  fi

  if [ -f "$src" ] && cmp -s "$src" "$target"; then
    rm -f "$target"
    echo "removed $dst"
    return
  fi

  echo "warning: kept modified config $dst; remove it manually if it is no longer needed" >&2
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

  echo "warning: could not reload D-Bus policy automatically" >&2
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

  echo "warning: could not reload NetworkManager automatically" >&2
}

remove_file "$LIBEXEC_DIR/nm-netbird-service"
remove_file "$LIBEXEC_DIR/nm-netbird-auth-dialog"
remove_file "$NM_VPN_DIR/nm-netbird-service.name"
remove_file "$DBUS_POLICY_DIR/nm-netbird-service.conf"
remove_config_if_matching "$NM_UNMANAGED_SRC" "$NM_CONF_DIR/90-netbird-unmanaged.conf"

if [ -z "$DESTDIR" ]; then
  reload_dbus_policy
  reload_networkmanager
else
  echo "staged uninstall under DESTDIR=$DESTDIR; skipped service reloads"
fi

cat <<'EOF'

NetworkManager NetBird plugin removed.
Saved NetworkManager connection profiles were not deleted.
EOF
