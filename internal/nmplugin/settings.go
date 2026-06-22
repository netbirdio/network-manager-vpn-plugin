package nmplugin

import (
	"fmt"
	"os"
	osuser "os/user"
	"slices"
	"strconv"
	"strings"

	"github.com/godbus/dbus/v5"
	"github.com/netbirdio/network-manager-plugin/internal/netbird/daemonclient"
)

const (
	vpnSettingName = "vpn"

	defaultManagementURL = "https://api.netbird.io:443"
	defaultAdminURL      = "https://app.netbird.io:443"

	netbirdPromptActivationID         = "x-netbird-activation-id"
	netbirdSSOHint                    = "x-netbird-sso"
	netbirdSSOVerificationURIHint     = "x-netbird-sso-verification-uri"
	netbirdSSOVerificationURIComplete = "x-netbird-sso-verification-uri-complete"
	netbirdSSOUserCodeHint            = "x-netbird-sso-user-code"
	netbirdSSOContinue                = "x-netbird-sso-continue"
	netbirdSSOCancel                  = "x-netbird-sso-cancel"
)

type activationSettings struct {
	Profile       daemonclient.ProfileRef
	SetupKey      string
	ManagementURL string
	AdminURL      string
	Hostname      string
	InterfaceName string
	PreSharedKey  string
	AuthMode      string

	PromptActivationID         string
	SSORequested               bool
	SSOVerificationURI         string
	SSOVerificationURIComplete string
	SSOUserCode                string
	SSOContinue                bool
	SSOCancel                  bool
}

func parseActivationSettings(settings ConnectionSettings) activationSettings {
	values := flattenConnectionSettings(settings)
	profileName := networkManagerConnectionProfileName(settings)

	username := firstSetting(values, "netbird-username", "username", "user-name", "user")
	if username == "" && profileName != "" {
		username = networkManagerConnectionPermissionUsername(settings)
	}

	interfaceName := normalizeInterfaceName(firstSetting(values, "interface-name", "interfaceName", "netbird-interface-name"))
	authMode := normalizeAuthMode(firstSetting(values, "auth", "auth-mode", "authentication", "login-mode"))

	return activationSettings{
		Profile: daemonclient.ProfileRef{
			ProfileName: profileName,
			Username:    username,
		},
		SetupKey:                   firstSetting(values, "setup-key", "setupKey", "netbird-setup-key"),
		ManagementURL:              firstSetting(values, "management-url", "managementUrl", "netbird-management-url"),
		AdminURL:                   firstSetting(values, "admin-url", "adminURL", "netbird-admin-url"),
		Hostname:                   firstSetting(values, "hostname", "host-name"),
		InterfaceName:              interfaceName,
		PreSharedKey:               firstSetting(values, "pre-shared-key", "preshared-key", "preSharedKey"),
		AuthMode:                   authMode,
		PromptActivationID:         firstSetting(values, netbirdPromptActivationID),
		SSORequested:               boolSetting(values, netbirdSSOHint),
		SSOVerificationURI:         firstSetting(values, netbirdSSOVerificationURIHint),
		SSOVerificationURIComplete: firstSetting(values, netbirdSSOVerificationURIComplete),
		SSOUserCode:                firstSetting(values, netbirdSSOUserCodeHint),
		SSOContinue:                boolSetting(values, netbirdSSOContinue),
		SSOCancel:                  boolSetting(values, netbirdSSOCancel),
	}
}

func normalizeInterfaceName(value string) string {
	value = strings.TrimSpace(value)
	// nmcli uses `ifname --` to mean "no bound device" for VPN profiles, but
	// stores that placeholder as connection.interface-name. Do not pass it back
	// to NetworkManager as the daemon tunnel name.
	if value == "--" {
		return ""
	}
	return value
}

func (s activationSettings) needsSetupKeySecret() bool {
	return s.AuthMode == "setup-key" && strings.TrimSpace(s.SetupKey) == ""
}

