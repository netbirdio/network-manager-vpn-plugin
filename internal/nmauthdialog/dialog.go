// Package nmauthdialog implements the NetworkManager VPN auth-dialog helper protocol.
package nmauthdialog

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
)

const (
	ServiceName = "org.freedesktop.NetworkManager.netbird"

	keyAuth            = "auth"
	keyNoSecret        = "no-secret"
	keySetupKey        = "setup-key"
	uiKeyfileGroup     = "VPN Plugin UI"
	setupKeyLabel      = "Setup key"
	setupKeyPrompt     = "Enter the NetBird setup key for this connection."
	setupKeyTitle      = "NetBird authentication"
	maxProtocolLineLen = 512 * 1024
	maxProtocolLines   = 4096
)

const (
	dataKeyTag   = "DATA_KEY="
	dataValTag   = "DATA_VAL="
	secretKeyTag = "SECRET_KEY="
	secretValTag = "SECRET_VAL="
)

type Options struct {
	UUID             string
	Name             string
	Service          string
	AllowInteraction bool
	ExternalUIMode   bool
	Reprompt         bool
	Hints            []string
}

type stringList []string

func (s *stringList) Set(value string) error {
	*s = append(*s, value)
	return nil
}

func (s *stringList) String() string {
	return strings.Join(*s, ",")
}

func ParseArgs(args []string) (Options, error) {
	var opts Options
	var hints stringList

	flags := flag.NewFlagSet("nm-netbird-auth-dialog", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&opts.UUID, "uuid", "", "UUID of VPN connection")
	flags.StringVar(&opts.UUID, "u", "", "UUID of VPN connection")
	flags.StringVar(&opts.Name, "name", "", "name of VPN connection")
	flags.StringVar(&opts.Name, "n", "", "name of VPN connection")
	flags.StringVar(&opts.Service, "service", "", "VPN service type")
	flags.StringVar(&opts.Service, "s", "", "VPN service type")
	flags.BoolVar(&opts.AllowInteraction, "allow-interaction", false, "allow user interaction")
	flags.BoolVar(&opts.AllowInteraction, "i", false, "allow user interaction")
	flags.BoolVar(&opts.ExternalUIMode, "external-ui-mode", false, "external UI mode")
	flags.BoolVar(&opts.Reprompt, "reprompt", false, "reprompt for secrets")
	flags.BoolVar(&opts.Reprompt, "r", false, "reprompt for secrets")
	flags.Var(&hints, "hint", "secret hint from NetworkManager")
	flags.Var(&hints, "t", "secret hint from NetworkManager")

	if err := flags.Parse(args); err != nil {
		return Options{}, err
	}
	if flags.NArg() != 0 {
		return Options{}, fmt.Errorf("unexpected arguments: %s", strings.Join(flags.Args(), " "))
	}

	opts.Hints = []string(hints)
	if strings.TrimSpace(opts.UUID) == "" {
		return Options{}, errors.New("missing --uuid")
	}
	if strings.TrimSpace(opts.Name) == "" {
		return Options{}, errors.New("missing --name")
	}
	if strings.TrimSpace(opts.Service) == "" {
		return Options{}, errors.New("missing --service")
	}
	if opts.Service != ServiceName {
		return Options{}, fmt.Errorf("this dialog only works with the %q service", ServiceName)
	}
	return opts, nil
}

func Run(args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) int {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}

	opts, err := ParseArgs(args)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	details, err := readVPNDetails(stdin)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "error: read VPN details: %v\n", err)
		return 1
	}

	if err := writeResponse(stdout, opts, details); err != nil {
		_, _ = fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	return 0
}

type vpnDetails struct {
	data    map[string]string
	secrets map[string]string
}

func readVPNDetails(r io.Reader) (vpnDetails, error) {
	if r == nil {
		return vpnDetails{}, errors.New("stdin is nil")
	}

	details := vpnDetails{
		data:    map[string]string{},
		secrets: map[string]string{},
	}
	reader := bufio.NewReader(r)
	parser := protocolParser{details: details}

	for lineNumber := 1; lineNumber <= maxProtocolLines; lineNumber++ {
		line, err := readProtocolLine(reader)
		if err != nil {
			return vpnDetails{}, err
		}
		if err := parser.consume(line); err != nil {
			return vpnDetails{}, fmt.Errorf("line %d: %w", lineNumber, err)
		}
		if parser.done {
			return parser.details, nil
		}
	}

	return vpnDetails{}, errors.New("too many protocol lines")
}

