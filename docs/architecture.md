# Architecture

How the NetworkManager NetBird VPN plugin works, why it works that way, and what happens during activation.

## The plugin is a thin bridge

The plugin is a D-Bus service that NetworkManager calls to start and stop VPN connections. It does **not** configure IP addresses, routes, DNS, or firewall rules. All of that remains the NetBird daemon's responsibility. The plugin's only job is to translate NetworkManager's VPN lifecycle (`Connect` → `Disconnect`) into NetBird daemon gRPC calls (`Up` → `Down`).

```txt
┌───────────────┐  D-Bus    ┌──────────────┐  gRPC    ┌──────────────┐
│ NetworkManager│ ◄──────►  │ nm-netbird-  │ ◄─────►  │  netbird     │
│               │           │  service     │          │  daemon      │
│ nmcli / GUI   │           │              │          │              │
│ frontends     │           │ VPN Plugin   │          │ WireGuard®   │
│               │           │              │          │ routes, DNS  │
└───────────────┘           └──────────────┘          └──────────────┘
```

NetworkManager sees a standard VPN plugin. The NetBird daemon sees standard gRPC calls. The plugin is just the adapter between them.

## Why the plugin doesn't manage IP configuration

A traditional NetworkManager VPN plugin (OpenVPN, WireGuard, etc.) receives IP addresses, routes, and DNS settings from the VPN server and hands them back to NetworkManager through `SetConfig` / `SetIp4Config` / `SetIp6Config` calls and `Config` / `Ip4Config` signals.

This plugin does none of that. NetBird already manages the WireGuard interface, assigns addresses from its own IPAM, configures kernel routes, sets DNS via its embedded resolver, and applies firewall rules through `nftables`/`iptables`. If the plugin tried to mirror these back to NetworkManager, the two would race for ownership of the same resources.

Instead, the plugin sends a **minimal `Config` signal** that:

- Includes the daemon-created tunnel interface (`tundev`) when the name is known from NetworkManager settings or daemon config.
- Provides an external gateway address derived from the management or admin URL.
- Sets `has-ip4=false` and `has-ip6=false` so NetworkManager knows not to expect or apply any IP configuration.

This is the contract: NetworkManager marks the connection as active but leaves IP configuration alone. The NetBird daemon continues to own and operate the tunnel.

## Activation lifecycle

### 1. `Connect` / `ConnectInteractive`

NetworkManager calls one of these when the user activates a connection. The plugin:

1. **Reserves the activation slot.** Only one activation can run at a time. If a daemon client is already active, the call fails with `AlreadyStarted`.
2. **Sets state to `STARTING`** and returns immediately. All long-running work happens in a goroutine.

### 2. Connect to the NetBird daemon

The plugin dials the gRPC socket (`--daemon-address`, default `unix:///var/run/netbird.sock`). If `--start-daemon` is enabled and the first dial fails, the plugin asks the init system to start the `netbird` service and retries.

### 3. Prepare the NetBird profile

The plugin maps the NetworkManager connection to a NetBird daemon profile:

