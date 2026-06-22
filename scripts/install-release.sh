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
NM_PLUGIN_DIR=${NM_PLUGIN_DIR:-/usr/lib/NetworkManager}
NM_VPN_DIR=${NM_VPN_DIR:-/etc/NetworkManager/VPN}
DBUS_POLICY_DIR=${DBUS_POLICY_DIR:-/etc/dbus-1/system.d}
NM_CONF_DIR=${NM_CONF_DIR:-/etc/NetworkManager/conf.d}
WITH_GTK4_SPECIFIED=${WITH_GTK4+x}
WITH_GTK4=${WITH_GTK4:-no}
if [ "$WITH_GTK4" != yes ]; then
  WITH_GTK4=no
fi
PROPERTIES_BUILD_DIR=

cleanup() {
  if [ -n "$PROPERTIES_BUILD_DIR" ]; then
    rm -rf "$PROPERTIES_BUILD_DIR"
  fi
}
trap cleanup EXIT HUP INT TERM

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

PROPERTIES_PLUGIN_SRC=${PROPERTIES_PLUGIN_SRC:-$SCRIPT_DIR/libnm-vpn-plugin-netbird.so}
if [ ! -f "$PROPERTIES_PLUGIN_SRC" ] && [ -f "$SCRIPT_DIR/bin/libnm-vpn-plugin-netbird.so" ]; then
  PROPERTIES_PLUGIN_SRC=$SCRIPT_DIR/bin/libnm-vpn-plugin-netbird.so
fi
if [ ! -f "$PROPERTIES_PLUGIN_SRC" ] && [ -f "$SOURCE_ROOT/bin/libnm-vpn-plugin-netbird.so" ]; then
  PROPERTIES_PLUGIN_SRC=$SOURCE_ROOT/bin/libnm-vpn-plugin-netbird.so
fi

PROPERTIES_GTK3_EDITOR_SRC=${PROPERTIES_GTK3_EDITOR_SRC:-$SCRIPT_DIR/libnm-vpn-plugin-netbird-editor.so}
if [ ! -f "$PROPERTIES_GTK3_EDITOR_SRC" ] && [ -f "$SCRIPT_DIR/bin/libnm-vpn-plugin-netbird-editor.so" ]; then
  PROPERTIES_GTK3_EDITOR_SRC=$SCRIPT_DIR/bin/libnm-vpn-plugin-netbird-editor.so
fi
if [ ! -f "$PROPERTIES_GTK3_EDITOR_SRC" ] && [ -f "$SOURCE_ROOT/bin/libnm-vpn-plugin-netbird-editor.so" ]; then
  PROPERTIES_GTK3_EDITOR_SRC=$SOURCE_ROOT/bin/libnm-vpn-plugin-netbird-editor.so
fi

PROPERTIES_GTK4_EDITOR_SRC=${PROPERTIES_GTK4_EDITOR_SRC:-}
if [ -z "$PROPERTIES_GTK4_EDITOR_SRC" ] && [ -f "$SCRIPT_DIR/libnm-gtk4-vpn-plugin-netbird-editor.so" ]; then
  PROPERTIES_GTK4_EDITOR_SRC=$SCRIPT_DIR/libnm-gtk4-vpn-plugin-netbird-editor.so
fi
if [ -z "$PROPERTIES_GTK4_EDITOR_SRC" ] && [ -f "$SCRIPT_DIR/bin/libnm-gtk4-vpn-plugin-netbird-editor.so" ]; then
  PROPERTIES_GTK4_EDITOR_SRC=$SCRIPT_DIR/bin/libnm-gtk4-vpn-plugin-netbird-editor.so
fi
if [ -z "$PROPERTIES_GTK4_EDITOR_SRC" ] && [ -f "$SOURCE_ROOT/bin/libnm-gtk4-vpn-plugin-netbird-editor.so" ]; then
  PROPERTIES_GTK4_EDITOR_SRC=$SOURCE_ROOT/bin/libnm-gtk4-vpn-plugin-netbird-editor.so
fi
if [ -z "$WITH_GTK4_SPECIFIED" ]; then
  if [ -n "$PROPERTIES_GTK4_EDITOR_SRC" ]; then
    WITH_GTK4=yes
  elif command -v pkg-config >/dev/null 2>&1 && pkg-config --exists gtk4 libnma-gtk4; then
    WITH_GTK4=yes
  fi
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

install_vpn_metadata() {
  tmp=$(mktemp "${TMPDIR:-/tmp}/nm-netbird-service.name.XXXXXX")
  awk -v plugin="$NM_PLUGIN_DIR/libnm-vpn-plugin-netbird.so" '
    /^plugin=/ { print "plugin=" plugin; next }
    { print }
  ' "$VPN_NAME_SRC" >"$tmp"
  install_file "$tmp" "$NM_VPN_DIR/nm-netbird-service.name" 0644
  rm -f "$tmp"
}

