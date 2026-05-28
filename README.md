# NetworkManager NetBird Plugin

A NetworkManager VPN plugin that controls the local NetBird daemon over gRPC.

NetworkManager is only a control/status frontend in this integration. NetBird remains responsible for the WireGuard interface, routes, DNS, firewall rules, profiles, and authentication/session state. The plugin still emits the minimal NetworkManager VPN `Config` signal required to mark activation complete, with no NM-managed IP configuration.

## Requirements

- Linux with NetworkManager
- NetBird daemon/runtime installed
- NetBird daemon gRPC socket available, by default:
  - `unix:///var/run/netbird.sock`
- NetworkManager VPN service metadata installed for VPN type `netbird` (packaging target)

For development you can run the service directly on the session bus and use the `Taskfile.yml` D-Bus smoke tasks.

## Install

The preferred distribution model is a distro package. This repository does not ship curl-to-shell installation automation.

Package/manual installs should provide:

- the `nm-netbird-service` binary in the runtime libexec directory
- NetworkManager VPN metadata for VPN type `netbird`
- D-Bus system policy for `org.freedesktop.NetworkManager.netbird`
- NetworkManager unmanaged-interface config for NetBird-owned interfaces

### System D-Bus security model

Installed system-bus policy only allows root-owned system components to own or send directly to `org.freedesktop.NetworkManager.netbird`. Normal users should not call the VPN plugin service directly.

Users interact with NetworkManager through `nmcli` or desktop frontends, NetworkManager applies PolicyKit and connection permission checks, NetworkManager talks to the NetBird VPN plugin, and the plugin talks to the local NetBird daemon.

This does not change session-bus development workflows (`--bus=session`) or NetBird daemon socket permissions. Direct system-bus debugging calls to `org.freedesktop.NetworkManager.netbird` should be run as root.

Distribution packages usually handle D-Bus policy reloads or service restarts. For manual installs, after changing the system policy file, reload/restart D-Bus and NetworkManager as appropriate for the distro. If system-bus restart is not supported safely, reboot before verifying the policy.

Release artifacts are produced by GoReleaser (`.goreleaser.yml`) and published by `.github/workflows/release.yml` when pushing `v*` tags.

## Build

```bash
go build -o bin/nm-netbird-service ./cmd/nm-netbird-service
```

## Running the service

Development session bus:

```bash
./bin/nm-netbird-service --bus=session --debug
```

System bus, as NetworkManager would use it:

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

## NetworkManager unmanaged interface

NetBird should own its interface. Package/manual installs should mark NetBird's default `wt*` interface prefix unmanaged:

```bash
sudo tee /etc/NetworkManager/conf.d/90-netbird-unmanaged.conf >/dev/null <<'EOF'
[keyfile]
# NetBird owns its WireGuard interfaces. NetworkManager must not configure them.
unmanaged-devices=interface-name:wt*
EOF

sudo systemctl reload NetworkManager
```

If you configure a custom daemon `interfaceName` or set `interface-name=` in `vpn.data` to a name outside `wt*`, update `90-netbird-unmanaged.conf` before activating the profile. Otherwise NetworkManager may manage the daemon-created interface and race NetBird for IP, DNS, and route ownership.

## nmcli usage

These examples assume the plugin service metadata has registered VPN type `netbird` with NetworkManager.

### Existing daemon login

Each NetworkManager connection maps to its own NetBird profile by default, named `nm-<NetworkManager connection UUID>`. Use this when that daemon profile already has a valid session, or set `profile-name` to an existing daemon profile:

```bash
nmcli connection add type vpn con-name netbird vpn-type netbird ifname --
nmcli connection up netbird
```

Activation maps to daemon `Up`; deactivation maps to daemon `Down`:

```bash
nmcli connection down netbird
```

### Setup-key login

For non-interactive first activation with a setup key:

```bash
nmcli connection add type vpn con-name netbird-setup vpn-type netbird ifname --

nmcli connection modify netbird-setup \
  +vpn.data "auth=setup-key,management-url=https://api.netbird.io,admin-url=https://app.netbird.io,interface-name=wt0"

nmcli connection modify netbird-setup \
  +vpn.secrets "setup-key=YOUR_SETUP_KEY"

nmcli connection up netbird-setup
```

### Profile selection

By default, every NetworkManager connection gets a separate NetBird profile named `nm-<connection UUID>` (falling back to a sanitized connection ID only if NetworkManager did not provide a UUID). You can override that mapping with `profile-name` and optional `username`.

NetBird still supports one active daemon engine. If a different profile is connected or connecting, the plugin fails safely instead of switching the active session. Switching between different NetworkManager-backed profiles is allowed once the daemon is disconnected.

```bash
nmcli connection add type vpn con-name netbird-prod vpn-type netbird ifname --

nmcli connection modify netbird-prod \
  +vpn.data "profile-name=prod,username=alice@example.com"

nmcli connection up netbird-prod
```

### SSO login

For `nmcli`, prefer an existing daemon login:

```bash
netbird login
nmcli connection up netbird
```

Interactive SSO for the current MVP is exposed through NetworkManager's interactive VPN flow in `nmcli --ask`. The service emits the verification URL/user code as a login banner and waits for daemon `WaitSSOLogin` completion using the longer SSO wait timeout.

```bash
nmcli connection add type vpn con-name netbird-sso vpn-type netbird ifname --
nmcli connection modify netbird-sso +vpn.data "auth=sso,hint=alice@example.com"
nmcli connection up netbird-sso --ask
```

Desktop NetworkManager auth-dialog support is not shipped yet. For desktop GUI use, log in with `netbird login` first, or use the `nmcli --ask` flow above.

## VPN data/secrets keys

The plugin reads keys from NetworkManager `vpn.data` and `vpn.secrets`. Store sensitive values in `vpn.secrets`.

| Key | Aliases | Description |
| --- | --- | --- |
| `auth` | `auth-mode`, `authentication`, `login-mode` | Auth behavior. Values: `setup-key`, `sso`, `login`; omit to reuse daemon auth |
| `setup-key` | `setupKey`, `netbird-setup-key` | NetBird setup key secret |
| `management-url` | `managementUrl`, `netbird-management-url` | Management URL for daemon login |
| `admin-url` | `adminURL`, `netbird-admin-url` | Admin URL for daemon login |
| `profile-name` | `profileName`, `profile`, `netbird-profile-name` | NetBird daemon profile name; defaults to `nm-<NetworkManager connection UUID>` |
| `username` | `user-name`, `user`, `netbird-username` | NetBird profile username |
| `hostname` | `host-name` | Hostname sent during daemon login |
| `interface-name` | `interfaceName`, `netbird-interface-name` | Desired NetBird interface name |
| `pre-shared-key` | `preshared-key`, `preSharedKey` | Optional WireGuard pre-shared key |
| `hint` | `login-hint`, `sso-hint` | SSO login hint, commonly an email address |

## Inspecting and troubleshooting

Connection status:

```bash
nmcli connection show --active
nmcli connection show netbird
```

Service logs depend on how the plugin is launched. For a systemd-managed service, use:

```bash
journalctl -u nm-netbird-service -f
journalctl -u netbird -f
```

Development D-Bus helpers:

```bash
task run:session
task dbus:introspect
task dbus:state
task dbus:connect
task dbus:disconnect
```