# Contributing

Thanks for helping improve the NetworkManager NetBird plugin.

## Development requirements

Install these tools/packages on your Linux development machine:

- Go `1.26.x` or newer matching `go.mod`
- `dbus-daemon` for D-Bus integration tests
- `gdbus` for manual smoke tests (`glib2`/`libglib2.0-bin` package on many distros)
- `task` for the Taskfile command interface
- `golangci-lint` and `opengrep` for the code-quality tasks
- NetworkManager for end-to-end/manual system-bus testing
- NetBird daemon/runtime for real daemon testing
- C editor build dependencies for `task quality`: `cc`, `pkg-config`, libnm, GTK 3, and libnma development headers

Optional but useful:

- a supported init system for daemon autostart testing (`systemd` currently)
- `nmcli` for NetworkManager profile testing
- GTK 4 and libnma-gtk4 development headers for optional GTK 4 editor builds

## Initial setup

Clone the repository and download Go modules:

```bash
git clone <repo-url>
cd network-manager-vpn-plugin
go mod download
```

Or, with Task:

```bash
task tidy
```

Build everything:

```bash
task build
```

The Go binaries can also be built directly:

```bash
go build -o bin/nm-netbird-service ./cmd/nm-netbird-service
go build -o bin/nm-netbird-auth-dialog ./cmd/nm-netbird-auth-dialog
```

The desktop properties editor uses the common NetworkManager split-loader layout. For local development, `task build:properties` builds the libnm loader and GTK 3 editor; `task build:properties:gtk4` builds the GTK 4 editor. `task test:properties` runs the settings mapping tests. Distro builds may also use the Meson files under `properties/`.

## Local development service

Run on the session bus for safe local D-Bus development:

```bash
./bin/nm-netbird-service --bus=session --debug
```

Or:

```bash
task run:session
```

Run on the system bus only when doing NetworkManager integration testing:

```bash
sudo ./bin/nm-netbird-service --bus=system --debug
```

Useful service flags:

| Flag | Default | Description |
| --- | --- | --- |
| `--bus` | `system` | D-Bus bus: `system` or `session` |
| `--debug` | `false` | Verbose lifecycle and signal logging |
| `--daemon-address` | `unix:///var/run/netbird.sock` | NetBird daemon gRPC endpoint |
| `--start-daemon` | `false` | Ask the configured init system to start NetBird if the first dial fails |
| `--daemon-init-system` | `auto` | Init system for daemon autostart (`auto` or `systemd`) |
| `--daemon-service` | `netbird` | Daemon service name to start |
| `--daemon-dial-timeout` | `3s` | Daemon dial timeout |
| `--daemon-rpc-timeout` | `15s` | Per-RPC timeout when no tighter deadline exists |
| `--activation-timeout` | `90s` | Maximum time to wait for activation phases other than interactive SSO |
| `--sso-wait-timeout` | `10m` | Maximum time to wait for interactive SSO completion |

Environment overrides are also supported:

| Variable | Overrides |
| --- | --- |
| `NM_NETBIRD_DAEMON_ADDRESS` | `--daemon-address` |
| `NM_NETBIRD_DAEMON_DIAL_TIMEOUT` | `--daemon-dial-timeout` |
| `NM_NETBIRD_DAEMON_RPC_TIMEOUT` | `--daemon-rpc-timeout` |
| `NM_NETBIRD_START_DAEMON` | `--start-daemon` |
| `NM_NETBIRD_DAEMON_INIT_SYSTEM` | `--daemon-init-system` |
| `NM_NETBIRD_DAEMON_SERVICE` | `--daemon-service` |

## Testing

Run Go tests:

```bash
go test ./...
```

Run the standard local quality gate, including Go tests, C properties editor tests, lint, and repository pattern checks:

```bash
task quality
```

The test suite includes:

- unit tests for daemon status mapping
- unit tests for profile resolution
- gRPC wrapper tests with a fake daemon server
- D-Bus service tests using a temporary `dbus-daemon`

