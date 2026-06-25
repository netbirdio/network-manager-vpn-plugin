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

  echo "warning: could not reload NetworkManager automatically; restart NetworkManager if vpn-type netbird is not visible" >&2
}

install_selinux_policy() {
  policy=/usr/share/selinux/packages/nm_netbird.pp
  marker_dir=/var/lib/network-manager-netbird
  marker="$marker_dir/selinux-policy-installed"

  [ -f "$policy" ] || return 0

  if ! command -v semodule >/dev/null 2>&1; then
    echo "warning: SELinux policy module is packaged at $policy but semodule was not found" >&2
    return 0
  fi

  if semodule -i "$policy" >/dev/null 2>&1; then
    mkdir -p "$marker_dir"
    : >"$marker"
    echo "installed SELinux policy module nm_netbird"
  else
    echo "warning: could not install SELinux policy module $policy" >&2
    return 0
  fi

  if command -v restorecon >/dev/null 2>&1; then
    for path in \
      /usr/libexec/nm-netbird-service \
      /usr/libexec/nm-netbird-auth-dialog \
      /run/netbird.sock \
      /var/run/netbird.sock; do
      [ -e "$path" ] && restorecon "$path" >/dev/null 2>&1 || true
    done
  fi
}

install_selinux_policy
reload_dbus_policy
reload_networkmanager

cat <<'EOF'
NetworkManager NetBird plugin installed.
Create a profile with:
  nmcli connection add type vpn con-name NetBird vpn-type netbird ifname --
EOF
