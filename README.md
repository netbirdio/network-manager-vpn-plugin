# NetworkManager NetBird Plugin

A NetworkManager VPN plugin that controls the local NetBird daemon over gRPC.

NetworkManager is only a control/status frontend in this integration. NetBird remains responsible for the WireGuard interface, routes, DNS, firewall rules, profiles, and authentication/session state. The plugin still emits the minimal NetworkManager VPN `Config` signal required to mark activation complete, with no NM-managed IP configuration.

## Requirements

- Linux with NetworkManager
- NetBird daemon/runtime installed
- NetBird daemon gRPC socket available, by default:
  - `unix:///var/run/netbird.sock`
- NetworkManager VPN service metadata installed for VPN type `netbird` (packaging target)
## Install

The preferred install path is the native package from the GitHub release. This repository does not ship curl-to-shell installation automation.

### Ubuntu/Debian quick start

1. Install NetworkManager, curl, and the NetBird daemon/runtime. Follow the [NetBird Ubuntu/Debian apt install guide](https://docs.netbird.io/get-started/install/linux#ubuntu-debian-apt) to configure the NetBird apt repository first. Once that repository is configured, this is usually:

   ```bash
   sudo apt update
   sudo apt install network-manager curl netbird
   sudo systemctl enable --now NetworkManager netbird
   ```

2. Download the latest `.deb` package. Native `.deb`/`.rpm` packages currently include prebuilt desktop editor modules for amd64/x86_64; use the tarball fallback below on other architectures.

   ```bash
   PACKAGE_URL=$(curl -fsSL https://api.github.com/repos/netbirdio/network-manager-vpn-plugin/releases \
     | grep -Eo 'https://github.com/netbirdio/network-manager-vpn-plugin/releases/download/[^" ]+/network-manager-netbird_[^" ]+_linux_amd64\.deb' \
     | head -n1)
   curl -fL "$PACKAGE_URL" -o network-manager-netbird.deb
   ```

   You can also download `network-manager-netbird_*.deb` manually from the repository's GitHub Releases page.

3. Install the package:

   ```bash
   sudo apt install ./network-manager-netbird.deb
   ```

   The package installs the NetworkManager VPN service, D-Bus policy, auth-dialog helper, desktop editor modules, and unmanaged-interface config for NetBird interfaces. The post-install script reloads D-Bus policy and NetworkManager when possible.

If NetworkManager does not recognize `vpn-type netbird` after package install, restart NetworkManager and try again:

```bash
sudo systemctl restart NetworkManager
```

After installation, create a VPN profile with your desktop NetworkManager frontend or the examples in [nmcli usage](#nmcli-usage).

### Fedora/RHEL-like distributions

1. Install NetworkManager, curl, and the NetBird daemon/runtime. Follow the [NetBird Fedora/Amazon Linux 2023 dnf install guide](https://docs.netbird.io/get-started/install/linux#fedora-amazon-linux-2023-dnf) to configure the NetBird dnf repository first. Once that repository is configured, this is usually:

   ```bash
   sudo dnf install NetworkManager curl netbird
   sudo systemctl enable --now NetworkManager netbird
   ```

2. Download the latest `.rpm` package. Native `.deb`/`.rpm` packages currently include prebuilt desktop editor modules for amd64/x86_64; use the tarball fallback below on other architectures.

   ```bash
   PACKAGE_URL=$(curl -fsSL https://api.github.com/repos/netbirdio/network-manager-vpn-plugin/releases \
     | grep -Eo 'https://github.com/netbirdio/network-manager-vpn-plugin/releases/download/[^" ]+/network-manager-netbird_[^" ]+_linux_amd64\.rpm' \
     | head -n1)
   curl -fL "$PACKAGE_URL" -o network-manager-netbird.rpm
   ```

   You can also download `network-manager-netbird_*.rpm` manually from the repository's GitHub Releases page.

3. Install the package:

   ```bash
   sudo dnf install ./network-manager-netbird.rpm
   ```

   The package installs the NetworkManager VPN service, D-Bus policy, auth-dialog helper, desktop editor modules, and unmanaged-interface config for NetBird interfaces. The post-install script reloads D-Bus policy and NetworkManager when possible.

If NetworkManager does not recognize `vpn-type netbird` after package install, restart NetworkManager and try again:

```bash
sudo systemctl restart NetworkManager
```

After installation, create a VPN profile with your desktop NetworkManager frontend or the examples in [nmcli usage](#nmcli-usage).

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

For system D-Bus policy details, see [System D-Bus security model](docs/d-bus-security-model.md).

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

### Setup-key login

For non-interactive first activation with a setup key. If `username` is omitted, the plugin derives the NetBird daemon profile owner from NetworkManager connection permissions when available or from the service process user. For system-wide NetworkManager profiles, that service process user is usually `root`; set `username` explicitly if you need another owner.

```bash
nmcli connection add type vpn con-name netbird-setup vpn-type netbird ifname --

nmcli connection modify netbird-setup \
  +vpn.data "auth=setup-key,management-url=https://api.netbird.io,admin-url=https://app.netbird.io,interface-name=wt0"

nmcli connection modify netbird-setup \
  +vpn.secrets "setup-key=YOUR_SETUP_KEY"

nmcli connection up netbird-setup
```

Activation maps to daemon `Up`; deactivation maps to daemon `Down`:

```bash
nmcli connection down netbird-setup
```

### SSO login

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
| `auth` | `auth-mode`, `authentication`, `login-mode` | Auth behavior. Supported values: `setup-key` or `sso` |
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
