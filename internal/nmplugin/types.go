package nmplugin

import "github.com/godbus/dbus/v5"

const (
	// BusName is the well-known D-Bus name NetworkManager will use to find the plugin.
	BusName = "org.freedesktop.NetworkManager.netbird"

	// Interface is the NetworkManager VPN plugin D-Bus interface.
	Interface = "org.freedesktop.NetworkManager.VPN.Plugin"

	// PropertiesInterface is the standard D-Bus properties interface.
	PropertiesInterface = "org.freedesktop.DBus.Properties"
)

const (
	// ObjectPath is the standard NetworkManager VPN plugin object path.
	ObjectPath dbus.ObjectPath = "/org/freedesktop/NetworkManager/VPN/Plugin"
)

// ConnectionSettings is NetworkManager's a{sa{sv}} connection settings map.
type ConnectionSettings = map[string]map[string]dbus.Variant

// VariantMap is a D-Bus a{sv} map.
type VariantMap = map[string]dbus.Variant

// ServiceState mirrors NMVpnServiceState from libnm's nm-vpn-dbus-interface.h.
type ServiceState uint32

const (
	ServiceStateUnknown  ServiceState = 0
	ServiceStateInit     ServiceState = 1
	ServiceStateShutdown ServiceState = 2
	ServiceStateStarting ServiceState = 3
	ServiceStateStarted  ServiceState = 4
	ServiceStateStopping ServiceState = 5
	ServiceStateStopped  ServiceState = 6
)

// PluginFailure mirrors NMVpnPluginFailure.
type PluginFailure uint32

const (
	PluginFailureLoginFailed   PluginFailure = 0
	PluginFailureConnectFailed PluginFailure = 1
	PluginFailureBadIPConfig   PluginFailure = 2
)