func (s activationSettings) shouldLogin(interactive bool) bool {
	switch s.AuthMode {
	case "setup-key":
		return true
	case "sso":
		return interactive
	default:
		return false
	}
}

func (s activationSettings) shouldUpdateProfile() bool {
	switch s.AuthMode {
	case "setup-key", "sso":
		return true
	}
	return strings.TrimSpace(s.ManagementURL) != "" ||
		strings.TrimSpace(s.AdminURL) != "" ||
		strings.TrimSpace(s.InterfaceName) != "" ||
		strings.TrimSpace(s.PreSharedKey) != ""
}

func (s activationSettings) isAuthModeValid() bool {
	if s.AuthMode == "" {
		return false
	}

	if s.AuthMode != "setup-key" && s.AuthMode != "sso" {
		return false
	}

	return true
}

func (s activationSettings) daemonLoginRequest() daemonclient.LoginRequest {
	hostname := strings.TrimSpace(s.Hostname)
	if hostname == "" {
		hostname = defaultHostname()
	}
	setupKey := ""
	if s.AuthMode == "setup-key" {
		setupKey = s.SetupKey
	}
	return daemonclient.LoginRequest{
		SetupKey:      setupKey,
		ManagementURL: s.resolvedManagementURL(),
		AdminURL:      s.resolvedAdminURL(),
		Hostname:      hostname,
		InterfaceName: s.InterfaceName,
		PreSharedKey:  s.PreSharedKey,
		Profile:       s.Profile,
	}
}

func (s activationSettings) daemonUpdateProfileRequest() daemonclient.UpdateProfileRequest {
	return daemonclient.UpdateProfileRequest{
		Profile:       s.Profile,
		ManagementURL: s.resolvedManagementURL(),
		AdminURL:      s.resolvedAdminURL(),
		InterfaceName: s.InterfaceName,
		PreSharedKey:  s.PreSharedKey,
	}
}

func (s activationSettings) resolvedManagementURL() string {
	if value := strings.TrimSpace(s.ManagementURL); value != "" {
		return value
	}
	return defaultManagementURL
}

func (s activationSettings) resolvedAdminURL() string {
	if value := strings.TrimSpace(s.AdminURL); value != "" {
		return value
	}
	return defaultAdminURL
}

func (s activationSettings) mergeDetails(details VariantMap) activationSettings {
	if len(details) == 0 {
		return s
	}

	settings := s // copy to avoid mutating the original
	values := normalizeStringMap(variantMapToStrings(details))

	if value := firstSetting(values, "auth", "auth-mode", "authentication", "login-mode"); value != "" {
		settings.AuthMode = normalizeAuthMode(value)
	}
	mergeStringDetail(values, &settings.SetupKey, "setup-key", "setupKey", "netbird-setup-key")
	mergeStringDetail(values, &settings.PromptActivationID, netbirdPromptActivationID)
	mergeStringDetail(values, &settings.SSOVerificationURI, netbirdSSOVerificationURIHint)
	mergeStringDetail(values, &settings.SSOVerificationURIComplete, netbirdSSOVerificationURIComplete)
	mergeStringDetail(values, &settings.SSOUserCode, netbirdSSOUserCodeHint)
	if boolSetting(values, netbirdSSOHint) {
		settings.SSORequested = true
	}
	if boolSetting(values, netbirdSSOContinue) {
		settings.SSOContinue = true
	}
	if boolSetting(values, netbirdSSOCancel) {
		settings.SSOCancel = true
	}

	return settings
}

func mergeStringDetail(values map[string]string, field *string, keys ...string) {
	if value := firstSetting(values, keys...); value != "" {
		*field = value
	}
}

