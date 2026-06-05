package nmplugin_test

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net/netip"
	"os/exec"
	osuser "os/user"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-openapi/testify/v2/require"
	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
	"github.com/netbirdio/network-manager-plugin/internal/netbird/daemonclient"
	"github.com/netbirdio/network-manager-plugin/internal/netbird/daemonproto"
	"github.com/netbirdio/network-manager-plugin/internal/nmplugin"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestService(t *testing.T) {
	obj, _ := newTestingBusObject(t)

	assertState(t, obj, nmplugin.ServiceStateInit)

	var settingName string
	require.NoError(t, obj.Call(nmplugin.Interface+".NeedSecrets", 0, nmplugin.ConnectionSettings{}).Store(&settingName))
	require.Empty(t, settingName)

	require.NoError(t, obj.Call(nmplugin.Interface+".Connect", 0, nmplugin.ConnectionSettings{}).Store())
	waitForState(t, obj, nmplugin.ServiceStateStarted)

	require.NoError(t, obj.Call(nmplugin.Interface+".Disconnect", 0).Store())
	assertState(t, obj, nmplugin.ServiceStateStopped)
}

func TestServiceIntrospection(t *testing.T) {
	obj, _ := newTestingBusObject(t)

	node, err := introspect.Call(obj)
	require.NoError(t, err)

	require.True(t, hasInterface(node, nmplugin.Interface), "introspection is missing %s", nmplugin.Interface)
	require.True(t, hasInterface(node, nmplugin.PropertiesInterface), "introspection is missing %s", nmplugin.PropertiesInterface)
}

func TestServiceStatePropertyIsReadOnly(t *testing.T) {
	obj, _ := newTestingBusObject(t)

	err := obj.SetProperty(nmplugin.Interface+".State", uint32(nmplugin.ServiceStateStarted))
	require.Error(t, err)

	var dbusErr dbus.Error
	require.ErrorAs(t, err, &dbusErr)
	require.Equal(t, "org.freedesktop.DBus.Error.PropertyReadOnly", dbusErr.Name)
}

func TestNeedSecretsForSetupKeyProfile(t *testing.T) {
	obj, _ := newTestingBusObject(t)

	settings := nmplugin.ConnectionSettings{
		"vpn": {
			"data": dbus.MakeVariant(map[string]string{"auth": "setup-key"}),
		},
	}

	var settingName string
	require.NoError(t, obj.Call(nmplugin.Interface+".NeedSecrets", 0, settings).Store(&settingName))
	require.Equal(t, "vpn", settingName)
}

func TestNeedSecretsForSSOProfileWithoutHint(t *testing.T) {
	obj, _ := newTestingBusObject(t)

	settings := nmplugin.ConnectionSettings{
		"vpn": {
			"data": dbus.MakeVariant(map[string]string{"auth": "sso"}),
		},
	}

	var settingName string
	require.NoError(t, obj.Call(nmplugin.Interface+".NeedSecrets", 0, settings).Store(&settingName))
	require.Equal(t, "vpn", settingName)
}

func TestNeedSecretsForSSOProfileWithHintStillPrompts(t *testing.T) {
	obj, _ := newTestingBusObject(t)

	settings := nmplugin.ConnectionSettings{
		"vpn": {
			"data": dbus.MakeVariant(map[string]string{"auth": "sso", "hint": "alice@example.com"}),
		},
	}

	var settingName string
	require.NoError(t, obj.Call(nmplugin.Interface+".NeedSecrets", 0, settings).Store(&settingName))
	require.Equal(t, "vpn", settingName)
}

func TestNeedSecretsForSSOProfileWithSubmittedUserNameDoesNotPromptAgain(t *testing.T) {
	obj, _ := newTestingBusObject(t)

	settings := nmplugin.ConnectionSettings{
		"vpn": {
			"data":      dbus.MakeVariant(map[string]string{"auth": "sso"}),
			"user-name": dbus.MakeVariant("alice@example.com"),
		},
	}

	var settingName string
	require.NoError(t, obj.Call(nmplugin.Interface+".NeedSecrets", 0, settings).Store(&settingName))
	require.Empty(t, settingName)
}

func TestNeedSecretsForSSOProfileWithSubmittedHintDoesNotPromptAgain(t *testing.T) {
	obj, _ := newTestingBusObject(t)

	settings := nmplugin.ConnectionSettings{
		"vpn": {
			"data": dbus.MakeVariant(map[string]string{
				"auth":               "sso",
				"x-netbird-sso-hint": "alice@example.com",
			}),
		},
	}

	var settingName string
	require.NoError(t, obj.Call(nmplugin.Interface+".NeedSecrets", 0, settings).Store(&settingName))
	require.Empty(t, settingName)
}

func TestConnectFailsWhenSetupKeyMissingNonInteractive(t *testing.T) {
	obj, fake := newTestingBusObject(t)

	settings := nmplugin.ConnectionSettings{
		"vpn": {
			"data": dbus.MakeVariant(map[string]string{"auth": "setup-key"}),
		},
	}

	require.NoError(t, obj.Call(nmplugin.Interface+".Connect", 0, settings).Store())
	waitForState(t, obj, nmplugin.ServiceStateStopped)

	fake.mu.Lock()
	defer fake.mu.Unlock()
	require.Empty(t, fake.loginRequests)
	require.Empty(t, fake.upRequests)
}

func TestConnectInteractiveWaitsForSetupKeyNewSecrets(t *testing.T) {
	obj, fake, clientConn := newTestingBusObjectWithConn(t)
	signals := watchSignals(t, clientConn, "SecretsRequired")

	settings := nmplugin.ConnectionSettings{
		"vpn": {
			"data": dbus.MakeVariant(map[string]string{"auth": "setup-key"}),
		},
	}

	require.NoError(t, obj.Call(nmplugin.Interface+".ConnectInteractive", 0, settings, nmplugin.VariantMap{}).Store())
	activationID := activationIDFromSecretsRequired(t, waitForSignal(t, signals, "SecretsRequired"))

	secrets := nmplugin.ConnectionSettings{
		"vpn": {
			"secrets": dbus.MakeVariant(map[string]string{
				"setup-key":               "secret-from-dialog",
				"x-netbird-activation-id": activationID,
			}),
		},
	}
	require.NoError(t, obj.Call(nmplugin.Interface+".NewSecrets", 0, secrets).Store())
	waitForState(t, obj, nmplugin.ServiceStateStarted)

	fake.mu.Lock()
	defer fake.mu.Unlock()
	require.Len(t, fake.loginRequests, 1)
	require.Equal(t, "secret-from-dialog", fake.loginRequests[0].SetupKey)
}

