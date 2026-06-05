#!/usr/bin/env sh
set -eu

out_dir=${1:-bin/linux_amd64}
mkdir -p "$out_dir"

echo "building properties editor modules in $out_dir"

cc -Wall -Wextra -fPIC -shared \
  -o "$out_dir/libnm-vpn-plugin-netbird.so" \
  properties/nm-netbird-editor-plugin.c \
  $(pkg-config --cflags --libs libnm) \
  -ldl

cc -Wall -Wextra \
  -DGDK_VERSION_MIN_REQUIRED=GDK_VERSION_3_22 \
  -DGDK_VERSION_MAX_ALLOWED=GDK_VERSION_3_22 \
  -fPIC -shared \
  -o "$out_dir/libnm-vpn-plugin-netbird-editor.so" \
  properties/nm-netbird-editor-model.c \
  properties/nm-netbird-editor.c \
  $(pkg-config --cflags --libs libnm gtk+-3.0 libnma)

cc -Wall -Wextra \
  -DGDK_VERSION_MIN_REQUIRED=GDK_VERSION_4_0 \
  -DGDK_VERSION_MAX_ALLOWED=GDK_VERSION_4_0 \
  -fPIC -shared \
  -o "$out_dir/libnm-gtk4-vpn-plugin-netbird-editor.so" \
  properties/nm-netbird-editor-model.c \
  properties/nm-netbird-editor.c \
  $(pkg-config --cflags --libs libnm gtk4 libnma-gtk4)