build_properties_plugins() {
  src_dir=$SOURCE_ROOT/properties
  if [ ! -f "$src_dir/nm-netbird-editor-model.c" ] || \
    [ ! -f "$src_dir/nm-netbird-editor.c" ] || \
    [ ! -f "$src_dir/nm-netbird-editor-plugin.c" ]; then
    return 1
  fi

  if ! command -v cc >/dev/null 2>&1 || ! command -v pkg-config >/dev/null 2>&1; then
    return 1
  fi

  if ! pkg-config --exists libnm gtk+-3.0 libnma; then
    return 1
  fi

  if [ "$WITH_GTK4" = yes ] && ! pkg-config --exists gtk4 libnma-gtk4; then
    return 1
  fi

  PROPERTIES_BUILD_DIR=$(mktemp -d "${TMPDIR:-/tmp}/nm-netbird-properties.XXXXXX")

  echo "building libnm-vpn-plugin-netbird.so from bundled source"
  cc -Wall -Wextra -fPIC -shared \
    -o "$PROPERTIES_BUILD_DIR/libnm-vpn-plugin-netbird.so" \
    "$src_dir/nm-netbird-editor-plugin.c" \
    $(pkg-config --cflags --libs libnm) \
    -ldl
  PROPERTIES_PLUGIN_SRC=$PROPERTIES_BUILD_DIR/libnm-vpn-plugin-netbird.so

  echo "building libnm-vpn-plugin-netbird-editor.so from bundled source"
  cc -Wall -Wextra \
    -DGDK_VERSION_MIN_REQUIRED=GDK_VERSION_3_22 \
    -DGDK_VERSION_MAX_ALLOWED=GDK_VERSION_3_22 \
    -fPIC -shared \
    -o "$PROPERTIES_BUILD_DIR/libnm-vpn-plugin-netbird-editor.so" \
    "$src_dir/nm-netbird-editor-model.c" \
    "$src_dir/nm-netbird-editor.c" \
    $(pkg-config --cflags --libs libnm gtk+-3.0 libnma)
  PROPERTIES_GTK3_EDITOR_SRC=$PROPERTIES_BUILD_DIR/libnm-vpn-plugin-netbird-editor.so

  if [ "$WITH_GTK4" = yes ]; then
    echo "building libnm-gtk4-vpn-plugin-netbird-editor.so from bundled source"
    cc -Wall -Wextra \
      -DGDK_VERSION_MIN_REQUIRED=GDK_VERSION_4_0 \
      -DGDK_VERSION_MAX_ALLOWED=GDK_VERSION_4_0 \
      -fPIC -shared \
      -o "$PROPERTIES_BUILD_DIR/libnm-gtk4-vpn-plugin-netbird-editor.so" \
      "$src_dir/nm-netbird-editor-model.c" \
      "$src_dir/nm-netbird-editor.c" \
      $(pkg-config --cflags --libs libnm gtk4 libnma-gtk4)
    PROPERTIES_GTK4_EDITOR_SRC=$PROPERTIES_BUILD_DIR/libnm-gtk4-vpn-plugin-netbird-editor.so
  fi
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

if [ ! -f "$PROPERTIES_PLUGIN_SRC" ] || [ ! -f "$PROPERTIES_GTK3_EDITOR_SRC" ] || \
  { [ "$WITH_GTK4" = yes ] && [ ! -f "$PROPERTIES_GTK4_EDITOR_SRC" ]; }; then
  if ! build_properties_plugins; then
    echo "error: missing required properties editor files" >&2
    echo "error: build bin/libnm-vpn-plugin-netbird.so and bin/libnm-vpn-plugin-netbird-editor.so first, or install C build dependencies: cc, pkg-config, libnm, GTK 3, and libnma" >&2
    if [ "$WITH_GTK4" = yes ]; then
      echo "error: GTK 4 editor builds also require gtk4 and libnma-gtk4" >&2
    fi
    exit 1
  fi
fi

require_file "$SERVICE_SRC"
require_file "$AUTH_DIALOG_SRC"
require_file "$PROPERTIES_PLUGIN_SRC"
require_file "$PROPERTIES_GTK3_EDITOR_SRC"
if [ "$WITH_GTK4" = yes ]; then
  require_file "$PROPERTIES_GTK4_EDITOR_SRC"
fi
require_file "$VPN_NAME_SRC"
require_file "$DBUS_POLICY_SRC"
require_file "$NM_UNMANAGED_SRC"

install_file "$SERVICE_SRC" "$LIBEXEC_DIR/nm-netbird-service" 0755
install_file "$AUTH_DIALOG_SRC" "$LIBEXEC_DIR/nm-netbird-auth-dialog" 0755
install_file "$PROPERTIES_PLUGIN_SRC" "$NM_PLUGIN_DIR/libnm-vpn-plugin-netbird.so" 0755
install_file "$PROPERTIES_GTK3_EDITOR_SRC" "$NM_PLUGIN_DIR/libnm-vpn-plugin-netbird-editor.so" 0755
if [ "$WITH_GTK4" = yes ]; then
  install_file "$PROPERTIES_GTK4_EDITOR_SRC" "$NM_PLUGIN_DIR/libnm-gtk4-vpn-plugin-netbird-editor.so" 0755
fi
install_vpn_metadata
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
  1. Make sure the NetBird daemon/runtime is installed and running.
  2. Create a NetworkManager profile with setup-key or SSO auth data, for example:
       nmcli connection add type vpn con-name NetBird vpn-type netbird ifname --
  3. Activate it:
       nmcli connection up NetBird
EOF
