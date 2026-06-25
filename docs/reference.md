# Reference

Consolidated reference for the NetworkManager NetBird VPN plugin.

## Service flags

Flags for `nm-netbird-service`. When installed as a package, NetworkManager launches the service with the default flag values below.

| Flag | Default | Description |
| --- | --- | --- |
| `--bus` | `system` | D-Bus bus: `system` or `session`. |
| `--debug` | `false` | Verbose lifecycle and signal logging to stdout. |
| `--daemon-address` | `unix:///var/run/netbird.sock` | NetBird daemon gRPC endpoint. |
| `--start-daemon` | `false` | Ask the configured init system to start the NetBird daemon if the first dial fails. |
| `--daemon-init-system` | `auto` | Init system for daemon autostart: `auto` (probes systemd) or `systemd`. |
| `--daemon-service` | `netbird` | Daemon service name to start when `--start-daemon` is enabled. |
| `--daemon-dial-timeout` | `3s` | Timeout for dialing the NetBird daemon gRPC socket. |
| `--daemon-rpc-timeout` | `15s` | Per-RPC timeout when no tighter deadline exists on the call. |
| `--activation-timeout` | `90s` | Maximum time to wait for activation phases other than interactive SSO. |
| `--sso-wait-timeout` | `10m` | Maximum time to wait for interactive SSO completion. |

## Environment variable overrides

The following environment variables override the corresponding service flags. Set them in the process environment before launching `nm-netbird-service`.

| Variable | Overrides |
| --- | --- |
| `NM_NETBIRD_DAEMON_ADDRESS` | `--daemon-address` |
| `NM_NETBIRD_DAEMON_DIAL_TIMEOUT` | `--daemon-dial-timeout` |
| `NM_NETBIRD_DAEMON_RPC_TIMEOUT` | `--daemon-rpc-timeout` |
| `NM_NETBIRD_START_DAEMON` | `--start-daemon` |
| `NM_NETBIRD_DAEMON_INIT_SYSTEM` | `--daemon-init-system` |
| `NM_NETBIRD_DAEMON_SERVICE` | `--daemon-service` |

## VPN data keys

Keys stored in NetworkManager `vpn.data`. The plugin reads these values during activation. When it updates the daemon profile, it sends only daemon profile settings through the NetBird daemon gRPC API: management URL, admin URL, and interface name from `vpn.data`, plus PSK from `vpn.secrets`.

| Key | Aliases | Description |
| --- | --- | --- |
| `auth` | `auth-mode`, `authentication`, `login-mode` | Auth behavior. Accepted values: `setup-key`, `sso`. Missing auth and legacy `login` / `reuse` values are normalized to `sso`; they are not separate modes. |
| `management-url` | `managementUrl`, `netbird-management-url` | Management URL for daemon login and profile updates. Defaults to `https://api.netbird.io:443`. |
| `admin-url` | `adminURL`, `netbird-admin-url` | Admin URL for daemon login and profile updates. Defaults to `https://app.netbird.io:443`. |
| `username` | `user-name`, `user`, `netbird-username` | NetBird daemon profile owner username. Inferred from NetworkManager connection permissions or the service process user when omitted. |
| `hostname` | `host-name` | Hostname sent during daemon login. Defaults to the local OS hostname. |
| `interface-name` | `interfaceName`, `netbird-interface-name` | Desired NetBird WireGuard interface name. The daemon defaults to `wt0` (or the next available `wtN`). |

## NetBird profile mapping

For NetworkManager-backed connections, the plugin derives a stable NetBird profile display name from NetworkManager metadata: `nm-<connection UUID>`, falling back to a sanitised connection ID when the UUID is absent. The plugin ignores any user-supplied `profile-name` value in `vpn.data` for this mapping.

During activation, the plugin resolves that display name for the selected username with `ListProfiles`. If it is missing, the plugin calls `AddProfile` before `SwitchProfile` or `Login`. NetBird 0.73+ returns a generated profile ID; the plugin uses that ID internally as the daemon handle for later RPCs while keeping `nm-<UUID>` as the display name. When an older daemon returns an empty ID, the plugin falls back to the display-name handle. Duplicate matching display names are treated as an activation error.