func readProtocolLine(reader *bufio.Reader) (string, error) {
	var line []byte
	for {
		chunk, isPrefix, err := reader.ReadLine()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return "", errors.New("missing DONE marker")
			}
			return "", err
		}
		line = append(line, chunk...)
		if len(line) > maxProtocolLineLen {
			return "", errors.New("protocol line is too long")
		}
		if !isPrefix {
			break
		}
	}

	if strings.ContainsRune(string(line), '\x00') {
		return "", errors.New("protocol line contains NUL")
	}
	return strings.TrimSuffix(string(line), "\r"), nil
}

type protocolParser struct {
	details vpnDetails
	done    bool
	gotItem bool

	pendingTarget string
	pendingKey    string
	pendingValue  string
	hasKey        bool
	hasValue      bool
	currentField  string
}

func (p *protocolParser) consume(line string) error {
	if line == "DONE" {
		if err := p.flush(); err != nil {
			return err
		}
		if !p.gotItem {
			return errors.New("no VPN data or secrets were provided")
		}
		p.done = true
		return nil
	}

	if strings.HasPrefix(line, "=") {
		return p.continuePrevious(line[1:])
	}

	if p.hasKey && p.hasValue {
		if err := p.flush(); err != nil {
			return err
		}
	}

	switch {
	case strings.HasPrefix(line, dataKeyTag):
		return p.startKey("data", strings.TrimPrefix(line, dataKeyTag))
	case strings.HasPrefix(line, dataValTag):
		return p.setValue("data", strings.TrimPrefix(line, dataValTag))
	case strings.HasPrefix(line, secretKeyTag):
		return p.startKey("secrets", strings.TrimPrefix(line, secretKeyTag))
	case strings.HasPrefix(line, secretValTag):
		return p.setValue("secrets", strings.TrimPrefix(line, secretValTag))
	default:
		return fmt.Errorf("unknown protocol line %q", line)
	}
}

func (p *protocolParser) continuePrevious(value string) error {
	switch p.currentField {
	case "key":
		p.pendingKey += "\n" + value
		return nil
	case "value":
		p.pendingValue += "\n" + value
		return nil
	default:
		return errors.New("continuation without a previous key or value")
	}
}

func (p *protocolParser) startKey(target string, key string) error {
	if p.hasKey || p.hasValue {
		return errors.New("new key started before previous key/value pair was complete")
	}
	p.pendingTarget = target
	p.pendingKey = key
	p.hasKey = true
	p.currentField = "key"
	return nil
}

func (p *protocolParser) setValue(target string, value string) error {
	if !p.hasKey || p.hasValue || p.pendingTarget != target {
		return errors.New("value without a matching key")
	}
	p.pendingValue = value
	p.hasValue = true
	p.currentField = "value"
	return nil
}

func (p *protocolParser) flush() error {
	if !p.hasKey && !p.hasValue {
		return nil
	}
	if !p.hasKey || !p.hasValue {
		return errors.New("incomplete key/value pair")
	}

	switch p.pendingTarget {
	case "data":
		p.details.data[p.pendingKey] = p.pendingValue
	case "secrets":
		p.details.secrets[p.pendingKey] = p.pendingValue
	default:
		return errors.New("invalid pending target")
	}

	p.pendingTarget = ""
	p.pendingKey = ""
	p.pendingValue = ""
	p.hasKey = false
	p.hasValue = false
	p.currentField = ""
	p.gotItem = true
	return nil
}

func writeResponse(w io.Writer, opts Options, details vpnDetails) error {
	needsSetupKey, setupKey, err := setupKeyRequirement(opts, details)
	if err != nil {
		return err
	}
	if !needsSetupKey {
		return writeNoSecret(w, opts.ExternalUIMode)
	}

	shouldAsk := opts.Reprompt || strings.TrimSpace(setupKey) == ""
	if shouldAsk && (!opts.ExternalUIMode || !opts.AllowInteraction) {
		return errors.New("setup-key secret is required but interaction is unavailable")
	}

	if opts.ExternalUIMode {
		return writeExternalSetupKey(w, setupKey, shouldAsk)
	}
	return writeStandardSecret(w, keySetupKey, setupKey)
}