func TestStaleSetupKeyNewSecretsDoesNotCompleteActivation(t *testing.T) {
	obj, fake, clientConn := newTestingBusObjectWithConn(t)
	signals := watchSignals(t, clientConn, "SecretsRequired")

	settings := nmplugin.ConnectionSettings{
		"vpn": {
			"data": dbus.MakeVariant(map[string]string{"auth": "setup-key"}),
		},
	}

	require.NoError(t, obj.Call(nmplugin.Interface+".ConnectInteractive", 0, settings, nmplugin.VariantMap{}).Store())
	activationID := activationIDFromSecretsRequired(t, waitForSignal(t, signals, "SecretsRequired"))

	staleSecrets := nmplugin.ConnectionSettings{
		"vpn": {
			"secrets": dbus.MakeVariant(map[string]string{
				"setup-key":               "stale-secret",
				"x-netbird-activation-id": "999999",
			}),
		},
	}
	require.NoError(t, obj.Call(nmplugin.Interface+".NewSecrets", 0, staleSecrets).Store())
	assertNoLoginRequestsUntil(t, fake)
	assertState(t, obj, nmplugin.ServiceStateStarting)

	secrets := nmplugin.ConnectionSettings{
		"vpn": {
			"secrets": dbus.MakeVariant(map[string]string{
				"setup-key":               "fresh-secret",
				"x-netbird-activation-id": activationID,
			}),
		},
	}
	require.NoError(t, obj.Call(nmplugin.Interface+".NewSecrets", 0, secrets).Store())
	waitForState(t, obj, nmplugin.ServiceStateStarted)

	fake.mu.Lock()
	defer fake.mu.Unlock()
	require.Len(t, fake.loginRequests, 1)
	require.Equal(t, "fresh-secret", fake.loginRequests[0].SetupKey)
}

func TestSetupKeyNewSecretsWithoutActivationIDUsesCurrentPrompt(t *testing.T) {
	obj, fake, clientConn := newTestingBusObjectWithConn(t)
	signals := watchSignals(t, clientConn, "SecretsRequired")

	settings := nmplugin.ConnectionSettings{
		"vpn": {
			"data": dbus.MakeVariant(map[string]string{"auth": "setup-key"}),
		},
	}

	require.NoError(t, obj.Call(nmplugin.Interface+".ConnectInteractive", 0, settings, nmplugin.VariantMap{}).Store())
	waitForSignal(t, signals, "SecretsRequired")

	secrets := nmplugin.ConnectionSettings{
		"vpn": {
			"secrets": dbus.MakeVariant(map[string]string{
				"setup-key": "secret-without-activation-id",
			}),
		},
	}
	require.NoError(t, obj.Call(nmplugin.Interface+".NewSecrets", 0, secrets).Store())
	waitForState(t, obj, nmplugin.ServiceStateStarted)

	fake.mu.Lock()
	defer fake.mu.Unlock()
	require.Len(t, fake.loginRequests, 1)
	require.Equal(t, "secret-without-activation-id", fake.loginRequests[0].SetupKey)
}

func TestSetFailureCancelsPendingSetupKeyPrompt(t *testing.T) {
	obj, fake, clientConn := newTestingBusObjectWithConn(t)
	signals := watchSignals(t, clientConn, "SecretsRequired")

	settings := nmplugin.ConnectionSettings{
		"vpn": {
			"data": dbus.MakeVariant(map[string]string{"auth": "setup-key"}),
		},
	}

	require.NoError(t, obj.Call(nmplugin.Interface+".ConnectInteractive", 0, settings, nmplugin.VariantMap{}).Store())
	activationID := activationIDFromSecretsRequired(t, waitForSignal(t, signals, "SecretsRequired"))
	require.NoError(t, obj.Call(nmplugin.Interface+".SetFailure", 0, "user canceled").Store())
	waitForState(t, obj, nmplugin.ServiceStateStopped)

	lateSecrets := nmplugin.ConnectionSettings{
		"vpn": {
			"secrets": dbus.MakeVariant(map[string]string{
				"setup-key":               "too-late",
				"x-netbird-activation-id": activationID,
			}),
		},
	}
	require.NoError(t, obj.Call(nmplugin.Interface+".NewSecrets", 0, lateSecrets).Store())

	fake.mu.Lock()
	defer fake.mu.Unlock()
	require.Empty(t, fake.loginRequests)
}

func TestSetupKeyPromptTimeoutClearsPendingNewSecrets(t *testing.T) {
	obj, fake, clientConn := newTestingBusObjectWithServiceOptions(t, func(options *nmplugin.ServiceOptions) {
		options.ActivationTimeout = 50 * time.Millisecond
	})
	signals := watchSignals(t, clientConn, "SecretsRequired")

	settings := nmplugin.ConnectionSettings{
		"vpn": {
			"data": dbus.MakeVariant(map[string]string{"auth": "setup-key"}),
		},
	}

	require.NoError(t, obj.Call(nmplugin.Interface+".ConnectInteractive", 0, settings, nmplugin.VariantMap{}).Store())
	activationID := activationIDFromSecretsRequired(t, waitForSignal(t, signals, "SecretsRequired"))
	waitForState(t, obj, nmplugin.ServiceStateStopped)

	lateSecrets := nmplugin.ConnectionSettings{
		"vpn": {
			"secrets": dbus.MakeVariant(map[string]string{
				"setup-key":               "too-late",
				"x-netbird-activation-id": activationID,
			}),
		},
	}
	require.NoError(t, obj.Call(nmplugin.Interface+".NewSecrets", 0, lateSecrets).Store())

	fake.mu.Lock()
	defer fake.mu.Unlock()
	require.Empty(t, fake.loginRequests)
}

