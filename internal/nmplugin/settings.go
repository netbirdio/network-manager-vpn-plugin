package nmplugin

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/godbus/dbus/v5"
	"github.com/netbirdio/network-manager-plugin/internal/netbird/daemonclient"
)

const vpnSettingName = "vpn"

type activationSettings struct {
	Profile       daemonclient.ProfileRef
	SetupKey      string
	ManagementURL string
	AdminURL      string
	Hostname      string
	InterfaceName string
	PreSharedKey  string
	Hint          string
	AuthMode      string
}

func parseActivationSettings(settings ConnectionSettings) activationSettings {
	values := flattenConnectionSettings(settings)
	profileName := firstSetting(values, "netbird-profile-name", "profile-name", "profileName", "profile")
	if profileName == "" {
		profileName = networkManagerConnectionProfileName(settings)
	}

	return activationSettings{
		Profile: daemonclient.ProfileRef{
			ProfileName: profileName,
			Username:    firstSetting(values, "netbird-username", "username", "user-name", "user"),
		},
		SetupKey:      firstSetting(values, "setup-key", "setupKey", "netbird-setup-key"),
		ManagementURL: firstSetting(values, "management-url", "managementUrl", "netbird-management-url"),
		AdminURL:      firstSetting(values, "admin-url", "adminURL", "netbird-admin-url"),
		Hostname:      firstSetting(values, "hostname", "host-name"),
		InterfaceName: firstSetting(values, "interface-name", "interfaceName", "netbird-interface-name"),
		PreSharedKey:  firstSetting(values, "pre-shared-key", "preshared-key", "preSharedKey"),
		Hint:          firstSetting(values, "hint", "login-hint", "sso-hint"),
		AuthMode:      normalizeAuthMode(firstSetting(values, "auth", "auth-mode", "authentication", "login-mode")),
	}
}

func (s activationSettings) needsSetupKeySecret() bool {
	return s.AuthMode == "setup-key" && strings.TrimSpace(s.SetupKey) == ""
}

func (s activationSettings) shouldLogin(interactive bool) bool {
	if strings.TrimSpace(s.SetupKey) != "" {
		return true
	}
	switch s.AuthMode {
	case "setup-key", "login":
		return true
	case "sso":
		return interactive
	default:
		return false
	}
}

func (s activationSettings) daemonLoginRequest() daemonclient.LoginRequest {
	hostname := strings.TrimSpace(s.Hostname)
	if hostname == "" {
		hostname = defaultHostname()
	}
	return daemonclient.LoginRequest{
		SetupKey:      s.SetupKey,
		ManagementURL: s.ManagementURL,
		AdminURL:      s.AdminURL,
		Hostname:      hostname,
		InterfaceName: s.InterfaceName,
		PreSharedKey:  s.PreSharedKey,
		Profile:       s.Profile,
		Hint:          s.Hint,
	}
}

func mergeActivationDetails(settings activationSettings, details VariantMap) activationSettings {
	if len(details) == 0 {
		return settings
	}
	values := normalizeStringMap(variantMapToStrings(details))
	if value := firstSetting(values, "auth", "auth-mode", "authentication", "login-mode"); value != "" {
		settings.AuthMode = normalizeAuthMode(value)
	}
	if value := firstSetting(values, "setup-key", "setupKey", "netbird-setup-key"); value != "" {
		settings.SetupKey = value
	}
	if value := firstSetting(values, "hint", "login-hint", "sso-hint"); value != "" {
		settings.Hint = value
	}
	return settings
}

func flattenConnectionSettings(settings ConnectionSettings) map[string]string {
	values := map[string]string{}
	for section, sectionValues := range settings {
		sectionName := normalizeSectionName(section)
		for key, variant := range sectionValues {
			mergeConnectionSetting(values, sectionName, key, variant)
		}
	}
	return normalizeStringMap(values)
}

func mergeConnectionSetting(values map[string]string, sectionName string, key string, variant dbus.Variant) {
	keyName := strings.TrimSpace(key)
	keyKind := normalizeSettingKey(keyName)
	if sectionName == vpnSettingName && isDataOrSecretsKey(keyKind) {
		mergeStringMap(values, variantToStringMap(variant))
		return
	}

	nested := variantToStringMap(variant)
	if len(nested) > 0 && isDataOrSecretsKey(keyKind) {
		mergeStringMap(values, nested)
		return
	}

	if value, ok := variantToString(variant); ok {
		values[keyName] = value
	}
}

func isDataOrSecretsKey(keyKind string) bool {
	return keyKind == "data" || keyKind == "secrets"
}

func mergeStringMap(dst map[string]string, src map[string]string) {
	for key, value := range src {
		dst[key] = value
	}
}

func normalizeStringMap(values map[string]string) map[string]string {
	normalized := make(map[string]string, len(values))
	for key, value := range values {
		normalized[normalizeSettingKey(key)] = strings.TrimSpace(value)
	}
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

func networkManagerConnectionProfileName(settings ConnectionSettings) string {
	if uuid := sanitizeProfileNameComponent(firstConnectionSetting(settings, "uuid")); uuid != "" {
		return "nm-" + uuid
	}
	if id := sanitizeProfileNameComponent(firstConnectionSetting(settings, "id")); id != "" {
		return "nm-" + id
	}
	return ""
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
	case "sso", "browser", "interactive":
		return "sso"
	case "login", "force-login":
		return "login"
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
	case map[string]interface{}:
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
