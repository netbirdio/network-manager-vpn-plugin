package nmauthdialog

import (
	"bytes"
	"os"
	"os/exec"
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
	"IsSecret=false\n" +
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
			name:       "sso interaction does not prompt for login hint",
			args:       append(baseArgs(), "--allow-interaction", "--external-ui-mode"),
			stdin:      "DATA_KEY=auth\nDATA_VAL=sso\nDONE\n",
			wantCode:   0,
			wantStdout: expectedNoSecretExternalUI,
		},
		{
			name:       "ignores blank separator lines",
			args:       baseArgs(),
			stdin:      "DATA_KEY=auth\nDATA_VAL=sso\n\nDONE\n",
			wantCode:   0,
			wantStdout: "no-secret\ntrue\n\n\n",
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

func TestRunIncludesActivationIDForDynamicSetupKeyPrompt(t *testing.T) {
	args := append(baseArgs(),
		"--allow-interaction",
		"--external-ui-mode",
		"--hint", "setup-key",
		"--hint", "x-netbird-activation-id=42",
	)
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run(args, strings.NewReader("DATA_KEY=auth\nDATA_VAL=setup-key\nDONE\n"), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d; stderr=%q", code, stderr.String())
	}
	got := stdout.String()
	for _, want := range []string{
		"[setup-key]\n",
		"ShouldAsk=true\n",
		"[x-netbird-activation-id]\n",
		"Value=42\n",
		"ShouldAsk=false\n",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("stdout does not contain %q:\n%s", want, got)
		}
	}
}

func TestRunShowsSSOHintsInExternalUI(t *testing.T) {
	args := append(baseArgs(),
		"--allow-interaction",
		"--external-ui-mode",
		"--hint", "x-netbird-sso=true",
		"--hint", "x-netbird-sso-verification-uri=https://login.netbird.io/device",
		"--hint", "x-netbird-sso-verification-uri-complete=https://login.netbird.io/device?user_code=ABCD-EFGH",
		"--hint", "x-netbird-sso-user-code=ABCD-EFGH",
		"--hint", "x-netbird-sso-hint=alice@example.com",
		"--hint", "x-netbird-activation-id=42",
	)
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run(args, strings.NewReader("DATA_KEY=auth\nDATA_VAL=sso\nDONE\n"), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d; stderr=%q", code, stderr.String())
	}
	got := stdout.String()
	for _, want := range []string{
		"Title=NetBird SSO login required\n",
		"Complete NetBird SSO in the browser window that opens.",
		"User code: ABCD-EFGH",
		"[x-netbird-sso-continue]\n",
		"Value=true\n",
		"IsSecret=false\n",
		"ShouldAsk=false\n",
		"[x-netbird-activation-id]\n",
		"Value=42\n",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("stdout does not contain %q:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{
		"x-netbird-sso-hint",
		"Login hint:",
		"alice@example.com",
	} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("stdout contains removed login hint %q:\n%s", unwanted, got)
		}
	}
}