func TestConnectUsesSetupKeyLogin(t *testing.T) {
	obj, fake := newTestingBusObject(t)

	settings := nmplugin.ConnectionSettings{
		"connection": {
			"id":   dbus.MakeVariant("netbird-generated"),
			"uuid": dbus.MakeVariant("11111111-1111-1111-1111-111111111111"),
		},
		"vpn": {
			"data":    dbus.MakeVariant(map[string]string{"auth": "setup-key", "username": "alice"}),
			"secrets": dbus.MakeVariant(map[string]string{"setup-key": "secret"}),
		},
	}

	require.NoError(t, obj.Call(nmplugin.Interface+".Connect", 0, settings).Store())
	waitForState(t, obj, nmplugin.ServiceStateStarted)

	fake.mu.Lock()
	defer fake.mu.Unlock()
	require.Len(t, fake.loginRequests, 1)
	require.Equal(t, "secret", fake.loginRequests[0].SetupKey)
	require.Equal(t, "nm-11111111-1111-1111-1111-111111111111", fake.loginRequests[0].Profile.ProfileName)
	require.Equal(t, "alice", fake.loginRequests[0].Profile.Username)
	require.NotEmpty(t, fake.upRequests)
	require.Equal(t, "nm-11111111-1111-1111-1111-111111111111", fake.upRequests[0].ProfileName)
	require.Equal(t, "alice", fake.upRequests[0].Username)
}

func TestConnectIgnoresVPNDataProfileName(t *testing.T) {
	obj, fake := newTestingBusObject(t)

	settings := nmplugin.ConnectionSettings{
		"connection": {
			"id":   dbus.MakeVariant("netbird-generated"),
			"uuid": dbus.MakeVariant("11111111-1111-1111-1111-111111111111"),
		},
		"vpn": {
			"data":    dbus.MakeVariant(map[string]string{"auth": "setup-key", "profile-name": "ignored", "username": "alice"}),
			"secrets": dbus.MakeVariant(map[string]string{"setup-key": "secret"}),
		},
	}

	require.NoError(t, obj.Call(nmplugin.Interface+".Connect", 0, settings).Store())
	waitForState(t, obj, nmplugin.ServiceStateStarted)

	fake.mu.Lock()
	defer fake.mu.Unlock()
	require.Len(t, fake.loginRequests, 1)
	require.Equal(t, "nm-11111111-1111-1111-1111-111111111111", fake.loginRequests[0].Profile.ProfileName)
	require.NotEmpty(t, fake.upRequests)
	require.Equal(t, "nm-11111111-1111-1111-1111-111111111111", fake.upRequests[0].ProfileName)
}

func TestConnectUsesNetworkManagerConnectionPermissionAsProfileUsername(t *testing.T) {
	obj, fake := newTestingBusObject(t)

	settings := nmplugin.ConnectionSettings{
		"connection": {
			"id":          dbus.MakeVariant("netbird-generated"),
			"uuid":        dbus.MakeVariant("11111111-1111-1111-1111-111111111111"),
			"permissions": dbus.MakeVariant([]string{"user:test:"}),
		},
		"vpn": {
			"data":    dbus.MakeVariant(map[string]string{"auth": "setup-key"}),
			"secrets": dbus.MakeVariant(map[string]string{"setup-key": "secret"}),
		},
	}

	require.NoError(t, obj.Call(nmplugin.Interface+".Connect", 0, settings).Store())
	waitForState(t, obj, nmplugin.ServiceStateStarted)

	fake.mu.Lock()
	defer fake.mu.Unlock()
	require.Len(t, fake.loginRequests, 1)
	require.Equal(t, "nm-11111111-1111-1111-1111-111111111111", fake.loginRequests[0].Profile.ProfileName)
	require.Equal(t, "test", fake.loginRequests[0].Profile.Username)
	require.NotEmpty(t, fake.upRequests)
	require.Equal(t, "nm-11111111-1111-1111-1111-111111111111", fake.upRequests[0].ProfileName)
	require.Equal(t, "test", fake.upRequests[0].Username)
}

func TestConnectFallsBackToProcessUsernameForNetworkManagerProfile(t *testing.T) {
	current, err := osuser.Current()
	if err != nil || strings.TrimSpace(current.Username) == "" {
		t.Skip("current process username is unavailable")
	}
	wantUsername := strings.TrimSpace(current.Username)

	obj, fake := newTestingBusObject(t)

	settings := nmplugin.ConnectionSettings{
		"connection": {
			"id":   dbus.MakeVariant("netbird-generated"),
			"uuid": dbus.MakeVariant("11111111-1111-1111-1111-111111111111"),
		},
		"vpn": {
			"data":    dbus.MakeVariant(map[string]string{"auth": "setup-key"}),
			"secrets": dbus.MakeVariant(map[string]string{"setup-key": "secret"}),
		},
	}

	require.NoError(t, obj.Call(nmplugin.Interface+".Connect", 0, settings).Store())
	waitForState(t, obj, nmplugin.ServiceStateStarted)

	fake.mu.Lock()
	defer fake.mu.Unlock()
	require.Len(t, fake.loginRequests, 1)
	require.Equal(t, "nm-11111111-1111-1111-1111-111111111111", fake.loginRequests[0].Profile.ProfileName)
	require.Equal(t, wantUsername, fake.loginRequests[0].Profile.Username)
	require.NotEmpty(t, fake.upRequests)
	require.Equal(t, "nm-11111111-1111-1111-1111-111111111111", fake.upRequests[0].ProfileName)
	require.Equal(t, wantUsername, fake.upRequests[0].Username)
}