The generated profile ID is not persisted in NetworkManager settings.

## VPN secrets keys

Keys stored in NetworkManager `vpn.secrets`. NetworkManager protects secrets according to its configured permissions model.

| Key | Aliases | Description |
| --- | --- | --- |
| `setup-key` | `setupKey`, `netbird-setup-key` | NetBird setup key. Required when `auth=setup-key`. |
| `pre-shared-key` | `preshared-key`, `preSharedKey` | Optional WireGuard pre-shared key for the daemon interface. |

## Prompt hints

During interactive activation (`ConnectInteractive`), the service emits a `SecretsRequired` signal with hint strings. Desktop auth-dialog helpers use these hints to drive the UI. The table below lists the hint prefixes sent by the service.

| Hint key | Purpose |
| --- | --- |
| `x-netbird-activation-id=` | Activation identifier for matching `NewSecrets` responses to in-flight prompts. Emitted with both setup-key and SSO prompts. |
| `x-netbird-sso=true` | Marks an SSO `SecretsRequired` prompt. |
| `x-netbird-sso-verification-uri=` | SSO device-code verification URL from the NetBird daemon. |
| `x-netbird-sso-verification-uri-complete=` | SSO verification URL with user code pre-filled. |
| `x-netbird-sso-user-code=` | SSO device-code user code. |
| `x-netbird-sso-continue` | Requested in SSO prompts and returned in `NewSecrets` to signal SSO should continue. |
| `x-netbird-sso-cancel=` | Included in `NewSecrets` to signal SSO should be cancelled. |

## Auth-dialog CLI smoke test

`nm-netbird-auth-dialog` normally runs as a NetworkManager auth-dialog helper. It also has a browser smoke-test mode for SSO desktop diagnostics:

```bash
/usr/libexec/nm-netbird-auth-dialog --test-browser 'https://login.netbird.io/device'
```

This validates that the URL is HTTP(S), checks for a desktop-open environment, runs `xdg-open`, and waits up to 5 seconds. Use `--test-browser-timeout 10s` to change the timeout, or `--test-browser-force` to try `xdg-open` even when no desktop environment is detected.

Package installs depend on `xdg-utils` for `xdg-open`. For tarball or manual installs, make sure `xdg-open` is available on `PATH`.

From a source checkout, build first and run `./bin/nm-netbird-auth-dialog --test-browser ...`.

## Service state constants

The `State` D-Bus property uses the following values (matching `NMVpnServiceState` from libnm).

| Value | Constant | Meaning |
| --- | --- | --- |
| `0` | `UNKNOWN` | State not yet known. |
| `1` | `INIT` | Service initialised, not active. |
| `2` | `SHUTDOWN` | Service is shutting down. |
| `3` | `STARTING` | Activation in progress. |
| `4` | `STARTED` | VPN connection active. |
| `5` | `STOPPING` | Deactivation in progress. |
| `6` | `STOPPED` | Service stopped. |

## Failure codes

The `Failure` signal carries a `uint32` reason matching `NMVpnPluginFailure`.

| Value | Constant | Meaning |
| --- | --- | --- |
| `0` | `LOGIN_FAILED` | Daemon login or authentication failed. |
| `1` | `CONNECT_FAILED` | Could not reach the daemon, or `Up` / status polling failed. |
| `2` | `BAD_IP_CONFIG` | Could not derive the gateway address for the NetworkManager `Config` signal from the management or admin URL. |

## D-Bus interface

### Connection details

| Detail | Value |
| --- | --- |
| Bus name | `org.freedesktop.NetworkManager.netbird` |
| Object path | `/org/freedesktop/NetworkManager/VPN/Plugin` |
| Interface | `org.freedesktop.NetworkManager.VPN.Plugin` |

### Methods

