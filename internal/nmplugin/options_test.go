package nmplugin

import (
	"io"
	"log"
	"testing"
	"time"
)

func TestNormalizeServiceOptionsUsesDefaultsForNonPositiveDurations(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input time.Duration
	}{
		{name: "zero", input: 0},
		{name: "negative", input: -time.Nanosecond},
	} {
		t.Run(tc.name, func(t *testing.T) {
			options := normalizeServiceOptions(ServiceOptions{
				ActivationTimeout:  tc.input,
				SSOWaitTimeout:     tc.input,
				OperationTimeout:   tc.input,
				ReadyPollInterval:  tc.input,
				StatusPollInterval: tc.input,
				StatusCallTimeout:  tc.input,
			}, log.New(io.Discard, "", 0))

			assertDuration(t, "ActivationTimeout", options.ActivationTimeout, defaultActivationTimeout)
			assertDuration(t, "SSOWaitTimeout", options.SSOWaitTimeout, defaultSSOWaitTimeout)
			assertDuration(t, "OperationTimeout", options.OperationTimeout, defaultOperationTimeout)
			assertDuration(t, "ReadyPollInterval", options.ReadyPollInterval, defaultReadyPollInterval)
			assertDuration(t, "StatusPollInterval", options.StatusPollInterval, defaultStatusPollInterval)
			assertDuration(t, "StatusCallTimeout", options.StatusCallTimeout, defaultStatusCallTimeout)
		})
	}
}

func assertDuration(t *testing.T, name string, got time.Duration, want time.Duration) {
	t.Helper()
	if got != want {
		t.Fatalf("%s = %s, want %s", name, got, want)
	}
}