func TestConnectUsesNetworkManagerConnectionUUIDAsProfile(t *testing.T) {
	obj, fake := newTestingBusObject(t)

	settings := nmplugin.ConnectionSettings{
		"connection": {
			"id":   dbus.MakeVariant("Work VPN"),
			"uuid": dbus.MakeVariant("22222222-2222-2222-2222-222222222222"),
		},
		"vpn": {
			"data":    dbus.MakeVariant(map[string]string{"auth": "setup-key"}),
			"secrets": dbus.MakeVariant(map[string]string{"setup-key": "secret"}),
		},
	}

	require.NoError(t, obj.Call(nmplugin.Interface+".Connect", 0, settings).Store())
	waitForState(t, obj, nmplugin.ServiceStateStarted)

	fake.mu.Lock()
	defer fake.mu.Unlock()
	require.Len(t, fake.loginRequests, 1)
	require.Equal(t, "nm-22222222-2222-2222-2222-222222222222", fake.loginRequests[0].Profile.ProfileName)
	require.NotEmpty(t, fake.upRequests)
	require.Equal(t, "nm-22222222-2222-2222-2222-222222222222", fake.upRequests[0].ProfileName)
}

func TestConnectUsesNetworkManagerConnectionIDAsProfileWhenUUIDIsMissing(t *testing.T) {
	obj, fake := newTestingBusObject(t)

	settings := nmplugin.ConnectionSettings{
		"connection": {
			"id": dbus.MakeVariant("Work VPN"),
		},
	}

	require.NoError(t, obj.Call(nmplugin.Interface+".Connect", 0, settings).Store())
	waitForState(t, obj, nmplugin.ServiceStateStarted)

	fake.mu.Lock()
	defer fake.mu.Unlock()
	require.NotEmpty(t, fake.upRequests)
	require.Equal(t, "nm-Work-VPN", fake.upRequests[0].ProfileName)
}

func TestConnectFallsBackToDefaultWhenDaemonProfilesAreDisabled(t *testing.T) {
	obj, fake := newTestingBusObject(t)

	fake.mu.Lock()
	fake.features = daemonclient.Features{DisableProfiles: true}
	fake.mu.Unlock()

	settings := nmplugin.ConnectionSettings{
		"connection": {
			"uuid": dbus.MakeVariant("33333333-3333-3333-3333-333333333333"),
		},
		"vpn": {
			"data":    dbus.MakeVariant(map[string]string{"auth": "setup-key"}),
			"secrets": dbus.MakeVariant(map[string]string{"setup-key": "secret"}),
		},
	}

	require.NoError(t, obj.Call(nmplugin.Interface+".Connect", 0, settings).Store())
	waitForState(t, obj, nmplugin.ServiceStateStarted)

	fake.mu.Lock()
	defer fake.mu.Unlock()
	require.Len(t, fake.loginRequests, 1)
	require.True(t, fake.loginRequests[0].Profile.Empty())
	require.NotEmpty(t, fake.upRequests)
	require.True(t, fake.upRequests[0].Empty())
}

func TestConnectDoesNotLoginWhenDifferentProfileIsAlreadyConnected(t *testing.T) {
	obj, fake := newTestingBusObject(t)

	fake.mu.Lock()
	fake.activeProfile = daemonclient.ProfileRef{ProfileName: "prod"}
	fake.mu.Unlock()

	settings := nmplugin.ConnectionSettings{
		"connection": {
			"uuid": dbus.MakeVariant("33333333-3333-3333-3333-333333333333"),
		},
		"vpn": {
			"data":    dbus.MakeVariant(map[string]string{"auth": "setup-key"}),
			"secrets": dbus.MakeVariant(map[string]string{"setup-key": "secret"}),
		},
	}

	require.NoError(t, obj.Call(nmplugin.Interface+".Connect", 0, settings).Store())
	waitForState(t, obj, nmplugin.ServiceStateStopped)

	fake.mu.Lock()
	defer fake.mu.Unlock()
	require.Empty(t, fake.loginRequests)
	require.Empty(t, fake.upRequests)
}

func TestConnectWithSSOAuthEmitsActionableMessage(t *testing.T) {
	obj, fake, clientConn := newTestingBusObjectWithConn(t)

	signals := make(chan *dbus.Signal, 10)
	clientConn.Signal(signals)
	t.Cleanup(func() { clientConn.RemoveSignal(signals) })

	match := fmt.Sprintf("type='signal',interface='%s',member='LoginBanner',path='%s'", nmplugin.Interface, nmplugin.ObjectPath)
	require.NoError(t, clientConn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0, match).Err)

	settings := nmplugin.ConnectionSettings{
		"vpn": {
			"data": dbus.MakeVariant(map[string]string{"auth": "sso"}),
		},
	}

	require.NoError(t, obj.Call(nmplugin.Interface+".Connect", 0, settings).Store())
	waitForState(t, obj, nmplugin.ServiceStateStopped)

	for {
		select {
		case signal := <-signals:
			if signal.Name != nmplugin.Interface+".LoginBanner" {
				continue
			}
			require.Len(t, signal.Body, 1)
			require.Equal(t, "This profile needs interactive SSO; rerun with nmcli connection up <name> --ask, or run netbird login first.", signal.Body[0])

			fake.mu.Lock()
			require.Empty(t, fake.loginRequests)
			require.Empty(t, fake.upRequests)
			fake.mu.Unlock()
			return
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for LoginBanner signal")
		}
	}
}

