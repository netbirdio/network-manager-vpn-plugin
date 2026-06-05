# NetworkManager NetBird Plugin

A NetworkManager VPN plugin that controls the local NetBird daemon over gRPC.

NetworkManager is only a control/status frontend in this integration. NetBird remains responsible for the WireGuard interface, routes, DNS, firewall rules, profiles, and authentication/session state. The plugin still emits the minimal NetworkManager VPN `Config` signal required to mark activation complete, with no NM-managed IP configuration.

## Requirements

- Linux with NetworkManager
- NetBird daemon/runtime installed
- NetBird daemon gRPC socket available, by default:
  - `unix:///var/run/netbird.sock`
- NetworkManager VPN service metadata installed for VPN type `netbird` (packaging target)
- For building the desktop properties editor from source: `cc`, `pkg-config`, libnm, GTK 3, and libnma development headers
- Optional GTK 4 editor builds additionally require GTK 4 and libnma-gtk4 development headers

For development you can run the service directly on the session bus and use the `Taskfile.yml` D-Bus smoke tasks.

## Install

The preferred install path is the native package from the GitHub release. This repository does not ship curl-to-shell installation automation.

### Ubuntu/Debian quick start

1. Install NetworkManager, curl, and the NetBird daemon/runtime. If you already configured the NetBird apt repository, this is usually:

   ```bash
   sudo apt update
   sudo apt install network-manager curl netbird
   sudo systemctl enable --now NetworkManager netbird
   ```

2. Download the latest `.deb` package. Native `.deb`/`.rpm` packages currently include prebuilt desktop editor modules for amd64/x86_64; use the tarball fallback below on other architectures. Set `RELEASE=snapshot` instead of `latest` if you want the continuously updated snapshot release:

   ```bash
   RELEASE=latest
   ARCH=$(dpkg --print-architecture)
   case "$ARCH" in
     amd64) ASSET_ARCH=amd64 ;;
     *) echo "native packages are currently published for amd64 only; use the tarball fallback on $ARCH" >&2; exit 1 ;;
   esac
   API_URL="https://api.github.com/repos/netbirdio/network-manager-vpn-plugin/releases/latest"
   [ "$RELEASE" = latest ] || API_URL="https://api.github.com/repos/netbirdio/network-manager-vpn-plugin/releases/tags/$RELEASE"
   PACKAGE_URL=$(curl -fsSL "$API_URL" | grep -Eo 'https://github.com/netbirdio/network-manager-vpn-plugin/releases/download/[^" ]+/network-manager-netbird_[^" ]+_linux_'"${ASSET_ARCH}"'\.deb' | head -n1)
   test -n "$PACKAGE_URL"
   curl -fL "$PACKAGE_URL" -o network-manager-netbird.deb
   ```

   You can also download `network-manager-netbird_*.deb` manually from the repository's GitHub Releases page.

3. Install the package:

   ```bash
   sudo apt install ./network-manager-netbird.deb
   ```

   The package installs the NetworkManager VPN service, D-Bus policy, auth-dialog helper, and unmanaged-interface config for NetBird interfaces. The post-install script reloads D-Bus policy and NetworkManager when possible.

4. Verify that NetworkManager can see the VPN type:

   ```bash
   nmcli connection add type vpn con-name NetBird vpn-type netbird ifname --
   nmcli connection show NetBird
   ```

5. Configure one authentication path, then activate the connection. For example, with a setup key:

   ```bash
   nmcli connection modify NetBird \
     +vpn.data "auth=setup-key,management-url=https://api.netbird.io,admin-url=https://app.netbird.io"
   nmcli connection modify NetBird \
     +vpn.secrets "setup-key=YOUR_SETUP_KEY"
   nmcli connection up NetBird
   ```

   For SSO, use the interactive flow instead:

   ```bash
   nmcli connection modify NetBird +vpn.data "auth=sso,hint=alice@example.com"
   nmcli connection up NetBird --ask
   ```

If `vpn-type netbird` is not recognized after package install, restart NetworkManager and try again:

```bash
sudo systemctl restart NetworkManager
```

### Fedora/RHEL-like distributions

1. Install NetworkManager, curl, and the NetBird daemon/runtime. If you already configured the NetBird dnf/yum repository, this is usually:

   ```bash
   sudo dnf install NetworkManager curl netbird
   sudo systemctl enable --now NetworkManager netbird
   ```

