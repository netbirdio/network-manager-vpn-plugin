package nmplugin

import (
	"testing"

	"github.com/godbus/dbus/v5"
)

func TestParseActivationSettingsVPNDataPrecedesDuplicateNonVPNKeys(t *testing.T) {
	settings := ConnectionSettings{
		"connection": {
			"interface-name": dbus.MakeVariant("eth0"),
		},
		"vpn": {
			"data": dbus.MakeVariant(map[string]string{
				"interface-name": "wt-netbird",
			}),
		},
	}

	for i := range 500 {
		parsed := parseActivationSettings(settings)
		if parsed.InterfaceName != "wt-netbird" {
			t.Fatalf("iteration %d: InterfaceName = %q, want vpn.data value", i, parsed.InterfaceName)
		}
	}
}

func TestParseActivationSettingsVPNSecretsPrecedeDuplicateVPNDataKeys(t *testing.T) {
	settings := ConnectionSettings{
		"vpn": {
			"data": dbus.MakeVariant(map[string]string{
				"setup-key": "data-secret",
			}),
			"secrets": dbus.MakeVariant(map[string]string{
				"setup-key": "secret-secret",
			}),
		},
	}

	for i := range 500 {
		parsed := parseActivationSettings(settings)
		if parsed.SetupKey != "secret-secret" {
			t.Fatalf("iteration %d: SetupKey = %q, want vpn.secrets value", i, parsed.SetupKey)
		}
	}
}

func TestParseActivationSettingsNormalizesLegacyAuthToSSO(t *testing.T) {
	tests := []struct {
		name     string
		settings ConnectionSettings
	}{
		{
			name:     "missing auth",
			settings: ConnectionSettings{},
		},
		{
			name: "legacy login",
			settings: ConnectionSettings{
				"vpn": {"data": dbus.MakeVariant(map[string]string{"auth": "login"})},
			},
		},
		{
			name: "legacy force login",
			settings: ConnectionSettings{
				"vpn": {"data": dbus.MakeVariant(map[string]string{"auth": "force-login"})},
			},
		},
		{
			name: "legacy reuse",
			settings: ConnectionSettings{
				"vpn": {"data": dbus.MakeVariant(map[string]string{"auth": "reuse"})},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed := parseActivationSettings(tt.settings)
			if parsed.AuthMode != "sso" {
				t.Fatalf("AuthMode = %q, want sso", parsed.AuthMode)
			}
		})
	}
}

func TestDaemonLoginRequestDefaultsURLs(t *testing.T) {
	settings := activationSettings{AuthMode: "sso"}

	request := settings.daemonLoginRequest()
	if request.ManagementURL != defaultManagementURL {
		t.Fatalf("ManagementURL = %q, want %q", request.ManagementURL, defaultManagementURL)
	}
	if request.AdminURL != defaultAdminURL {
		t.Fatalf("AdminURL = %q, want %q", request.AdminURL, defaultAdminURL)
	}
	if request.SetupKey != "" {
		t.Fatalf("SetupKey = %q, want empty", request.SetupKey)
	}
}

func TestDaemonLoginRequestPreservesConfiguredURLs(t *testing.T) {
	settings := activationSettings{
		AuthMode:      "setup-key",
		SetupKey:      "secret",
		ManagementURL: " https://api.example.com ",
		AdminURL:      " https://app.example.com ",
	}

	request := settings.daemonLoginRequest()
	if request.ManagementURL != "https://api.example.com" {
		t.Fatalf("ManagementURL = %q", request.ManagementURL)
	}
	if request.AdminURL != "https://app.example.com" {
		t.Fatalf("AdminURL = %q", request.AdminURL)
	}
	if request.SetupKey != "secret" {
		t.Fatalf("SetupKey = %q", request.SetupKey)
	}
}

func TestParseActivationSettingsNetBirdPromptKeys(t *testing.T) {
	settings := ConnectionSettings{
		"vpn": {
			"secrets": dbus.MakeVariant(map[string]string{
				"setup-key":                       "secret",
				netbirdPromptActivationID:         "42",
				netbirdSSOHint:                    "true",
				netbirdSSOVerificationURIHint:     "https://login.netbird.io/device",
				netbirdSSOVerificationURIComplete: "https://login.netbird.io/device?user_code=ABCD-EFGH",
				netbirdSSOUserCodeHint:            "ABCD-EFGH",
				netbirdSSOContinue:                "yes",
				netbirdSSOCancel:                  "no",
			}),
		},
	}

	parsed := parseActivationSettings(settings)
	if parsed.SetupKey != "secret" {
		t.Fatalf("SetupKey = %q", parsed.SetupKey)
	}
	if parsed.PromptActivationID != "42" {
		t.Fatalf("PromptActivationID = %q", parsed.PromptActivationID)
	}
	if !parsed.SSORequested {
		t.Fatal("SSORequested = false")
	}
	if parsed.SSOVerificationURI != "https://login.netbird.io/device" {
		t.Fatalf("SSOVerificationURI = %q", parsed.SSOVerificationURI)
	}
	if parsed.SSOVerificationURIComplete != "https://login.netbird.io/device?user_code=ABCD-EFGH" {
		t.Fatalf("SSOVerificationURIComplete = %q", parsed.SSOVerificationURIComplete)
	}
	if parsed.SSOUserCode != "ABCD-EFGH" {
		t.Fatalf("SSOUserCode = %q", parsed.SSOUserCode)
	}
	if !parsed.SSOContinue {
		t.Fatal("SSOContinue = false")
	}
	if parsed.SSOCancel {
		t.Fatal("SSOCancel = true")
	}
}

func TestMergeDetailsNormalizesPromptKeys(t *testing.T) {
	settings := activationSettings{AuthMode: "sso"}.mergeDetails(VariantMap{
		"auth-mode":                       dbus.MakeVariant("setup-key"),
		"netbird-setup-key":               dbus.MakeVariant("secret"),
		netbirdPromptActivationID:         dbus.MakeVariant("42"),
		netbirdSSOVerificationURIHint:     dbus.MakeVariant("https://login.netbird.io/device"),
		netbirdSSOVerificationURIComplete: dbus.MakeVariant("https://login.netbird.io/device?user_code=ABCD-EFGH"),
		netbirdSSOUserCodeHint:            dbus.MakeVariant("ABCD-EFGH"),
		netbirdSSOContinue:                dbus.MakeVariant("true"),
	})

	if settings.AuthMode != "setup-key" {
		t.Fatalf("AuthMode = %q", settings.AuthMode)
	}
	if settings.SetupKey != "secret" {
		t.Fatalf("SetupKey = %q", settings.SetupKey)
	}
	if settings.PromptActivationID != "42" {
		t.Fatalf("PromptActivationID = %q", settings.PromptActivationID)
	}
	if settings.SSOVerificationURI != "https://login.netbird.io/device" {
		t.Fatalf("SSOVerificationURI = %q", settings.SSOVerificationURI)
	}
	if settings.SSOVerificationURIComplete != "https://login.netbird.io/device?user_code=ABCD-EFGH" {
		t.Fatalf("SSOVerificationURIComplete = %q", settings.SSOVerificationURIComplete)
	}
	if settings.SSOUserCode != "ABCD-EFGH" {
		t.Fatalf("SSOUserCode = %q", settings.SSOUserCode)
	}
	if !settings.SSOContinue {
		t.Fatal("SSOContinue = false")
	}
}