func TestConnectInteractiveWithSSOAuthEmitsPromptHints(t *testing.T) {
	obj, fake, clientConn := newTestingBusObjectWithConn(t)
	signals := watchSignals(t, clientConn, "LoginBanner", "SecretsRequired")

	waitStarted := make(chan struct{})
	waitRelease := make(chan struct{})
	var releaseOnce sync.Once
	releaseWait := func() {
		releaseOnce.Do(func() { close(waitRelease) })
	}
	t.Cleanup(releaseWait)

	fake.mu.Lock()
	fake.loginResponse = daemonclient.LoginResponse{
		NeedsSSOLogin:           true,
		UserCode:                "ABCD-EFGH",
		VerificationURI:         "https://login.netbird.io/device",
		VerificationURIComplete: "https://login.netbird.io/device?user_code=ABCD-EFGH",
	}
	fake.waitSSOStarted = waitStarted
	fake.waitSSORelease = waitRelease
	fake.mu.Unlock()

	settings := nmplugin.ConnectionSettings{
		"vpn": {
			"data": dbus.MakeVariant(map[string]string{"auth": "sso", "hint": "alice@example.com"}),
		},
	}

	require.NoError(t, obj.Call(nmplugin.Interface+".ConnectInteractive", 0, settings, nmplugin.VariantMap{}).Store())

	var sawBanner bool
	var sawPrompt bool
	for !sawBanner || !sawPrompt {
		signal := waitForSignal(t, signals, "LoginBanner", "SecretsRequired")
		switch signal.Name {
		case nmplugin.Interface + ".LoginBanner":
			require.Len(t, signal.Body, 1)
			banner, ok := signal.Body[0].(string)
			require.True(t, ok, "LoginBanner body type = %T", signal.Body[0])
			require.Contains(t, banner, "https://login.netbird.io/device?user_code=ABCD-EFGH")
			require.Contains(t, banner, "ABCD-EFGH")
			sawBanner = true
		case nmplugin.Interface + ".SecretsRequired":
			hints := secretsRequiredHints(t, signal)
			require.Contains(t, hints, "x-netbird-sso=true")
			require.Contains(t, hints, "x-netbird-sso-verification-uri=https://login.netbird.io/device")
			require.Contains(t, hints, "x-netbird-sso-verification-uri-complete=https://login.netbird.io/device?user_code=ABCD-EFGH")
			require.Contains(t, hints, "x-netbird-sso-user-code=ABCD-EFGH")
			require.Contains(t, hints, "x-netbird-sso-hint=alice@example.com")
			require.NotEmpty(t, activationIDFromHints(t, hints))
			sawPrompt = true
		}
	}

	select {
	case <-waitStarted:
	case <-time.After(time.Second):
		t.Fatal("SSO wait did not start")
	}
	releaseWait()
	waitForState(t, obj, nmplugin.ServiceStateStarted)
}

func TestConnectInteractiveSSOCancelStopsActivation(t *testing.T) {
	obj, fake, clientConn := newTestingBusObjectWithConn(t)
	signals := watchSignals(t, clientConn, "SecretsRequired")

	waitStarted := make(chan struct{})
	waitRelease := make(chan struct{})
	var releaseOnce sync.Once
	releaseWait := func() {
		releaseOnce.Do(func() { close(waitRelease) })
	}
	t.Cleanup(releaseWait)

	fake.mu.Lock()
	fake.loginResponse = daemonclient.LoginResponse{NeedsSSOLogin: true, UserCode: "ABCD-EFGH"}
	fake.waitSSOStarted = waitStarted
	fake.waitSSORelease = waitRelease
	fake.mu.Unlock()

	settings := nmplugin.ConnectionSettings{
		"vpn": {
			"data": dbus.MakeVariant(map[string]string{"auth": "sso"}),
		},
	}

	require.NoError(t, obj.Call(nmplugin.Interface+".ConnectInteractive", 0, settings, nmplugin.VariantMap{}).Store())
	activationID := activationIDFromSecretsRequired(t, waitForSignal(t, signals, "SecretsRequired"))

	select {
	case <-waitStarted:
	case <-time.After(time.Second):
		t.Fatal("SSO wait did not start")
	}

	cancelSecrets := nmplugin.ConnectionSettings{
		"vpn": {
			"secrets": dbus.MakeVariant(map[string]string{
				"x-netbird-activation-id": activationID,
				"x-netbird-sso-cancel":    "true",
			}),
		},
	}
	require.NoError(t, obj.Call(nmplugin.Interface+".NewSecrets", 0, cancelSecrets).Store())
	waitForState(t, obj, nmplugin.ServiceStateStopped)

	fake.mu.Lock()
	defer fake.mu.Unlock()
	require.Empty(t, fake.upRequests)
}

func TestConnectInteractiveRetriesWithSSOWhenDaemonRequiresAuthentication(t *testing.T) {
	obj, fake := newTestingBusObject(t)

	fake.mu.Lock()
	fake.upErrs = []error{daemonclient.ErrAuthenticationRequired, nil}
	fake.mu.Unlock()

	require.NoError(t, obj.Call(nmplugin.Interface+".ConnectInteractive", 0, nmplugin.ConnectionSettings{}, nmplugin.VariantMap{}).Store())
	waitForState(t, obj, nmplugin.ServiceStateStarted)

	fake.mu.Lock()
	defer fake.mu.Unlock()
	require.Len(t, fake.loginRequests, 1)
	require.Len(t, fake.upRequests, 2)
}

func TestConnectInteractiveWaitsPastActivationTimeoutForSSO(t *testing.T) {
	obj, fake, _ := newTestingBusObjectWithServiceOptions(t, func(options *nmplugin.ServiceOptions) {
		options.ActivationTimeout = 100 * time.Millisecond
		options.SSOWaitTimeout = time.Second
	})

	waitStarted := make(chan struct{})
	waitRelease := make(chan struct{})
	var releaseOnce sync.Once
	releaseWait := func() {
		releaseOnce.Do(func() { close(waitRelease) })
	}
	t.Cleanup(releaseWait)

	fake.mu.Lock()
	fake.loginResponse = daemonclient.LoginResponse{NeedsSSOLogin: true, UserCode: "CODE"}
	fake.waitSSOStarted = waitStarted
	fake.waitSSORelease = waitRelease
	fake.mu.Unlock()

	settings := nmplugin.ConnectionSettings{
		"vpn": {
			"data": dbus.MakeVariant(map[string]string{"auth": "sso"}),
		},
	}

	require.NoError(t, obj.Call(nmplugin.Interface+".ConnectInteractive", 0, settings, nmplugin.VariantMap{}).Store())

	select {
	case <-waitStarted:
	case <-time.After(time.Second):
		t.Fatal("SSO wait did not start")
	}

	time.Sleep(150 * time.Millisecond)
	assertState(t, obj, nmplugin.ServiceStateStarting)

	releaseWait()
	waitForState(t, obj, nmplugin.ServiceStateStarted)
}

