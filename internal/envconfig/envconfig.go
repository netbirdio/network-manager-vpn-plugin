// Package envconfig reads process environment values used for configuration.
package envconfig

import (
	"os"
	"strings"
)

// String returns the trimmed value of key, or an empty string when unset or blank.
func String(key string) string {
	return strings.TrimSpace(os.Getenv(key))
}

// StringDefault returns String(key), or fallback when key is unset or blank.
func StringDefault(key string, fallback string) string {
	if value := String(key); value != "" {
		return value
	}
	return fallback
}