func flattenConnectionSettings(settings ConnectionSettings) map[string]string {
	values := map[string]string{}

	// NetworkManager supplies settings as maps, so iterating all sections and
	// fields directly makes duplicate keys nondeterministic. Merge in explicit
	// precedence order instead: whitelisted NetworkManager connection fields,
	// VPN scalar fields, vpn.data, then vpn.secrets. The daemon-facing vpn.data
	// and vpn.secrets values must win over duplicate keys elsewhere.
	for _, setting := range sortedSectionSettings(settings, "connection") {
		if setting.keyKind == "interfacename" {
			mergeScalarSetting(values, setting.keyKind, setting.variant)
		}
	}

	vpnSettings := sortedSectionSettings(settings, vpnSettingName)
	for _, setting := range vpnSettings {
		if setting.keyKind == "data" || setting.keyKind == "secrets" {
			continue
		}
		mergeScalarSetting(values, setting.keyKind, setting.variant)
	}

	for _, nestedKey := range []string{"data", "secrets"} {
		for _, setting := range vpnSettings {
			if setting.keyKind != nestedKey {
				continue
			}
			mergeNormalizedStringMap(values, variantToStringMap(setting.variant))
		}
	}

	return values
}

type settingEntry struct {
	keyKind string
	variant dbus.Variant
}

func sortedSectionSettings(settings ConnectionSettings, sectionName string) []settingEntry {
	sections := make([]string, 0, len(settings))
	for section := range settings {
		sections = append(sections, section)
	}
	slices.Sort(sections)

	entries := []settingEntry{}
	for _, section := range sections {
		if normalizeSectionName(section) != sectionName {
			continue
		}

		sectionValues := settings[section]
		keys := make([]string, 0, len(sectionValues))
		for key := range sectionValues {
			keys = append(keys, key)
		}
		slices.Sort(keys)

		for _, key := range keys {
			entries = append(entries, settingEntry{
				keyKind: normalizeSettingKey(key),
				variant: sectionValues[key],
			})
		}
	}
	return entries
}

func mergeScalarSetting(values map[string]string, keyName string, variant dbus.Variant) {
	if keyName == "" {
		return
	}
	if value, ok := variantToString(variant); ok {
		values[keyName] = strings.TrimSpace(value)
	}
}

func mergeNormalizedStringMap(dst map[string]string, src map[string]string) {
	keys := make([]string, 0, len(src))
	for key := range src {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	for _, key := range keys {
		keyName := normalizeSettingKey(key)
		if keyName == "" {
			continue
		}
		dst[keyName] = strings.TrimSpace(src[key])
	}
}

func normalizeStringMap(values map[string]string) map[string]string {
	normalized := make(map[string]string, len(values))
	mergeNormalizedStringMap(normalized, values)
	return normalized
}

func firstSetting(values map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(values[normalizeSettingKey(key)]); value != "" {
			return value
		}
	}
	return ""
}

func boolSetting(values map[string]string, key string) bool {
	value := strings.ToLower(strings.TrimSpace(values[normalizeSettingKey(key)]))
	switch value {
	case "1", "t", "true", "y", "yes", "on":
		return true
	default:
		return false
	}
}

func networkManagerConnectionProfileName(settings ConnectionSettings) string {
	if uuid := sanitizeProfileNameComponent(firstConnectionSetting(settings, "uuid")); uuid != "" {
		return "nm-" + uuid
	}
	if id := sanitizeProfileNameComponent(firstConnectionSetting(settings, "id")); id != "" {
		return "nm-" + id
	}
	return ""
}

func networkManagerConnectionPermissionUsername(settings ConnectionSettings) string {
	users := map[string]struct{}{}
	for section, sectionValues := range settings {
		if normalizeSectionName(section) != "connection" {
			continue
		}
		for key, variant := range sectionValues {
			if normalizeSettingKey(key) != "permissions" {
				continue
			}
			for _, username := range permissionUsers(variant) {
				users[username] = struct{}{}
			}
		}
	}
	if len(users) != 1 {
		return ""
	}
	for username := range users {
		return username
	}
	return ""
}