func TestConnectInteractiveReturnsBeforeSSOWaitCompletes(t *testing.T) {
	obj, fake := newTestingBusObject(t)

	waitStarted := make(chan struct{})
	waitRelease := make(chan struct{})
	var releaseOnce sync.Once
	releaseWait := func() {
		releaseOnce.Do(func() { close(waitRelease) })
	}
	t.Cleanup(releaseWait)

	fake.mu.Lock()
	fake.loginResponse = daemonclient.LoginResponse{NeedsSSOLogin: true, UserCode: "CODE"}
	fake.waitSSOStarted = waitStarted
	fake.waitSSORelease = waitRelease
	fake.mu.Unlock()

	settings := nmplugin.ConnectionSettings{
		"vpn": {
			"data": dbus.MakeVariant(map[string]string{"auth": "sso"}),
		},
	}

	done := make(chan error, 1)
	go func() {
		done <- obj.Call(nmplugin.Interface+".ConnectInteractive", 0, settings, nmplugin.VariantMap{}).Store()
	}()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("ConnectInteractive did not return while SSO wait was still pending")
	}

	select {
	case <-waitStarted:
	case <-time.After(time.Second):
		t.Fatal("SSO wait did not start")
	}

	fake.mu.Lock()
	require.Empty(t, fake.upRequests)
	fake.mu.Unlock()

	releaseWait()
	waitForState(t, obj, nmplugin.ServiceStateStarted)
}

func TestDisconnectDuringActivationAfterDaemonUpCallsDown(t *testing.T) {
	obj, fake := newTestingBusObject(t)

	getConfigStarted := make(chan struct{})
	fake.mu.Lock()
	fake.getConfigStarted = getConfigStarted
	fake.mu.Unlock()

	require.NoError(t, obj.Call(nmplugin.Interface+".Connect", 0, nmplugin.ConnectionSettings{}).Store())

	select {
	case <-getConfigStarted:
	case <-time.After(time.Second):
		t.Fatal("activation did not reach config emission")
	}

	require.NoError(t, obj.Call(nmplugin.Interface+".Disconnect", 0).Store())

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		fake.mu.Lock()
		downCalls := fake.downCalls
		closed := fake.closed
		fake.mu.Unlock()
		if downCalls == 1 && closed {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	fake.mu.Lock()
	require.Equal(t, 1, fake.downCalls)
	require.True(t, fake.closed)
	fake.mu.Unlock()
	assertState(t, obj, nmplugin.ServiceStateStopped)
}

func TestConnectEmitsMinimalNetworkManagerConfig(t *testing.T) {
	obj, _, clientConn := newTestingBusObjectWithConn(t)

	signals := make(chan *dbus.Signal, 10)
	clientConn.Signal(signals)
	t.Cleanup(func() { clientConn.RemoveSignal(signals) })

	match := fmt.Sprintf("type='signal',interface='%s',member='Config',path='%s'", nmplugin.Interface, nmplugin.ObjectPath)
	require.NoError(t, clientConn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0, match).Err)

	settings := nmplugin.ConnectionSettings{
		"vpn": {
			"data": dbus.MakeVariant(map[string]string{
				"interface-name": "wt-test",
				"management-url": "https://192.0.2.10",
			}),
		},
	}

	require.NoError(t, obj.Call(nmplugin.Interface+".Connect", 0, settings).Store())

	for {
		select {
		case signal := <-signals:
			if signal.Name != nmplugin.Interface+".Config" {
				continue
			}
			require.Len(t, signal.Body, 1)
			config, ok := signal.Body[0].(map[string]dbus.Variant)
			require.True(t, ok, "Config signal body type = %T", signal.Body[0])
			require.Equal(t, "wt-test", config["tundev"].Value())
			require.Equal(t, false, config["has-ip4"].Value())
			require.Equal(t, false, config["has-ip6"].Value())
			gateway, ok := config["gateway"]
			require.True(t, ok, "Config signal is missing NetworkManager gateway metadata")
			require.Equal(t, nativeIPv4(t, "192.0.2.10"), gateway.Value())
			return
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for Config signal")
		}
	}
}

func TestConnectIgnoresNmcliUnspecifiedIfnamePlaceholder(t *testing.T) {
	obj, fake, clientConn := newTestingBusObjectWithConn(t)

	fake.mu.Lock()
	fake.config = &daemonproto.GetConfigResponse{
		ManagementUrl: "https://192.0.2.11",
		InterfaceName: "wt-daemon",
	}
	fake.mu.Unlock()

	signals := make(chan *dbus.Signal, 10)
	clientConn.Signal(signals)
	t.Cleanup(func() { clientConn.RemoveSignal(signals) })

	match := fmt.Sprintf("type='signal',interface='%s',member='Config',path='%s'", nmplugin.Interface, nmplugin.ObjectPath)
	require.NoError(t, clientConn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0, match).Err)

	settings := nmplugin.ConnectionSettings{
		"connection": {
			"interface-name": dbus.MakeVariant("--"),
		},
	}
	require.NoError(t, obj.Call(nmplugin.Interface+".Connect", 0, settings).Store())

	for {
		select {
		case signal := <-signals:
			if signal.Name != nmplugin.Interface+".Config" {
				continue
			}
			config, ok := signal.Body[0].(map[string]dbus.Variant)
			require.True(t, ok, "Config signal body type = %T", signal.Body[0])
			require.Equal(t, "wt-daemon", config["tundev"].Value())
			return
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for Config signal")
		}
	}
}

func TestConnectUsesDaemonConfigForNetworkManagerMetadata(t *testing.T) {
	obj, fake, clientConn := newTestingBusObjectWithConn(t)

	fake.mu.Lock()
	fake.config = &daemonproto.GetConfigResponse{
		ManagementUrl: "https://192.0.2.11",
		InterfaceName: "wt-daemon",
	}
	fake.mu.Unlock()

	signals := make(chan *dbus.Signal, 10)
	clientConn.Signal(signals)
	t.Cleanup(func() { clientConn.RemoveSignal(signals) })

	match := fmt.Sprintf("type='signal',interface='%s',member='Config',path='%s'", nmplugin.Interface, nmplugin.ObjectPath)
	require.NoError(t, clientConn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0, match).Err)

	require.NoError(t, obj.Call(nmplugin.Interface+".Connect", 0, nmplugin.ConnectionSettings{}).Store())

	for {
		select {
		case signal := <-signals:
			if signal.Name != nmplugin.Interface+".Config" {
				continue
			}
			require.Len(t, signal.Body, 1)
			config, ok := signal.Body[0].(map[string]dbus.Variant)
			require.True(t, ok, "Config signal body type = %T", signal.Body[0])
			require.Equal(t, "wt-daemon", config["tundev"].Value())
			gateway, ok := config["gateway"]
			require.True(t, ok, "Config signal is missing NetworkManager gateway metadata")
			require.Equal(t, nativeIPv4(t, "192.0.2.11"), gateway.Value())
			return
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for Config signal")
		}
	}
}