If D-Bus tests are skipped, install `dbus-daemon`.

## Smoke tests

Build, launch the service on the session bus, introspect it, call basic methods, and stop it:

```bash
task smoke
```

Useful individual smoke tasks:

```bash
task run:session
task dbus:introspect
task dbus:state
task dbus:need-secrets
task dbus:connect
task dbus:connect-interactive
task dbus:disconnect
task dbus:monitor
```

## Real daemon testing

Make sure the NetBird daemon is installed and running:

```bash
sudo systemctl enable --now netbird.service
```

Then run the plugin against the daemon socket:

```bash
sudo ./bin/nm-netbird-service \
  --bus=system \
  --debug \
  --daemon-address=unix:///var/run/netbird.sock
```

To test daemon autostart through the init-system abstraction:

```bash
sudo ./bin/nm-netbird-service \
  --bus=system \
  --debug \
  --start-daemon \
  --daemon-init-system=systemd \
  --daemon-service=netbird
```

## NetworkManager/nmcli testing

For end-to-end testing, install or provide NetworkManager VPN service metadata for VPN type `netbird`, then test a supported authentication flow.

Setup-key flow:

```bash
nmcli connection add type vpn con-name netbird-setup vpn-type netbird ifname --
nmcli connection modify netbird-setup \
  +vpn.data "auth=setup-key,management-url=https://api.netbird.io,admin-url=https://app.netbird.io,interface-name=wt0"
nmcli connection modify netbird-setup \
  +vpn.secrets "setup-key=YOUR_SETUP_KEY"
nmcli connection up netbird-setup
nmcli connection down netbird-setup
```

Interactive SSO flow:

```bash
nmcli connection add type vpn con-name netbird-sso vpn-type netbird ifname --
nmcli connection modify netbird-sso +vpn.data "auth=sso,hint=alice@example.com"
nmcli connection up netbird-sso --ask
nmcli connection down netbird-sso
```

See `README.md` for the complete list of supported `vpn.data` and `vpn.secrets` keys.

## NetBird interface ownership

NetBird owns its interface, routes, DNS, firewall state, and WireGuard configuration. NetworkManager should not manage the NetBird interface.

For local testing, mark NetBird's default `wt*` interface prefix unmanaged:

```bash
sudo tee /etc/NetworkManager/conf.d/90-netbird-unmanaged.conf >/dev/null <<'EOF'
[keyfile]
# NetBird owns its WireGuard interfaces. NetworkManager must not configure them.
unmanaged-devices=interface-name:wt*
EOF

sudo systemctl reload NetworkManager
```

If a test or profile uses a custom daemon `interfaceName` / VPN `interface-name` outside `wt*`, update this file before activation so NetworkManager does not race NetBird for interface, IP, DNS, or route ownership.

## Coding guidelines

- Keep NetworkManager/D-Bus code independent from generated gRPC bindings.
- Use `internal/netbird/daemonclient.Client` for daemon operations.
- Keep status mapping in `internal/netbird/status`.
- Keep profile resolution in `internal/netbird/profile`.
- Add unit tests for status/profile/auth/activation behavior whenever possible.
- Do not vendor the full NetBird repository; only the daemon proto contract is vendored.

## Pragmatic Go style

This repository values pragmatic, explicit, locally understandable Go.

## Release artifacts

Release artifacts are produced by GoReleaser (`.goreleaser.yml`) and published by `.github/workflows/release.yml` when pushing `v*` tags. Snapshot artifacts are built by the same workflow for non-tag builds.

The release workflow builds precompiled amd64/x86_64 NetworkManager desktop editor modules with `scripts/build-release-properties.sh` before GoReleaser packages `.deb`, `.rpm`, and tarball artifacts.

## Before submitting changes

Run the standard quality gate:

```bash
task quality
```

For riskier changes, run the full gate, including race tests:

```bash
task quality:full
```

If your change affects D-Bus behavior, also run:

```bash
task smoke
```