func setupKeyRequirement(opts Options, details vpnDetails) (bool, string, error) {
	for _, hint := range opts.Hints {
		if strings.TrimSpace(hint) == "" || isSetupKeyName(hint) {
			continue
		}
		return false, "", fmt.Errorf("unsupported secret hint %q", hint)
	}

	authMode := normalizeAuthMode(firstSetting(details.data, keyAuth, "auth-mode", "authentication", "login-mode"))
	setupKey := firstSetting(details.secrets, keySetupKey, "setupKey", "netbird-setup-key")
	if setupKey == "" {
		setupKey = firstSetting(details.data, keySetupKey, "setupKey", "netbird-setup-key")
	}
	return authMode == "setup-key" || hasSetupKeyHint(opts.Hints), setupKey, nil
}

func hasSetupKeyHint(hints []string) bool {
	for _, hint := range hints {
		if isSetupKeyName(hint) {
			return true
		}
	}
	return false
}

func isSetupKeyName(value string) bool {
	switch normalizeSettingKey(value) {
	case "setupkey", "netbirdsetupkey":
		return true
	default:
		return false
	}
}

func firstSetting(values map[string]string, keys ...string) string {
	if len(values) == 0 {
		return ""
	}

	normalized := make(map[string]string, len(values))
	for key, value := range values {
		normalized[normalizeSettingKey(key)] = strings.TrimSpace(value)
	}
	for _, key := range keys {
		if value := normalized[normalizeSettingKey(key)]; value != "" {
			return value
		}
	}
	return ""
}

func normalizeSettingKey(key string) string {
	key = strings.ToLower(strings.TrimSpace(key))
	var b strings.Builder
	for _, r := range key {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func normalizeAuthMode(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "_", "-")
	switch value {
	case "setupkey", "setup-key", "key":
		return "setup-key"
	case "sso", "browser", "interactive":
		return "sso"
	case "login", "force-login":
		return "login"
	default:
		return value
	}
}

func writeNoSecret(w io.Writer, externalUI bool) error {
	if externalUI {
		return writeExternalNoSecret(w)
	}
	return writeStandardSecret(w, keyNoSecret, "true")
}

func writeStandardSecret(w io.Writer, key string, value string) error {
	if strings.ContainsAny(key, "\n\r\x00") || strings.ContainsAny(value, "\n\r\x00") {
		return errors.New("standard auth-dialog output cannot contain newlines or NUL bytes")
	}
	_, err := fmt.Fprintf(w, "%s\n%s\n\n\n", key, value)
	return err
}

func writeExternalNoSecret(w io.Writer) error {
	var b strings.Builder
	writeKeyfileUIHeader(&b, "", "")
	writeKeyfileEntry(&b, keyNoSecret, "true", "", true, false)
	_, err := io.WriteString(w, b.String())
	return err
}

func writeExternalSetupKey(w io.Writer, value string, shouldAsk bool) error {
	var b strings.Builder
	writeKeyfileUIHeader(&b, setupKeyTitle, setupKeyPrompt)
	writeKeyfileEntry(&b, keySetupKey, value, setupKeyLabel, true, shouldAsk)
	_, err := io.WriteString(w, b.String())
	return err
}

func writeKeyfileUIHeader(b *strings.Builder, title string, description string) {
	b.WriteString("[")
	b.WriteString(uiKeyfileGroup)
	b.WriteString("]\nVersion=2\n")
	if description != "" {
		b.WriteString("Description=")
		b.WriteString(escapeKeyfileValue(description))
		b.WriteByte('\n')
	}
	if title != "" {
		b.WriteString("Title=")
		b.WriteString(escapeKeyfileValue(title))
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
}

func writeKeyfileEntry(b *strings.Builder, key string, value string, label string, isSecret bool, shouldAsk bool) {
	b.WriteString("[")
	b.WriteString(key)
	b.WriteString("]\nValue=")
	b.WriteString(escapeKeyfileValue(value))
	b.WriteString("\nLabel=")
	b.WriteString(escapeKeyfileValue(label))
	b.WriteString("\nIsSecret=")
	b.WriteString(strconv.FormatBool(isSecret))
	b.WriteString("\nShouldAsk=")
	b.WriteString(strconv.FormatBool(shouldAsk))
	b.WriteByte('\n')
}

func escapeKeyfileValue(value string) string {
	var b strings.Builder
	for _, r := range value {
		switch r {
		case '\\':
			b.WriteString("\\\\")
		case '\n':
			b.WriteString("\\n")
		case '\r':
			b.WriteString("\\r")
		case '\t':
			b.WriteString("\\t")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