// -----------------------------------------------------------------------------
// ------ TESTING HELPERS ------------------------------------------------------
// -----------------------------------------------------------------------------

func nativeIPv4(t *testing.T, value string) uint32 {
	t.Helper()

	addr, err := netip.ParseAddr(value)
	require.NoError(t, err)
	bytes := addr.As4()
	return binary.NativeEndian.Uint32(bytes[:])
}

func assertState(t *testing.T, obj dbus.BusObject, want nmplugin.ServiceState) {
	t.Helper()
	require.Equal(t, want, currentState(t, obj))
}

func assertNoLoginRequestsUntil(t *testing.T, fake *fakeDaemonClient) {
	t.Helper()

	deadline := time.Now().Add(100 * time.Millisecond)
	for time.Now().Before(deadline) {
		fake.mu.Lock()
		loginRequests := len(fake.loginRequests)
		fake.mu.Unlock()
		require.Zero(t, loginRequests)
		time.Sleep(10 * time.Millisecond)
	}
}

func waitForState(t *testing.T, obj dbus.BusObject, want nmplugin.ServiceState) {
	t.Helper()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if currentState(t, obj) == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	assertState(t, obj, want)
}

func currentState(t *testing.T, obj dbus.BusObject) nmplugin.ServiceState {
	t.Helper()

	stateVariant, err := obj.GetProperty(nmplugin.Interface + ".State")
	require.NoError(t, err)

	got, ok := stateVariant.Value().(uint32)
	require.True(t, ok, "State property type = %T, want uint32", stateVariant.Value())
	return nmplugin.ServiceState(got)
}

func watchSignals(t *testing.T, conn *dbus.Conn, members ...string) chan *dbus.Signal {
	t.Helper()

	signals := make(chan *dbus.Signal, 20)
	conn.Signal(signals)
	t.Cleanup(func() { conn.RemoveSignal(signals) })

	for _, member := range members {
		match := fmt.Sprintf("type='signal',interface='%s',member='%s',path='%s'", nmplugin.Interface, member, nmplugin.ObjectPath)
		require.NoError(t, conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0, match).Err)
	}
	return signals
}

func waitForSignal(t *testing.T, signals <-chan *dbus.Signal, members ...string) *dbus.Signal {
	t.Helper()

	want := map[string]bool{}
	for _, member := range members {
		want[nmplugin.Interface+"."+member] = true
	}

	deadline := time.After(time.Second)
	for {
		select {
		case signal := <-signals:
			if want[signal.Name] {
				return signal
			}
		case <-deadline:
			t.Fatalf("timed out waiting for signals %v", members)
		}
	}
}

func activationIDFromSecretsRequired(t *testing.T, signal *dbus.Signal) string {
	t.Helper()
	return activationIDFromHints(t, secretsRequiredHints(t, signal))
}

func activationIDFromHints(t *testing.T, hints []string) string {
	t.Helper()

	for _, hint := range hints {
		key, value, ok := strings.Cut(hint, "=")
		if ok && key == "x-netbird-activation-id" {
			return value
		}
	}
	t.Fatalf("SecretsRequired hints do not contain x-netbird-activation-id: %#v", hints)
	return ""
}

func secretsRequiredHints(t *testing.T, signal *dbus.Signal) []string {
	t.Helper()

	require.Equal(t, nmplugin.Interface+".SecretsRequired", signal.Name)
	require.Len(t, signal.Body, 2)
	hints, ok := signal.Body[1].([]string)
	require.True(t, ok, "SecretsRequired hints type = %T", signal.Body[1])
	return hints
}

func hasInterface(node *introspect.Node, name string) bool {
	for _, iface := range node.Interfaces {
		if iface.Name == name {
			return true
		}
	}
	return false
}

// -----------------------------------------------------------------------------
// ----- TESTING BUS OBJECT ----------------------------------------------------
// -----------------------------------------------------------------------------

type testingBusObject struct {
	dbus.BusObject
	timeout time.Duration
}

func newTestingBusObject(t *testing.T) (dbus.BusObject, *fakeDaemonClient) {
	t.Helper()

	obj, fake, _ := newTestingBusObjectWithConn(t)
	return obj, fake
}

func newTestingBusObjectWithConn(t *testing.T) (dbus.BusObject, *fakeDaemonClient, *dbus.Conn) {
	t.Helper()

	return newTestingBusObjectWithServiceOptions(t, nil)
}

func newTestingBusObjectWithServiceOptions(t *testing.T, configure func(*nmplugin.ServiceOptions)) (dbus.BusObject, *fakeDaemonClient, *dbus.Conn) {
	t.Helper()

	address := startTestBus(t)
	fake := newFakeDaemonClient()

	serverConn := connectTestBus(t, address)
	options := nmplugin.ServiceOptions{
		ClientFactory:      fakeFactory{client: fake},
		ActivationTimeout:  time.Second,
		SSOWaitTimeout:     time.Second,
		OperationTimeout:   time.Second,
		ReadyPollInterval:  10 * time.Millisecond,
		StatusPollInterval: time.Hour,
		StatusCallTimeout:  time.Second,
	}
	if configure != nil {
		configure(&options)
	}
	service := nmplugin.NewService(serverConn, log.New(io.Discard, "", 0), false, options)
	require.NoError(t, service.Export())

	reply, err := serverConn.RequestName(nmplugin.BusName, dbus.NameFlagDoNotQueue)
	require.NoError(t, err)
	require.Equal(t, dbus.RequestNameReplyPrimaryOwner, reply)
	t.Cleanup(func() {
		_, _ = serverConn.ReleaseName(nmplugin.BusName)
	})

	clientConn := connectTestBus(t, address)
	t.Cleanup(func() {
		_ = service.Disconnect()
	})
	return testingBusObject{
		BusObject: clientConn.Object(nmplugin.BusName, nmplugin.ObjectPath),
		timeout:   time.Second,
	}, fake, clientConn
}