2. Download the latest `.rpm` package. Native `.deb`/`.rpm` packages currently include prebuilt desktop editor modules for amd64/x86_64; use the tarball fallback below on other architectures. Set `RELEASE=snapshot` instead of `latest` if you want the continuously updated snapshot release:

   ```bash
   RELEASE=latest
   ARCH=$(uname -m)
   case "$ARCH" in
     x86_64) ASSET_ARCH=amd64 ;;
     *) echo "native packages are currently published for x86_64 only; use the tarball fallback on $ARCH" >&2; exit 1 ;;
   esac
   API_URL="https://api.github.com/repos/netbirdio/network-manager-vpn-plugin/releases/latest"
   [ "$RELEASE" = latest ] || API_URL="https://api.github.com/repos/netbirdio/network-manager-vpn-plugin/releases/tags/$RELEASE"
   PACKAGE_URL=$(curl -fsSL "$API_URL" | grep -Eo 'https://github.com/netbirdio/network-manager-vpn-plugin/releases/download/[^" ]+/network-manager-netbird_[^" ]+_linux_'"${ASSET_ARCH}"'\.rpm' | head -n1)
   test -n "$PACKAGE_URL"
   curl -fL "$PACKAGE_URL" -o network-manager-netbird.rpm
   ```

   You can also download `network-manager-netbird_*.rpm` manually from the repository's GitHub Releases page.

3. Install the package:

   ```bash
   sudo dnf install ./network-manager-netbird.rpm
   ```

4. Verify that NetworkManager can see the VPN type:

   ```bash
   nmcli connection add type vpn con-name NetBird vpn-type netbird ifname --
   nmcli connection show NetBird
   ```

5. Configure one authentication path, then activate the connection. For example, with a setup key:

   ```bash
   nmcli connection modify NetBird \
     +vpn.data "auth=setup-key,management-url=https://api.netbird.io,admin-url=https://app.netbird.io"
   nmcli connection modify NetBird \
     +vpn.secrets "setup-key=YOUR_SETUP_KEY"
   nmcli connection up NetBird
   ```

   For SSO, use the interactive flow instead:

   ```bash
   nmcli connection modify NetBird +vpn.data "auth=sso,hint=alice@example.com"
   nmcli connection up NetBird --ask
   ```

If `vpn-type netbird` is not recognized after package install, restart NetworkManager and try again:

```bash
sudo systemctl restart NetworkManager
```

### Manual tarball fallback

Use the tarball on distributions where the native package is not suitable:

```bash
tar xf nm-netbird-service_linux_amd64.tar.gz
cd nm-netbird-service_linux_amd64
sudo ./install.sh
```

The tarball also includes `uninstall.sh`. Both scripts accept `DESTDIR` plus path overrides such as `LIBEXEC_DIR`, `NM_PLUGIN_DIR`, `NM_VPN_DIR`, `DBUS_POLICY_DIR`, and `NM_CONF_DIR` for staging or distro-specific layouts. By default, `NM_VPN_DIR` is `/etc/NetworkManager/VPN`, where NetworkManager discovers local VPN service metadata.

If the tarball does not include prebuilt desktop properties editor modules, `install.sh` builds the libnm loader and GTK 3 editor from bundled C sources when `cc`, `pkg-config`, libnm, GTK 3, and libnma development headers are installed.

GTK 4 editor support is optional. When GTK 4 and libnma-gtk4 development packages are available, the installer also builds and installs the GTK 4 editor unless `WITH_GTK4=no` is set. To require GTK 4 editor installation explicitly, run:

```bash
sudo WITH_GTK4=yes ./install.sh
```

### Installed files

Native `.deb`/`.rpm` packages are currently published for amd64/x86_64 and install:

- the `nm-netbird-service` binary in the runtime libexec directory
- the `nm-netbird-auth-dialog` helper in the runtime libexec directory
- the `libnm-vpn-plugin-netbird.so` desktop properties loader in the NetworkManager plugin directory
- the `libnm-vpn-plugin-netbird-editor.so` GTK 3 editor module in the same directory
- the `libnm-gtk4-vpn-plugin-netbird-editor.so` GTK 4 editor module in the same directory
- NetworkManager VPN metadata for VPN type `netbird` in NetworkManager's VPN service directory
- D-Bus system policy for `org.freedesktop.NetworkManager.netbird`
- NetworkManager unmanaged-interface config for NetBird-owned interfaces

The tarball/source installer builds and installs desktop editor modules when the local C build dependencies are available.

The `[libnm]` section in `/etc/NetworkManager/VPN/nm-netbird-service.name` should keep pointing at `/usr/lib/NetworkManager/libnm-vpn-plugin-netbird.so`; that loader selects the GTK 3 or GTK 4 editor module at runtime.

### System D-Bus security model

Installed system-bus policy only allows root-owned system components to own or send directly to `org.freedesktop.NetworkManager.netbird`. Normal users should not call the VPN plugin service directly.

Users interact with NetworkManager through `nmcli` or desktop frontends, NetworkManager applies PolicyKit and connection permission checks, NetworkManager talks to the NetBird VPN plugin, and the plugin talks to the local NetBird daemon.

This does not change session-bus development workflows (`--bus=session`) or NetBird daemon socket permissions. Direct system-bus debugging calls to `org.freedesktop.NetworkManager.netbird` should be run as root.

