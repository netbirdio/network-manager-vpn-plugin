package nmplugin

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/netbirdio/network-manager-plugin/internal/netbird/daemonclient"
)

func timeoutCtxWithDefault(parent context.Context, activationTimeout time.Duration, fallback time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, positiveDuration(activationTimeout, fallback))
}

func positiveDuration(value time.Duration, fallback time.Duration) time.Duration {
	if value <= 0 {
		return fallback
	}
	return value
}

func contextCancelled(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}

func nextConsecutiveStatusFailures(current int, failed bool) int {
	if failed {
		return current + 1
	}
	return 0
}

func waitForStatusPollInterval(ctx context.Context, interval <-chan time.Time) bool {
	select {
	case <-ctx.Done():
		return false
	case <-interval:
		return true
	}
}

func (s *Service) logf(format string, args ...any) {
	if s.debug {
		s.logger.Printf(format, args...)
		return
	}

	// Keep non-debug mode quiet but still useful for lifecycle visibility.
	if strings.HasPrefix(format, "Connect") || strings.HasPrefix(format, "Disconnect") {
		s.logger.Printf(format, args...)
	}
}

func normalizeServiceOptions(options ServiceOptions, logger *log.Logger) ServiceOptions {
	if options.ClientFactory == nil {
		clientOptions := daemonclient.DefaultOptionsFromEnv()
		clientOptions.Logger = logger
		options.ClientFactory = daemonclient.NewFactory(clientOptions)
	}
	options.ActivationTimeout = positiveDuration(options.ActivationTimeout, defaultActivationTimeout)
	options.SSOWaitTimeout = positiveDuration(options.SSOWaitTimeout, defaultSSOWaitTimeout)
	options.OperationTimeout = positiveDuration(options.OperationTimeout, defaultOperationTimeout)
	options.ReadyPollInterval = positiveDuration(options.ReadyPollInterval, defaultReadyPollInterval)
	options.StatusPollInterval = positiveDuration(options.StatusPollInterval, defaultStatusPollInterval)
	options.StatusCallTimeout = positiveDuration(options.StatusCallTimeout, defaultStatusCallTimeout)
	return options
}

func summarizeSettings(settings ConnectionSettings) string {
	if len(settings) == 0 {
		return "{}"
	}

	sections := make([]string, 0, len(settings))
	for section, values := range settings {
		keys := make([]string, 0, len(values))
		for key := range values {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		sections = append(sections, fmt.Sprintf("%s[%s]", section, strings.Join(keys, ",")))
	}
	sort.Strings(sections)
	return strings.Join(sections, "; ")
}

func summarizeVariantMap(values VariantMap) string {
	if len(values) == 0 {
		return "{}"
	}

	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return "{" + strings.Join(keys, ",") + "}"
}