- **Profile name:** `nm-<connection UUID>` (falls back to a sanitised connection ID if NetworkManager doesn't provide a UUID).
- **Username:** Inferred from:
  1. The `username` key in `vpn.data`.
  2. NetworkManager connection permissions (when the profile has exactly one `user:` permission).
  3. The service process user (usually `root` for system-wide connections).

If a different NetBird profile is already connected or connecting on the daemon, the plugin fails safely — NetBird supports one active engine at a time, and the plugin will not switch away from another session.

### 4. Authenticate

Depending on the `auth` mode in `vpn.data`:

| `auth` value | Behaviour |
| --- | --- |
| `setup-key` | Call `Login` on the daemon with the setup key from `vpn.secrets`. If the key is missing and the call was `ConnectInteractive`, emit `SecretsRequired` and wait for `NewSecrets`. |
| `sso` | Call `Login` on the daemon to initiate device-code SSO, emit `LoginBanner` with the verification URL and user code, emit `SecretsRequired` for SSO hints, then poll `WaitSSOLogin` until the daemon reports completion or the SSO wait timeout (`--sso-wait-timeout`, default 10 minutes) expires. |

Missing auth and legacy `login` / `reuse` values are normalized to `sso`; they are not separate modes. If `auth=sso` but the call was non-interactive `Connect`, the plugin fails immediately and emits a `LoginBanner` telling the user to rerun with `--ask`.

### 5. Update daemon profile

The plugin updates the daemon profile through the NetBird daemon `SetConfig` RPC (wrapped internally as `UpdateProfile`) when:

- `auth` is `setup-key` or `sso`; or
- any of management URL, admin URL, interface name, or PSK is explicitly present in the NetworkManager settings.

The plugin does not first compare these values with the daemon's current configuration. For update requests, empty management/admin URLs are filled with the plugin defaults (`https://api.netbird.io:443` and `https://app.netbird.io:443`). The update is skipped if the daemon reports `DisableUpdateSettings` in its feature flags.

### 6. `Up`

The plugin calls `Up` on the daemon for the resolved profile. If the daemon responds with an authentication error, activation fails with `LOGIN_FAILED`.

### 7. Wait for ready

The plugin polls daemon `Status` at 500 ms intervals until the daemon reports `Connected`. If the daemon reports `Failed`, the plugin fails the activation. If the activation timeout (`--activation-timeout`, default 90 seconds) expires before the daemon reaches `Connected`, the activation fails.

### 8. Emit the `Config` signal

Once the daemon is connected, the plugin reads the daemon config (for interface name, management URL) and emits the minimal `Config` signal. NetworkManager considers the VPN active at this point.

### 9. Monitoring

A background goroutine polls daemon `Status` every 5 seconds. If the daemon disconnects or fails, the plugin emits `StateChanged` → `STOPPED` and, for failures, `Failure(CONNECT_FAILED)`. The daemon gRPC client is closed, and a new activation can be started.

### 10. `Disconnect`

When NetworkManager calls `Disconnect`, the plugin:

1. Cancels any in-flight activation.
2. Calls `Down` on the daemon (this stops the global daemon engine).
3. Closes the gRPC client.
4. Sets state to `STOPPED`.

## Profile mapping and the single-engine constraint

The NetBird daemon supports **one active WireGuard engine** at a time. This means only one NetworkManager-backed connection can be active. The plugin enforces this by reserving the activation slot and holding a reference to the active daemon client.

Each NetworkManager connection maps to a distinct NetBird profile name (`nm-<UUID>`). Switching between connections works because:

1. `Disconnect` calls daemon `Down`, stopping the engine.
2. A new `Connect` starts a new activation with a different profile name.
3. The daemon `Up` call binds the engine to the new profile.

The profile owner username ensures that when multiple system users have NetworkManager connections, they each get the correct NetBird profile scope. Per-user connections (user-scoped `nmcli`) forward the connection's permission username; system-wide connections use the service process user.

## Security boundaries

### D-Bus policy

The system-bus policy file (packaged as `nm-netbird-service.conf` under the system D-Bus policy directory, for example `/usr/share/dbus-1/system.d/` or `/etc/dbus-1/system.d/`) restricts ownership and direct messaging to `root`:

```xml
<policy user="root">
    <allow own="org.freedesktop.NetworkManager.netbird"/>
    <allow send_destination="org.freedesktop.NetworkManager.netbird"/>
</policy>
<policy context="default">
    <deny own="org.freedesktop.NetworkManager.netbird"/>
    <deny send_destination="org.freedesktop.NetworkManager.netbird"/>
</policy>
```

Normal users cannot call plugin methods directly on the system bus. They must go through NetworkManager, which applies its own PolicyKit authorisation and connection permission checks before forwarding requests to the plugin.

### Daemon socket

The plugin communicates with the NetBird daemon over a local gRPC socket (`unix:///var/run/netbird.sock` by default). The security of this channel is determined by the Unix socket permissions set by the NetBird daemon. The plugin does not add or enforce its own authentication on this channel — it trusts that the local daemon socket is adequately protected.

### Secrets handling

- `setup-key` and `pre-shared-key` are stored in `vpn.secrets`, which NetworkManager protects according to its permissions model.
- The plugin never logs or persists secrets.
- The plugin does not write secrets back to NetworkManager storage after activation.

## Desktop editor (split-loader design)

The desktop properties editor uses NetworkManager's standard split-loader pattern:

```txt
nm-connection-editor / GNOME Settings
        │
        ▼
libnm-vpn-plugin-netbird.so          ← thin loader, loaded by NetworkManager
        │
        ├── GTK 3 frontend
        │      └── libnm-vpn-plugin-netbird-editor.so   (GTK 3 editor)
        │
        └── GTK 4 frontend
               └── libnm-gtk4-vpn-plugin-netbird-editor.so (GTK 4 editor)
```

The loader (`libnm-vpn-plugin-netbird.so`) inspects the runtime environment at load time and selects the matching GTK 3 or GTK 4 editor module. When both editor modules are installed, this allows a single package to work with both GNOME Settings (GTK 4 on modern GNOME) and `nm-connection-editor` (GTK 3). Source builds always build the GTK 3 module; the GTK 4 module is controlled by the Meson `gtk4` option and is disabled by default.

The editor validates local field syntax only: auth mode, HTTP(S) URL shape, and interface-name characters. It does not contact the NetBird daemon while editing. It preserves unknown existing data and secrets keys when saving, so users can hand-edit special keys without the editor stripping them.

## Unmanaged interface configuration

NetBird owns its WireGuard interfaces. The plugin package installs a NetworkManager config fragment (`/etc/NetworkManager/conf.d/90-netbird-unmanaged.conf`) that marks the default `wt*` interface prefix as unmanaged:

```ini
[keyfile]
unmanaged-devices=interface-name:wt*
```

If you configure a custom daemon `interfaceName` outside the `wt*` pattern, you must update this file to include that name. Otherwise NetworkManager may attempt to configure the daemon-created interface and race NetBird for IP, DNS, and route ownership.
