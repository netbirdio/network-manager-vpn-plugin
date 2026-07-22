#!/usr/bin/env sh
set -eu

# Verify that every binary RPM in the dist directory carries a valid GPG
# signature from the published NetBird yum repository key.
#
# CI runs this as a release gate: the RPM signature template in
# .goreleaser.yml silently skips signing when the key file is absent, so an
# unsigned package would otherwise only surface for users installing with
# gpgcheck=1. Source RPMs are skipped (only binary RPMs are signed). The
# check also fails when no binary RPMs are present at all — an empty dist
# directory must not pass while verifying nothing.

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
dist_dir=${1:-$repo_root/dist}
fedora_image=${FEDORA_IMAGE:-fedora:41}
key_url=${RPM_PUBLIC_KEY_URL:-https://pkgs.netbird.io/yum/repodata/repomd.xml.key}
runtime=${CONTAINER_RUNTIME:-}

if [ ! -d "$dist_dir" ]; then
  echo "error: dist directory not found: $dist_dir" >&2
  exit 1
fi

if [ -z "$runtime" ]; then
  if command -v docker >/dev/null 2>&1; then
    runtime=docker
  elif command -v podman >/dev/null 2>&1; then
    runtime=podman
  else
    echo "error: docker or podman is required to verify RPM signatures" >&2
    exit 1
  fi
fi

case "$runtime" in
  podman) volume_arg="$dist_dir:/dist:ro,Z" ;;
  docker) volume_arg="$dist_dir:/dist:ro,z" ;;
  *) volume_arg="$dist_dir:/dist:ro" ;;
esac

"$runtime" run --rm \
  -e KEY_URL="$key_url" \
  -v "$volume_arg" \
  "$fedora_image" \
  sh -eu -c '
    dnf install -y -q rpm-sign curl >/dev/null 2>&1
    curl -sSL "$KEY_URL" -o /tmp/rpm-pub.key
    rpm --import /tmp/rpm-pub.key

    echo "=== Verifying RPM signatures ==="
    status=0
    count=0
    for rpm_file in /dist/*.rpm; do
      [ -f "$rpm_file" ] || continue
      case "$rpm_file" in *.src.rpm) continue ;; esac
      count=$((count + 1))
      echo "--- $(basename "$rpm_file") ---"
      output=$(rpm -K "$rpm_file" || true)
      echo "$output"
      case "$output" in
        *"signatures OK"*) ;;
        *)
          echo "ERROR: missing or invalid GPG signature"
          status=1
          ;;
      esac
    done

    if [ "$count" -eq 0 ]; then
      echo "ERROR: no binary RPM files found in /dist to verify"
      status=1
    fi

    exit $status
  '