Distribution packages usually handle D-Bus policy reloads or service restarts. For manual installs, after changing the system policy file, reload/restart D-Bus and NetworkManager as appropriate for the distro. If system-bus restart is not supported safely, reboot before verifying the policy.

Release artifacts are produced by GoReleaser (`.goreleaser.yml`) and published by `.github/workflows/release.yml` when pushing `v*` tags.

## Build

```bash
task build
```

The Go binaries can still be built directly:

```bash
go build -o bin/nm-netbird-service ./cmd/nm-netbird-service
go build -o bin/nm-netbird-auth-dialog ./cmd/nm-netbird-auth-dialog
```

The desktop properties editor uses the common NetworkManager split-loader layout. For local development `task build:properties` builds the libnm loader and GTK 3 editor; `task build:properties:gtk4` builds the GTK 4 editor. `task test:properties` runs the settings mapping tests; distro builds may also use the Meson files under `properties/`.

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

Each NetworkManager connection maps to its own NetBird profile, named `nm-<NetworkManager connection UUID>`.

```bash
nmcli connection add type vpn con-name netbird vpn-type netbird ifname --
nmcli connection up netbird
```

Activation maps to daemon `Up`; deactivation maps to daemon `Down`:

```bash
nmcli connection down netbird
```

### Setup-key login

For non-interactive first activation with a setup key. If `username` is omitted, the plugin derives the NetBird daemon profile owner from NetworkManager connection permissions when available, then from a matching active daemon profile or the service process user. For system-wide NetworkManager profiles, that service process user is usually `root`; set `username` explicitly if you need another owner.

```bash
nmcli connection add type vpn con-name netbird-setup vpn-type netbird ifname --

nmcli connection modify netbird-setup \
  +vpn.data "auth=setup-key,management-url=https://api.netbird.io,admin-url=https://app.netbird.io,interface-name=wt0"

nmcli connection modify netbird-setup \
  +vpn.secrets "setup-key=YOUR_SETUP_KEY"

nmcli connection up netbird-setup
```

### Profile mapping

Every NetworkManager connection gets a separate NetBird profile named `nm-<connection UUID>` (falling back to a sanitized connection ID only if NetworkManager did not provide a UUID). The profile owner username is inferred from NetworkManager connection permissions when possible, then from a matching active daemon profile or the service process user. For system-wide NetworkManager profiles, that service process user is usually `root`; set `username` explicitly if you need another owner.

NetBird still supports one active daemon engine. If a different profile is connected or connecting, the plugin fails safely instead of switching the active session. Switching between different NetworkManager-backed profiles is allowed once the daemon is disconnected.

### SSO login

For `nmcli`, prefer an existing daemon login:

```bash
netbird login
nmcli connection up netbird
```

Interactive SSO is exposed through NetworkManager's interactive VPN flow in `nmcli --ask`. The service emits the verification URL/user code as a login banner and waits for daemon `WaitSSOLogin` completion using the longer SSO wait timeout.

```bash
nmcli connection add type vpn con-name netbird-sso vpn-type netbird ifname --
nmcli connection modify netbird-sso +vpn.data "auth=sso,hint=alice@example.com"
nmcli connection up netbird-sso --ask
```

Desktop NetworkManager frontends can discover the packaged `nm-netbird-auth-dialog` helper. The helper can request a missing setup key and show NetBird SSO verification URL/user-code hints. Browser-opening/progress controls depend on the frontend; the service never opens UI or browser windows.

## Desktop profile editor

The packaged libnm editor plugin lets desktop NetworkManager frontends such as GNOME Settings or `nm-connection-editor` create and edit NetBird VPN profiles. The shared loader selects the GTK 3 or GTK 4 editor module at runtime based on the frontend.

The editor writes the same `vpn.data` and `vpn.secrets` keys used by `nmcli`:

- setup keys and pre-shared keys are saved in `vpn.secrets`
- management/admin URLs, username, interface name, hostname, auth mode, and SSO login hint are saved in `vpn.data`
- unknown existing NetBird data/secrets are preserved when saving

The editor validates only local field syntax. It does not contact the NetBird daemon while editing.

## VPN data/secrets keys

The plugin reads keys from NetworkManager `vpn.data` and `vpn.secrets`. Store sensitive values in `vpn.secrets`.

| Key | Aliases | Description |
| --- | --- | --- |
| `auth` | `auth-mode`, `authentication`, `login-mode` | Auth behavior. Values: `setup-key` or `sso`; omit to use an existing daemon session. Legacy `login`/`reuse` values are accepted for compatibility but are not exposed by the editor |
| `setup-key` | `setupKey`, `netbird-setup-key` | NetBird setup key secret |
| `management-url` | `managementUrl`, `netbird-management-url` | Management URL for daemon login |
| `admin-url` | `adminURL`, `netbird-admin-url` | Admin URL for daemon login |
| `username` | `user-name`, `user`, `netbird-username` | NetBird daemon profile owner username; inferred when omitted |
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