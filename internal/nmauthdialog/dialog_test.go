package nmauthdialog

import (
	"bytes"
	"strings"
	"testing"
)

const expectedSetupKeyExternalUI = "[VPN Plugin UI]\n" +
	"Version=2\n" +
	"Description=Enter the NetBird setup key for this connection.\n" +
	"Title=NetBird authentication\n" +
	"\n" +
	"[setup-key]\n" +
	"Value=\n" +
	"Label=Setup key\n" +
	"IsSecret=true\n" +
	"ShouldAsk=true\n"

const expectedNoSecretExternalUI = "[VPN Plugin UI]\n" +
	"Version=2\n" +
	"\n" +
	"[no-secret]\n" +
	"Value=true\n" +
	"Label=\n" +
	"IsSecret=true\n" +
	"ShouldAsk=false\n"

func TestParseArgs(t *testing.T) {
	opts, err := ParseArgs([]string{
		"--uuid", "11111111-1111-1111-1111-111111111111",
		"--name", "NetBird",
		"--service", ServiceName,
		"--allow-interaction",
		"--external-ui-mode",
		"--reprompt",
		"--hint", "setup-key",
		"-t", "setupKey",
	})
	if err != nil {
		t.Fatalf("ParseArgs returned error: %v", err)
	}

	if opts.UUID != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("UUID = %q", opts.UUID)
	}
	if opts.Name != "NetBird" {
		t.Fatalf("Name = %q", opts.Name)
	}
	if opts.Service != ServiceName {
		t.Fatalf("Service = %q", opts.Service)
	}
	if !opts.AllowInteraction || !opts.ExternalUIMode || !opts.Reprompt {
		t.Fatalf("boolean flags were not parsed: %+v", opts)
	}
	if strings.Join(opts.Hints, ",") != "setup-key,setupKey" {
		t.Fatalf("Hints = %#v", opts.Hints)
	}
}

func TestParseArgsRejectsWrongService(t *testing.T) {
	_, err := ParseArgs([]string{
		"--uuid", "11111111-1111-1111-1111-111111111111",
		"--name", "NetBird",
		"--service", "org.freedesktop.NetworkManager.openvpn",
	})
	if err == nil {
		t.Fatal("ParseArgs accepted a non-NetBird service")
	}
}

func TestRunFixtures(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		stdin      string
		wantCode   int
		wantStdout string
	}{
		{
			name:       "no secrets required",
			args:       baseArgs(),
			stdin:      "DATA_KEY=auth\nDATA_VAL=sso\nDONE\n",
			wantCode:   0,
			wantStdout: "no-secret\ntrue\n\n\n",
		},
		{
			name:       "no secrets required in external UI mode",
			args:       append(baseArgs(), "--external-ui-mode"),
			stdin:      "DATA_KEY=auth\nDATA_VAL=sso\nDONE\n",
			wantCode:   0,
			wantStdout: expectedNoSecretExternalUI,
		},
		{
			name:     "setup-key required but interaction disallowed",
			args:     baseArgs(),
			stdin:    "DATA_KEY=auth\nDATA_VAL=setup-key\nDONE\n",
			wantCode: 1,
		},
		{
			name: "setup-key required and external UI enabled",
			args: append(baseArgs(), "--allow-interaction", "--external-ui-mode"),
			stdin: "DATA_KEY=auth\n" +
				"DATA_VAL=setup-key\n" +
				"DONE\n",
			wantCode:   0,
			wantStdout: expectedSetupKeyExternalUI,
		},
		{
			name: "setup-key hint and external UI enabled",
			args: append(baseArgs(),
				"--allow-interaction",
				"--external-ui-mode",
				"--hint", "setup-key",
			),
			stdin:      "DATA_KEY=management-url\nDATA_VAL=https://api.netbird.io\nDONE\n",
			wantCode:   0,
			wantStdout: expectedSetupKeyExternalUI,
		},
		{
			name: "wrong service type",
			args: []string{
				"--uuid", "11111111-1111-1111-1111-111111111111",
				"--name", "NetBird",
				"--service", "org.freedesktop.NetworkManager.openvpn",
			},
			stdin:    "DATA_KEY=auth\nDATA_VAL=sso\nDONE\n",
			wantCode: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer

			code := Run(tt.args, strings.NewReader(tt.stdin), &stdout, &stderr)
			if code != tt.wantCode {
				t.Fatalf("exit code = %d, want %d; stderr=%q", code, tt.wantCode, stderr.String())
			}
			if stdout.String() != tt.wantStdout {
				t.Fatalf("stdout = %q, want %q", stdout.String(), tt.wantStdout)
			}
		})
	}
}

func TestRunReturnsExistingSetupKey(t *testing.T) {
	stdin := "DATA_KEY=auth\n" +
		"DATA_VAL=setup-key\n" +
		"SECRET_KEY=setup-key\n" +
		"SECRET_VAL=secret-123\n" +
		"DONE\n"
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run(baseArgs(), strings.NewReader(stdin), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d; stderr=%q", code, stderr.String())
	}
	if stdout.String() != "setup-key\nsecret-123\n\n\n" {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func baseArgs() []string {
	return []string{
		"--uuid", "11111111-1111-1111-1111-111111111111",
		"--name", "NetBird",
		"--service", ServiceName,
	}
}
