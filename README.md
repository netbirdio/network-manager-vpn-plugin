# NetworkManager NetBird Plugin

A NetworkManager VPN plugin that controls the local NetBird daemon over gRPC.

NetworkManager is only a control/status frontend in this integration. NetBird remains responsible for the WireGuard interface, routes, DNS, firewall rules, profiles, and authentication/session state. The plugin emits the minimal NetworkManager VPN `Config` signal required to mark activation complete, with no NM-managed IP configuration.

## Requirements

- Linux with NetworkManager
- NetBird daemon/runtime installed (see the [official NetBird Linux installation guide](https://docs.netbird.io/get-started/install/linux))
- NetBird daemon gRPC socket available (default: `unix:///var/run/netbird.sock`)
When building the desktop properties editor from source:

- `cc`, `pkg-config`, libnm, GTK 3, and libnma development headers
- Optional GTK 4 editor builds additionally require GTK 4 and libnma-gtk4 development headers

## Documentation

| Document | What it covers |
| --- | --- |
| [Reference](docs/reference.md) | Service flags, environment variables, VPN data/secrets keys, D-Bus interface, gRPC surface, installed files, build targets. |
| [Architecture](docs/architecture.md) | How the plugin works, activation lifecycle, profile mapping, security model, editor design, why the plugin doesn't manage IP. |

## Quick install

### One-line quickstart

For Ubuntu/Debian/Fedora/RHEL on `amd64`/`x86_64`, the quickstart script installs NetworkManager prerequisites, enables NetworkManager, downloads the latest package, and installs it. The NetBird daemon/runtime requirement above still applies:

```bash
curl -fsSL https://raw.githubusercontent.com/netbirdio/network-manager-vpn-plugin/main/scripts/quickstart.sh | sh
```

To review it before running:

```bash
curl -fsSLO https://raw.githubusercontent.com/netbirdio/network-manager-vpn-plugin/main/scripts/quickstart.sh
less quickstart.sh
sh quickstart.sh
```

For snapshot releases:

```bash
curl -fsSL https://raw.githubusercontent.com/netbirdio/network-manager-vpn-plugin/main/scripts/quickstart.sh | env RELEASE_TAG=snapshot sh
```

### Ubuntu/Debian

```bash
sudo apt update
sudo apt install network-manager curl
sudo systemctl enable --now NetworkManager

curl -fL -o network-manager-netbird_linux_amd64.deb "$(curl -fsSL https://api.github.com/repos/netbirdio/network-manager-vpn-plugin/releases/latest | grep '"browser_download_url":' | grep -E 'network-manager-netbird.*(_linux_amd64|_amd64|\.x86_64)\.deb"' | cut -d '"' -f 4 | head -n 1)"
sudo apt install ./network-manager-netbird_linux_amd64.deb
```

### Fedora/RHEL

```bash
sudo dnf install NetworkManager curl
sudo systemctl enable --now NetworkManager

RELEASE_API=https://api.github.com/repos/netbirdio/network-manager-vpn-plugin/releases/latest
asset_url="$(curl -fsSL "$RELEASE_API" |
  grep '"browser_download_url":' |
  grep -E 'network-manager-netbird.*(_linux_amd64|_amd64|\.x86_64)\.rpm"' |
  cut -d '"' -f 4 |
  head -n 1)"
curl -fL -o network-manager-netbird_linux_amd64.rpm "$asset_url"
sudo dnf install ./network-manager-netbird_linux_amd64.rpm
```

### Other distributions (tarball)

```bash
curl -fLO https://github.com/netbirdio/network-manager-vpn-plugin/releases/latest/download/nm-netbird-service_linux_amd64.tar.gz
tar xf nm-netbird-service_linux_amd64.tar.gz
cd nm-netbird-service_linux_amd64
sudo ./install.sh
```

Substitute `amd64` with `arm64` or `armv7` as needed.

For snapshot releases, use `RELEASE_TAG=snapshot` with the quickstart script, set `RELEASE_API=https://api.github.com/repos/netbirdio/network-manager-vpn-plugin/releases/tags/snapshot` in the package examples, or replace `latest/download` with `download/snapshot` in the tarball URL.

### Verify

```bash
test -f /etc/NetworkManager/VPN/nm-netbird-service.name && echo "service metadata installed"

nmcli connection add type vpn con-name netbird-test vpn-type netbird ifname --
nmcli connection show netbird-test
nmcli connection delete netbird-test
```

## Quick start

NetworkManager profiles can set `auth=sso` or `auth=setup-key`. If `auth` is omitted, or if a legacy `login` / `reuse` value is used, the service treats the profile as `auth=sso`.

**Setup-key login:**

```bash
nmcli connection add type vpn con-name netbird-setup vpn-type netbird ifname --
nmcli connection modify netbird-setup \
  +vpn.data "auth=setup-key,management-url=https://api.netbird.io,admin-url=https://app.netbird.io"
nmcli connection modify netbird-setup \
  +vpn.secrets "setup-key=YOUR_SETUP_KEY"
nmcli connection up netbird-setup
```

**SSO login:**

```bash
nmcli connection add type vpn con-name netbird-sso vpn-type netbird ifname --
nmcli connection modify netbird-sso +vpn.data "auth=sso,hint=alice@example.com"
nmcli connection up netbird-sso --ask
```

Deactivate a connection by name:

```bash
nmcli connection down netbird-sso
```

## Unmanaged interface

NetBird owns its WireGuard interfaces. Mark `wt*` as unmanaged so NetworkManager does not touch them:

```bash
sudo tee /etc/NetworkManager/conf.d/90-netbird-unmanaged.conf >/dev/null <<'EOF'
[keyfile]
unmanaged-devices=interface-name:wt*
EOF
sudo systemctl reload NetworkManager
```

If you configure a custom interface name outside `wt*`, update `90-netbird-unmanaged.conf` before activating.

## Build from source

```bash
task deps:install # install system build/test dependencies (Debian/Ubuntu/Arch/Fedora/RHEL)
task build        # Go binaries + GTK 3 editor
task test         # unit tests
task run:session  # development on the session bus
```

See the [Reference](docs/reference.md#build-targets) for all build targets.

## Troubleshooting

```bash
# Connection status
nmcli connection show --active
nmcli connection show netbird

# Service logs
journalctl -u nm-netbird-service -f
journalctl -u netbird -f

# Development D-Bus introspection
task dbus:introspect
task dbus:state
```

See the [Architecture](docs/architecture.md#activation-lifecycle) doc for the activation lifecycle and common failure points.

## Upgrade and uninstall

```bash
# Upgrade a package install: download the newer .deb/.rpm and install it the same way.
sudo apt install ./network-manager-netbird_linux_amd64.deb   # Debian
sudo dnf install ./network-manager-netbird_linux_amd64.rpm   # Fedora

# Uninstall
sudo apt remove network-manager-netbird   # Ubuntu/Debian
sudo dnf remove network-manager-netbird   # Fedora/RHEL
```

For tarball installs, run the bundled `uninstall.sh`.

## License

See [LICENSE](LICENSE).
