package nmplugin

import (
	"errors"
	"fmt"

	"github.com/godbus/dbus/v5"
	"github.com/netbirdio/network-manager-plugin/internal/netbird/daemonclient"
)

const (
	dbusErrorPropertyReadOnly = "org.freedesktop.DBus.Error.PropertyReadOnly"
	dbusErrorUnknownInterface = "org.freedesktop.DBus.Error.UnknownInterface"
	dbusErrorUnknownProperty  = "org.freedesktop.DBus.Error.UnknownProperty"
	dbusErrorVPNFailed        = "org.freedesktop.NetworkManager.VPN.Error.Failed"
	dbusErrorVPNAlreadyActive = "org.freedesktop.NetworkManager.VPN.Error.AlreadyStarted"
)

const interactiveSSORequiredMessage = "This profile needs interactive SSO; rerun with nmcli connection up <name> --ask."

var (
	errMissingSetupKey      = errors.New("setup-key authentication requested but no setup-key secret was provided")
	errInteractiveSSONeeded = errors.New("interactive SSO required")
	errPromptUnavailable    = errors.New("activation prompt is no longer available")
)

func classifyUpFailure(err error) PluginFailure {
	if errors.Is(err, daemonclient.ErrAuthenticationRequired) {
		return PluginFailureLoginFailed
	}
	return PluginFailureConnectFailed
}

func readyWaitError(ctxErr error, lastErr error, lastMessage string) error {
	if lastErr != nil {
		return fmt.Errorf("timeout waiting for netbird ready: %w (last error: %v)", ctxErr, lastErr)
	}
	if lastMessage != "" {
		return fmt.Errorf("timeout waiting for netbird ready: %w (last status: %s)", ctxErr, lastMessage)
	}
	return fmt.Errorf("timeout waiting for netbird ready: %w", ctxErr)
}

func newDBusError(name string, format string, args ...any) *dbus.Error {
	return dbus.NewError(name, []any{fmt.Sprintf(format, args...)})
}