func TestRunRejectsSSOWhenInteractionDisallowed(t *testing.T) {
	args := append(baseArgs(), "--hint", "x-netbird-sso=true")
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run(args, strings.NewReader("DATA_KEY=auth\nDATA_VAL=sso\nDONE\n"), &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1; stdout=%q stderr=%q", code, stdout.String(), stderr.String())
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

func TestValidateSSOBrowserURI(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
		ok   bool
	}{
		{name: "https", raw: "https://login.netbird.io/device", want: "https://login.netbird.io/device", ok: true},
		{name: "http", raw: "http://login.netbird.io/device", want: "http://login.netbird.io/device", ok: true},
		{name: "trim allowed", raw: "  https://login.netbird.io/device  ", want: "https://login.netbird.io/device", ok: true},
		{name: "file", raw: "file:///tmp/secret", ok: false},
		{name: "vscode", raw: "vscode://open", ok: false},
		{name: "javascript", raw: "javascript:alert(1)", ok: false},
		{name: "bare path", raw: "/tmp/secret", ok: false},
		{name: "no host", raw: "https:login.netbird.io/device", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := validateSSOBrowserURI(tt.raw)
			if ok != tt.ok || got != tt.want {
				t.Fatalf("validateSSOBrowserURI(%q) = %q, %v; want %q, %v", tt.raw, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestOpenSSOBrowserOnlyInvokesXDGOpenForAllowedURI(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") == "1" {
		os.Exit(0)
	}

	t.Setenv("DISPLAY", ":1")
	oldExecCommand := execCommand
	oldNotifyOpeningSSOBrowser := notifyOpeningSSOBrowser
	t.Cleanup(func() {
		execCommand = oldExecCommand
		notifyOpeningSSOBrowser = oldNotifyOpeningSSOBrowser
	})

	var notifications int
	notifyOpeningSSOBrowser = func() { notifications++ }

	var calls [][]string
	var lastCmd *exec.Cmd
	execCommand = func(name string, args ...string) *exec.Cmd {
		calls = append(calls, append([]string{name}, args...))
		cmd := exec.Command(os.Args[0], "-test.run=^TestOpenSSOBrowserOnlyInvokesXDGOpenForAllowedURI$")
		cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")
		lastCmd = cmd
		return cmd
	}

	openSSOBrowser(parseHints([]string{"x-netbird-sso-verification-uri=file:///tmp/secret"}))
	if len(calls) != 0 {
		t.Fatalf("xdg-open invoked for rejected URI: %#v", calls)
	}
	if notifications != 0 {
		t.Fatalf("notification shown for rejected URI: %d", notifications)
	}

	openSSOBrowser(parseHints([]string{"x-netbird-sso-verification-uri=  https://login.netbird.io/device  "}))
	if len(calls) != 1 {
		t.Fatalf("xdg-open calls = %#v, want exactly one", calls)
	}
	if got := strings.Join(calls[0], " "); got != "xdg-open https://login.netbird.io/device" {
		t.Fatalf("xdg-open invocation = %q", got)
	}
	if lastCmd.Stdin != nil || lastCmd.Stdout != nil || lastCmd.Stderr != nil {
		t.Fatalf("detached xdg-open should use /dev/null stdio, got stdin=%T stdout=%T stderr=%T", lastCmd.Stdin, lastCmd.Stdout, lastCmd.Stderr)
	}
	if notifications != 1 {
		t.Fatalf("notifications = %d, want 1", notifications)
	}

	calls = nil
	t.Setenv("DISPLAY", "")
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "")
	t.Setenv("XDG_RUNTIME_DIR", "")
	openSSOBrowser(parseHints([]string{"x-netbird-sso-verification-uri=https://login.netbird.io/device"}))
	if len(calls) != 0 {
		t.Fatalf("xdg-open invoked without desktop open environment: %#v", calls)
	}
	if notifications != 1 {
		t.Fatalf("notification shown without desktop open environment: %d", notifications)
	}

	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "unix:path=/run/user/1000/bus")
	openSSOBrowser(parseHints([]string{"x-netbird-sso-verification-uri=https://login.netbird.io/device"}))
	if len(calls) != 1 {
		t.Fatalf("xdg-open calls with session bus = %#v, want exactly one", calls)
	}
	if notifications != 2 {
		t.Fatalf("notifications with session bus = %d, want 2", notifications)
	}
}

func TestRunBrowserTestCLI(t *testing.T) {
	if os.Getenv("GO_WANT_BROWSER_TEST_HELPER") == "1" {
		os.Exit(0)
	}

	t.Setenv("DISPLAY", ":1")
	oldExecCommand := execCommand
	t.Cleanup(func() { execCommand = oldExecCommand })

	var calls [][]string
	execCommand = func(name string, args ...string) *exec.Cmd {
		calls = append(calls, append([]string{name}, args...))
		cmd := exec.Command(os.Args[0], "-test.run=^TestRunBrowserTestCLI$")
		cmd.Env = append(os.Environ(), "GO_WANT_BROWSER_TEST_HELPER=1")
		return cmd
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run([]string{
		"--test-browser", " https://login.netbird.io/device?user_code=ABCD-EFGH ",
	}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d; stderr=%q", code, stderr.String())
	}
	if stdout.String() != "opened https://login.netbird.io/device?user_code=ABCD-EFGH\n" {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if len(calls) != 1 {
		t.Fatalf("xdg-open calls = %#v, want exactly one", calls)
	}
	if got := strings.Join(calls[0], " "); got != "xdg-open https://login.netbird.io/device?user_code=ABCD-EFGH" {
		t.Fatalf("xdg-open invocation = %q", got)
	}
}

func TestRunBrowserTestCLIRequiresDesktopEnvironment(t *testing.T) {
	t.Setenv("DISPLAY", "")
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "")
	t.Setenv("XDG_RUNTIME_DIR", "")

	oldExecCommand := execCommand
	t.Cleanup(func() { execCommand = oldExecCommand })

	var calls [][]string
	execCommand = func(name string, args ...string) *exec.Cmd {
		calls = append(calls, append([]string{name}, args...))
		return exec.Command(os.Args[0], "-test.run=^TestRunBrowserTestCLIRequiresDesktopEnvironment$")
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run([]string{"--test-browser", "https://login.netbird.io/device"}, strings.NewReader(""), &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1; stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if len(calls) != 0 {
		t.Fatalf("xdg-open invoked without desktop environment: %#v", calls)
	}
	if !strings.Contains(stderr.String(), "no desktop environment detected") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunDoesNotEchoMalformedProtocolSecret(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(baseArgs(), strings.NewReader("SECRET_VALUE=super-secret-token\nDONE\n"), &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if strings.Contains(stderr.String(), "super-secret-token") {
		t.Fatalf("stderr leaked secret value: %q", stderr.String())
	}
}

func TestExternalUIEscapesLeadingSpacesAndPreservesInvalidUTF8(t *testing.T) {
	stdin := "DATA_KEY=auth\n" +
		"DATA_VAL=setup-key\n" +
		"SECRET_KEY=setup-key\n" +
		"SECRET_VAL=  \xffsecret\n" +
		"DONE\n"
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run(append(baseArgs(), "--allow-interaction", "--external-ui-mode"), strings.NewReader(stdin), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d; stderr=%q", code, stderr.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte("Value=\\s\\s\xffsecret\n")) {
		t.Fatalf("stdout did not preserve escaped leading spaces and invalid byte: %q", stdout.Bytes())
	}
}

func TestRunIgnoresUnknownHints(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(append(baseArgs(), "--hint", "unknown-secret-hint=super-secret-token"), strings.NewReader("DATA_KEY=auth\nDATA_VAL=unsupported\nDONE\n"), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d; stderr=%q", code, stderr.String())
	}
	if stdout.String() != "no-secret\ntrue\n\n\n" {
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