func permissionUsers(variant dbus.Variant) []string {
	switch typed := variant.Value().(type) {
	case []string:
		return permissionUsersFromStrings(typed)
	case []dbus.Variant:
		permissions := make([]string, 0, len(typed))
		for _, item := range typed {
			if value, ok := variantToString(item); ok {
				permissions = append(permissions, value)
			}
		}
		return permissionUsersFromStrings(permissions)
	case []any:
		permissions := make([]string, 0, len(typed))
		for _, item := range typed {
			if value, ok := anyToString(item); ok {
				permissions = append(permissions, value)
			}
		}
		return permissionUsersFromStrings(permissions)
	case string:
		return permissionUsersFromStrings([]string{typed})
	default:
		return nil
	}
}

func permissionUsersFromStrings(permissions []string) []string {
	users := make([]string, 0, len(permissions))
	for _, permission := range permissions {
		permission = strings.TrimSpace(permission)
		if !strings.HasPrefix(permission, "user:") {
			continue
		}
		username := strings.TrimPrefix(permission, "user:")
		if before, _, ok := strings.Cut(username, ":"); ok {
			username = before
		}
		username = strings.TrimSpace(username)
		if username != "" {
			users = append(users, username)
		}
	}
	return users
}

func currentProcessUsername() string {
	current, err := osuser.Current()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(current.Username)
}

func firstConnectionSetting(settings ConnectionSettings, keys ...string) string {
	for section, sectionValues := range settings {
		if normalizeSectionName(section) != "connection" {
			continue
		}

		values := make(map[string]string, len(sectionValues))
		for key, variant := range sectionValues {
			if value, ok := variantToString(variant); ok {
				values[key] = value
			}
		}
		if value := firstSetting(normalizeStringMap(values), keys...); value != "" {
			return value
		}
	}
	return ""
}

func sanitizeProfileNameComponent(value string) string {
	value = strings.TrimSpace(value)
	var b strings.Builder
	lastSeparator := false
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
			lastSeparator = false
			continue
		}
		if b.Len() > 0 && !lastSeparator {
			b.WriteByte('-')
			lastSeparator = true
		}
	}
	return strings.Trim(b.String(), "-_.")
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

func normalizeSectionName(section string) string {
	section = strings.ToLower(strings.TrimSpace(section))
	section = strings.TrimPrefix(section, "org.freedesktop.networkmanager.settings.")
	return section
}

func normalizeAuthMode(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "_", "-")
	switch value {
	case "setupkey", "setup-key", "key":
		return "setup-key"
	case "", "sso", "browser", "interactive", "login", "force-login", "reuse":
		return "sso"
	default:
		return value
	}
}

func variantMapToStrings(values VariantMap) map[string]string {
	out := make(map[string]string, len(values))
	for key, variant := range values {
		if value, ok := variantToString(variant); ok {
			out[key] = value
		}
	}
	return out
}

func variantToStringMap(variant dbus.Variant) map[string]string {
	value := variant.Value()
	switch typed := value.(type) {
	case map[string]string:
		return typed
	case map[string]dbus.Variant:
		return variantMapToStrings(typed)
	case map[string]any:
		out := make(map[string]string, len(typed))
		for key, nested := range typed {
			if value, ok := anyToString(nested); ok {
				out[key] = value
			}
		}
		return out
	default:
		return nil
	}
}

func variantToString(variant dbus.Variant) (string, bool) {
	return anyToString(variant.Value())
}

func anyToString(value any) (string, bool) {
	switch typed := value.(type) {
	case string:
		return typed, true
	case []byte:
		return string(typed), true
	case fmt.Stringer:
		return typed.String(), true
	case bool:
		return strconv.FormatBool(typed), true
	case int:
		return strconv.Itoa(typed), true
	case int32:
		return strconv.FormatInt(int64(typed), 10), true
	case int64:
		return strconv.FormatInt(typed, 10), true
	case uint:
		return strconv.FormatUint(uint64(typed), 10), true
	case uint32:
		return strconv.FormatUint(uint64(typed), 10), true
	case uint64:
		return strconv.FormatUint(typed, 10), true
	default:
		return "", false
	}
}

func defaultHostname() string {
	hostname, _ := os.Hostname()
	return strings.TrimSpace(hostname)
}
