package nmplugin

import (
	"testing"

	"github.com/godbus/dbus/v5"
)

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
				netbirdSSOLoginHint:               "alice@example.com",
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
	if parsed.SSOHint != "alice@example.com" {
		t.Fatalf("SSOHint = %q", parsed.SSOHint)
	}
	if !parsed.SSOContinue {
		t.Fatal("SSOContinue = false")
	}
	if parsed.SSOCancel {
		t.Fatal("SSOCancel = true")
	}
}
