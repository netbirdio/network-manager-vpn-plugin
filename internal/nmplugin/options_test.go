package nmplugin

import (
	"io"
	"log"
	"testing"
	"time"
)

func TestNormalizeServiceOptionsUsesDefaultsForNonPositiveDurations(t *testing.T) {
	options := normalizeServiceOptions(ServiceOptions{
		ActivationTimeout:  -time.Nanosecond,
		SSOWaitTimeout:     -time.Nanosecond,
		OperationTimeout:   -time.Nanosecond,
		ReadyPollInterval:  -time.Nanosecond,
		StatusPollInterval: -time.Nanosecond,
		StatusCallTimeout:  -time.Nanosecond,
	}, log.New(io.Discard, "", 0))

	assertDuration(t, "ActivationTimeout", options.ActivationTimeout, defaultActivationTimeout)
	assertDuration(t, "SSOWaitTimeout", options.SSOWaitTimeout, defaultSSOWaitTimeout)
	assertDuration(t, "OperationTimeout", options.OperationTimeout, defaultOperationTimeout)
	assertDuration(t, "ReadyPollInterval", options.ReadyPollInterval, defaultReadyPollInterval)
	assertDuration(t, "StatusPollInterval", options.StatusPollInterval, defaultStatusPollInterval)
	assertDuration(t, "StatusCallTimeout", options.StatusCallTimeout, defaultStatusCallTimeout)
}

func assertDuration(t *testing.T, name string, got time.Duration, want time.Duration) {
	t.Helper()
	if got != want {
		t.Fatalf("%s = %s, want %s", name, got, want)
	}
}