func (o testingBusObject) Call(method string, flags dbus.Flags, args ...any) *dbus.Call {
	ctx, cancel := context.WithTimeout(context.Background(), o.timeout)
	defer cancel()
	return o.CallWithContext(ctx, method, flags, args...)
}

func (o testingBusObject) GetProperty(p string) (dbus.Variant, error) {
	var result dbus.Variant
	err := o.StoreProperty(p, &result)
	return result, err
}

func (o testingBusObject) StoreProperty(p string, value any) error {
	iface, prop, err := splitProperty(p)
	if err != nil {
		return err
	}
	return o.Call(nmplugin.PropertiesInterface+".Get", 0, iface, prop).Store(value)
}

func (o testingBusObject) SetProperty(p string, v any) error {
	iface, prop, err := splitProperty(p)
	if err != nil {
		return err
	}

	variant, ok := v.(dbus.Variant)
	if !ok {
		variant = dbus.MakeVariant(v)
	}
	return o.Call(nmplugin.PropertiesInterface+".Set", 0, iface, prop, variant).Err
}

func splitProperty(p string) (string, string, error) {
	idx := strings.LastIndex(p, ".")
	if idx == -1 || idx+1 == len(p) {
		return "", "", fmt.Errorf("dbus: invalid property %s", p)
	}
	return p[:idx], p[idx+1:], nil
}

type fakeFactory struct {
	client *fakeDaemonClient
	err    error
}

func (f fakeFactory) NewClient(ctx context.Context) (daemonclient.Client, error) {
	return f.client, f.err
}

type fakeDaemonClient struct {
	mu sync.Mutex

	loginResponse daemonclient.LoginResponse
	status        *daemonproto.StatusResponse
	config        *daemonproto.GetConfigResponse
	features      daemonclient.Features
	activeProfile daemonclient.ProfileRef
	profiles      []daemonclient.Profile

	waitSSOStarted   chan struct{}
	waitSSORelease   chan struct{}
	getConfigStarted chan struct{}
	getConfigRelease chan struct{}

	loginRequests         []daemonclient.LoginRequest
	updateProfileRequests []daemonclient.UpdateProfileRequest
	upRequests            []daemonclient.ProfileRef
	upErrs                []error
	downCalls             int
	closed                bool
}

func newFakeDaemonClient() *fakeDaemonClient {
	return &fakeDaemonClient{
		status: &daemonproto.StatusResponse{Status: "connected", DaemonVersion: "test"},
		config: &daemonproto.GetConfigResponse{ManagementUrl: "https://192.0.2.10"},
	}
}

func (f *fakeDaemonClient) Login(ctx context.Context, request daemonclient.LoginRequest) (daemonclient.LoginResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.loginRequests = append(f.loginRequests, request)
	return f.loginResponse, nil
}

func (f *fakeDaemonClient) UpdateProfile(ctx context.Context, request daemonclient.UpdateProfileRequest) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.updateProfileRequests = append(f.updateProfileRequests, request)
	return nil
}

func (f *fakeDaemonClient) WaitSSOLogin(ctx context.Context, request daemonclient.WaitSSOLoginRequest) (daemonclient.WaitSSOLoginResponse, error) {
	f.mu.Lock()
	started := f.waitSSOStarted
	f.waitSSOStarted = nil
	release := f.waitSSORelease
	f.mu.Unlock()

	if started != nil {
		close(started)
	}
	if release != nil {
		select {
		case <-release:
		case <-ctx.Done():
			return daemonclient.WaitSSOLoginResponse{}, ctx.Err()
		}
	}
	return daemonclient.WaitSSOLoginResponse{Email: "alice@example.com"}, nil
}

func (f *fakeDaemonClient) Up(ctx context.Context, ref daemonclient.ProfileRef) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.upRequests = append(f.upRequests, ref)
	if len(f.upErrs) == 0 {
		return nil
	}
	err := f.upErrs[0]
	f.upErrs = f.upErrs[1:]
	return err
}

func (f *fakeDaemonClient) Down(ctx context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.downCalls++
	return nil
}

func (f *fakeDaemonClient) Status(ctx context.Context, options daemonclient.StatusOptions) (*daemonproto.StatusResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.status, nil
}

func (f *fakeDaemonClient) GetConfig(ctx context.Context, ref daemonclient.ProfileRef) (*daemonproto.GetConfigResponse, error) {
	f.mu.Lock()
	started := f.getConfigStarted
	f.getConfigStarted = nil
	release := f.getConfigRelease
	config := f.config
	f.mu.Unlock()

	if started != nil {
		close(started)
	}
	if started != nil || release != nil {
		if release != nil {
			select {
			case <-release:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		} else {
			<-ctx.Done()
			return nil, ctx.Err()
		}
	}
	if config != nil {
		return config, nil
	}
	return &daemonproto.GetConfigResponse{}, nil
}

func (f *fakeDaemonClient) GetFeatures(ctx context.Context) (daemonclient.Features, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.features, nil
}

func (f *fakeDaemonClient) GetActiveProfile(ctx context.Context) (daemonclient.ProfileRef, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.activeProfile, nil
}

func (f *fakeDaemonClient) ListProfiles(ctx context.Context, username string) ([]daemonclient.Profile, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]daemonclient.Profile(nil), f.profiles...), nil
}

func (f *fakeDaemonClient) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}

func startTestBus(t *testing.T) string {
	t.Helper()

	if _, err := exec.LookPath("dbus-daemon"); err != nil {
		t.Skip("dbus-daemon is required for nmplugin D-Bus tests")
	}

	cmd := exec.Command("dbus-daemon", "--session", "--nofork", "--print-address=1")
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	var stderr strings.Builder
	cmd.Stderr = &stderr

	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	address, err := bufio.NewReader(stdout).ReadString('\n')
	require.NoError(t, err, "read dbus-daemon address\n%s", stderr.String())

	return strings.TrimSpace(address)
}

func connectTestBus(t *testing.T, address string) *dbus.Conn {
	t.Helper()

	conn, err := dbus.Connect(address)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = conn.Close()
	})
	return conn
}
