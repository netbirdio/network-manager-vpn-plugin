#!/usr/bin/env sh
set -eu

# Build and validate the SELinux policy module against Fedora's policy store.
#
# The binary .pp format is not portable across distribution policy versions.
# This script prevents accidentally packaging a module compiled against another
# reference policy (for example Ubuntu's selinux-policy-dev) into Fedora/RPM
# artifacts.

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
fedora_image=${FEDORA_IMAGE:-fedora:44}
runtime=${CONTAINER_RUNTIME:-}

if [ -z "$runtime" ]; then
  if command -v docker >/dev/null 2>&1; then
    runtime=docker
  elif command -v podman >/dev/null 2>&1; then
    runtime=podman
  else
    echo "error: docker or podman is required to build the Fedora SELinux policy" >&2
    exit 1
  fi
fi

case "$runtime" in
  podman) volume_arg="$repo_root:/src:Z" ;;
  docker) volume_arg="$repo_root:/src:z" ;;
  *) volume_arg="$repo_root:/src" ;;
esac

"$runtime" run --rm \
  -v "$volume_arg" \
  -w /src \
  "$fedora_image" \
  sh -eu -c '
    dnf install -y make policycoreutils selinux-policy-devel selinux-policy-targeted

    make -C packaging/selinux clean all

    store=$(mktemp -d)
    trap '\''rm -rf "$store"'\'' EXIT
    cp -a /var/lib/selinux/targeted "$store/"
    semodule -s targeted -S "$store" -N -i packaging/selinux/nm_netbird.pp
  '

# Docker commonly writes generated files as root. Restore ownership so later CI
# steps and local cleanups can read/remove the artifacts without sudo.
if [ "$(id -u)" -ne 0 ]; then
  for path in \
    "$repo_root/packaging/selinux/nm_netbird.pp" \
    "$repo_root/packaging/selinux/nm_netbird.mod" \
    "$repo_root/packaging/selinux/nm_netbird.mod.fc" \
    "$repo_root/packaging/selinux/nm_netbird.if" \
    "$repo_root/packaging/selinux/nm_netbird.fc.tmp" \
    "$repo_root/packaging/selinux/tmp"; do
    [ -e "$path" ] || continue
    if chown -R "$(id -u):$(id -g)" "$path" 2>/dev/null; then
      continue
    fi
    if command -v sudo >/dev/null 2>&1; then
      sudo chown -R "$(id -u):$(id -g)" "$path" || true
    fi
  done
fi
