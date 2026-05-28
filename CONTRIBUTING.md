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

Optional but useful:

- a supported init system for daemon autostart testing (`systemd` currently)
- `nmcli` for NetworkManager profile testing

## Initial setup

Clone the repository and download Go modules:

```bash
git clone <repo-url>
cd network-manager-plugin
go mod download
```

Or, with Task:

```bash
task tidy
```

Build the service:

```bash
go build -o bin/nm-netbird-service ./cmd/nm-netbird-service
```

Or:

```bash
task build
```

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

## Testing

Run all automated tests:

```bash
go test ./...
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

For end-to-end testing, install or provide NetworkManager VPN service metadata for VPN type `netbird`, then create a profile:

```bash
nmcli connection add type vpn con-name netbird vpn-type netbird ifname --
nmcli connection up netbird
nmcli connection down netbird
```

Setup-key flow:

```bash
nmcli connection add type vpn con-name netbird-setup vpn-type netbird ifname --
nmcli connection modify netbird-setup \
  +vpn.data "auth=setup-key,management-url=https://api.netbird.io,admin-url=https://app.netbird.io,interface-name=wt0"
nmcli connection modify netbird-setup \
  +vpn.secrets "setup-key=YOUR_SETUP_KEY"
nmcli connection up netbird-setup
```

Profile flow:

```bash
nmcli connection add type vpn con-name netbird-prod vpn-type netbird ifname --
nmcli connection modify netbird-prod \
  +vpn.data "profile-name=prod,username=alice@example.com"
nmcli connection up netbird-prod
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

Run before submitting implementation changes:

```bash
task quality
```

For riskier changes, run:

```bash
task quality:full
```

## Before submitting changes

Run the standard quality gate:

```bash
task quality
```

If your change affects D-Bus behavior, also run:

```bash
task smoke
```
