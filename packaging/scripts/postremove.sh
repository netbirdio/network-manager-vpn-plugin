#!/usr/bin/env sh
set -eu

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

remove_selinux_policy() {
  case "${1:-}" in
    0|remove|purge) ;;
    *) return 0 ;;
  esac

  marker=/var/lib/network-manager-netbird/selinux-policy-installed
  [ -e "$marker" ] || return 0

  if ! command -v semodule >/dev/null 2>&1; then
    echo "warning: could not remove SELinux policy module nm_netbird because semodule was not found" >&2
    return 0
  fi

  modules=$(semodule -l 2>/dev/null) || {
    echo "warning: could not query SELinux policy modules; keeping $marker" >&2
    return 0
  }

  if ! printf '%s\n' "$modules" | awk '{ print $1 }' | grep -qx nm_netbird; then
    echo "SELinux policy module nm_netbird is not installed"
    rm -f "$marker"
    rmdir /var/lib/network-manager-netbird 2>/dev/null || true
    return 0
  fi

  if semodule -r nm_netbird >/dev/null 2>&1; then
    echo "removed SELinux policy module nm_netbird"
  else
    echo "warning: could not remove SELinux policy module nm_netbird; keeping $marker" >&2
    return 0
  fi

  rm -f "$marker"
  rmdir /var/lib/network-manager-netbird 2>/dev/null || true
}

remove_selinux_policy "$@"
reload_dbus_policy
reload_networkmanager
