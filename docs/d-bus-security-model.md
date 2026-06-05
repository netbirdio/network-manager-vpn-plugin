# System D-Bus security model

Installed system-bus policy only allows root-owned system components to own or send directly to `org.freedesktop.NetworkManager.netbird`. Normal users should not call the VPN plugin service directly.

Users interact with NetworkManager through `nmcli` or desktop frontends. NetworkManager applies PolicyKit and connection permission checks, NetworkManager talks to the NetBird VPN plugin, and the plugin talks to the local NetBird daemon.

This does not change session-bus development workflows (`--bus=session`) or NetBird daemon socket permissions. Direct system-bus debugging calls to `org.freedesktop.NetworkManager.netbird` should be run as root.

Distribution packages usually handle D-Bus policy reloads or service restarts. For manual installs, after changing the system policy file, reload/restart D-Bus and NetworkManager as appropriate for the distro. If system-bus restart is not supported safely, reboot before verifying the policy.