| Method | Signature | Description |
| --- | --- | --- |
| `Connect` | `(connection: a{sa{sv}}) → ()` | Start a non-interactive VPN connection. Fails if SSO is needed without an interactive flow. |
| `ConnectInteractive` | `(connection: a{sa{sv}}, details: a{sv}) → ()` | Start a VPN connection with interactive secret prompting. |
| `NeedSecrets` | `(settings: a{sa{sv}}) → (setting_name: s)` | Returns `"vpn"` if the connection needs a setup-key secret; returns `""` otherwise. |
| `NewSecrets` | `(connection: a{sa{sv}}) → ()` | Deliver additional secrets to an in-flight activation prompt. |
| `Disconnect` | `() → ()` | Stop the active VPN connection and close the daemon client. |
| `SetConfig` | `(config: a{sv}) → ()` | No-op. NetBird owns the interface; NetworkManager config is not applied. |
| `SetIp4Config` | `(config: a{sv}) → ()` | No-op. |
| `SetIp6Config` | `(config: a{sv}) → ()` | No-op. |
| `SetFailure` | `(reason: s) → ()` | Cancels any in-flight activation and clears the prompt. |

### Properties

| Property | Type | Access | Description |
| --- | --- | --- | --- |
| `State` | `u` (uint32) | read | Current service state. See [Service state constants](#service-state-constants). |

### Signals

| Signal | Signature | Description |
| --- | --- | --- |
| `StateChanged` | `(state: u)` | Emitted when the service state changes. |
| `SecretsRequired` | `(message: s, secrets: as)` | Emitted when interactive authentication needs a setup-key secret or SSO user confirmation. |
| `Config` | `(config: a{sv})` | Minimal config identifying the daemon-owned tunnel interface and external gateway. |
| `Ip4Config` | `(ip4config: a{sv})` | Not used by this plugin; emitted only if needed for compatibility. |
| `Ip6Config` | `(ip6config: a{sv})` | Not used by this plugin; emitted only if needed for compatibility. |
| `LoginBanner` | `(banner: s)` | Emitted with SSO verification URL and user code, or a message instructing the user to use `--ask`. |
| `Failure` | `(reason: u)` | Emitted when activation fails. See [Failure codes](#failure-codes). |

### D-Bus errors

| Error name | Meaning |
| --- | --- |
| `org.freedesktop.NetworkManager.VPN.Error.Failed` | A daemon or activation operation failed. |
| `org.freedesktop.NetworkManager.VPN.Error.AlreadyStarted` | An activation is already in progress or active. |

## gRPC interface (NetBird daemon)

The plugin communicates with the local NetBird daemon over gRPC at the configured `--daemon-address`. The following daemon RPCs are used during activation and monitoring.

| RPC | Phase | Description |
| --- | --- | --- |
| `Login` | Authentication | Non-interactive daemon login with setup-key, or device-code initiation for SSO, using the resolved profile ID handle when available. |
| `WaitSSOLogin` | Authentication | Polls the daemon for completion of an interactive SSO login. |
| `GetActiveProfile` | Activation | Reads the currently active daemon profile to detect sessions owned by other connections and capture profile IDs. |
| `ListProfiles` | Activation | Lists daemon profiles to resolve the `nm-<UUID>` display name and detect duplicates. |
| `AddProfile` | Activation | Creates the missing `nm-<UUID>` daemon profile before switching or logging in. The returned generated ID is used as the internal handle when present. |
| `SetConfig` | Activation | Updates daemon profile settings (management URL, admin URL, interface name, PSK) from NetworkManager settings using the resolved profile handle. The service wraps this daemon RPC as `UpdateProfile` internally. |
| `GetFeatures` | Activation | Reads daemon feature flags to decide whether profile updates are supported. |
| `Up` | Activation | Starts the daemon engine for the resolved NetBird profile ID/display-name handle. |
| `Down` | Deactivation | Stops the daemon engine. |
| `Status` | Monitoring | Polls daemon connection state and peer status at ~5 second intervals. |
| `GetConfig` | Activation | Reads daemon config for the resolved profile handle to populate the NetworkManager `Config` signal when `vpn.data` omits certain fields. |

## Installed files

Package installs (`.deb`, `.rpm`) place files at the following paths. Tarball installs use the same layout for binaries, NetworkManager metadata, editor modules, and unmanaged-interface config by default, with overrides available through `DESTDIR`, `LIBEXEC_DIR`, `NM_PLUGIN_DIR`, `NM_VPN_DIR`, `DBUS_POLICY_DIR`, and `NM_CONF_DIR`.

| Path | Purpose |
| --- | --- |
| `/usr/libexec/nm-netbird-service` | VPN plugin service binary. |
| `/usr/libexec/nm-netbird-auth-dialog` | Desktop auth-dialog helper for GNOME/KDE. |
| `/usr/lib/NetworkManager/libnm-vpn-plugin-netbird.so` | libnm editor loader (selects GTK 3 or GTK 4 editor at runtime). |
| `/usr/lib/NetworkManager/libnm-vpn-plugin-netbird-editor.so` | GTK 3 editor module. |
| `/usr/lib/NetworkManager/libnm-gtk4-vpn-plugin-netbird-editor.so` | GTK 4 editor module. |
| `/etc/NetworkManager/VPN/nm-netbird-service.name` | VPN service metadata. NetworkManager discovers the plugin through this file. |
| `/usr/share/dbus-1/system.d/nm-netbird-service.conf` | D-Bus system policy for `.deb`/`.rpm` packages (root-only access). Tarball installs default to `/etc/dbus-1/system.d/nm-netbird-service.conf` unless `DBUS_POLICY_DIR` is overridden. |
| `/etc/NetworkManager/conf.d/90-netbird-unmanaged.conf` | Marks `wt*` interfaces as unmanaged so NetworkManager does not touch them. |
| `/usr/share/selinux/packages/nm_netbird.pp` | SELinux policy module for RPM packages. The RPM post-install script installs it with `semodule` when available. |
| `/usr/share/doc/network-manager-netbird/LICENSE` | License text for `.deb`/`.rpm` packages. Tarball releases include `LICENSE` at the archive root. |

On Fedora, the RPM SELinux policy labels `/run/netbird.sock` as `nm_netbird_runtime_t` and allows the plugin domain to connect to NetBird daemon domains that opt into `nm_netbird_daemon_domain`. It also includes a compatibility allowance for the upstream NetBird RPM when `netbird.service` runs as `unconfined_service_t`; a dedicated NetBird daemon SELinux domain remains the preferred long-term integration.

## Build targets

All targets are run via `task <name>`. See `Taskfile.yml` for full definitions.

| Target | Description |
| --- | --- |
| `deps:install` | Install system build/test dependencies on Debian/Ubuntu/Arch/Fedora/RHEL; set `WITH_GTK4=1` to include GTK 4 headers. |
| `build` | Build Go binaries + libnm editor plugin + GTK 3 editor. |
| `build:go` | Build `nm-netbird-service` and `nm-netbird-auth-dialog`. |
| `build:properties` | Build the libnm loader (`libnm-vpn-plugin-netbird.so`) and GTK 3 editor. |
| `build:properties:gtk4` | Build the GTK 4 editor. |
| `selinux:build` | Build the `nm_netbird` SELinux policy module for RPM packaging/manual install. |
| `selinux:install` | Install the built SELinux policy module with `semodule`. |
| `selinux:clean` | Remove generated SELinux policy build artifacts. |
| `test` | Run Go unit tests + editor model tests. |
| `test:go` | Run Go unit tests. |
| `test:properties` | Run editor settings mapping tests. |
| `test:race` | Run Go unit tests with race detector. |
| `fmt` | Format Go code and tidy modules. |
| `vet` | Run `go vet`. |
| `lint` | Run `golangci-lint`. |
| `patterns` | Run pragmatic code-pattern review checks. |
| `quality` | Run the standard local quality gate (fmt, vet, test, lint, patterns). |
| `quality:full` | Full quality gate including race tests. |
| `run:session` | Run the plugin on the session bus with `--debug`. |
| `run:system` | Run the plugin on the system bus with `--debug` (requires `sudo`). |
| `dbus:introspect` | Introspect the development instance on the session bus. |
| `dbus:state` | Read the plugin `State` property. |
| `dbus:connect` | Call `Connect` with empty settings. |
| `dbus:connect-interactive` | Call `ConnectInteractive` with empty settings. |
| `dbus:disconnect` | Call `Disconnect`. |
| `dbus:monitor` | Monitor plugin D-Bus signals on the session bus. |
| `smoke` | Full smoke test: build, launch session bus, introspect, connect, check state, disconnect. |
