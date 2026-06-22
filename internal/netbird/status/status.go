// Package status translates NetBird daemon status responses into NetworkManager VPN plugin state.
package status

import (
	"fmt"
	"slices"
	"strings"

	"github.com/netbirdio/netbird/client/proto"
)

// State is a daemon-oriented connection state independent from D-Bus/NM enums.
type State int

const (
	Unknown State = iota
	Disconnected
	Connecting
	Connected
	Failed
)

func (s State) String() string {
	switch s {
	case Disconnected:
		return "disconnected"
	case Connecting:
		return "connecting"
	case Connected:
		return "connected"
	case Failed:
		return "failed"
	default:
		return "unknown"
	}
}

// Mapping is the normalized daemon status consumed by the VPN service.
type Mapping struct {
	State         State
	Message       string
	DaemonVersion string
}

func (m Mapping) Ready() bool  { return m.State == Connected }
func (m Mapping) Failed() bool { return m.State == Failed }

// Map converts a daemon StatusResponse into a tolerant state. The daemon status
// string is intentionally treated as free-form text; fullStatus is used as a
// fallback and for failure diagnostics when present.
func Map(resp *proto.StatusResponse) Mapping {
	if resp == nil {
		return Mapping{State: Unknown, Message: "daemon returned no status"}
	}

	raw := strings.TrimSpace(resp.GetStatus())
	mapped := mapStatusString(raw)
	message := raw
	if message == "" {
		message = mapped.String()
	}

	if failureMessage, failed := fullStatusFailure(resp.GetFullStatus()); failed && mapped != Connected {
		return Mapping{State: Failed, Message: failureMessage, DaemonVersion: resp.GetDaemonVersion()}
	}

	if mapped == Unknown {
		mapped, message = mapFullStatus(resp.GetFullStatus(), message)
	}

	return Mapping{State: mapped, Message: message, DaemonVersion: resp.GetDaemonVersion()}
}

func mapStatusString(raw string) State {
	status := normalize(raw)
	if status == "" {
		return Unknown
	}

	if containsAny(status,
		"failed", "failure", "error", "fatal", "invalid", "expired", "unauthorized", "unauthenticated",
		"authentication failed", "login failed",
	) {
		return Failed
	}

	if containsAny(status,
		"not connected", "disconnected", "disconnect", "stopped", "stopping", "down", "offline", "idle",
		"login required", "needs login", "need login", "not logged", "logged out",
	) {
		return Disconnected
	}

	if containsAny(status,
		"connecting", "reconnecting", "starting", "initializing", "initialising", "waiting", "authenticating",
		"logging in", "login pending", "in progress", "updating",
	) {
		return Connecting
	}

	if containsAny(status, "connected", "ready", "running", "up", "active") {
		return Connected
	}

	return Unknown
}

func mapFullStatus(full *proto.FullStatus, fallbackMessage string) (State, string) {
	if full == nil {
		return Unknown, fallbackMessage
	}

	managementConnected := full.GetManagementState().GetConnected()
	signalConnected := full.GetSignalState().GetConnected()
	localPeerIP := strings.TrimSpace(full.GetLocalPeerState().GetIP())

	if managementConnected && signalConnected && localPeerIP != "" {
		return Connected, "daemon full status is connected"
	}
	if managementConnected || signalConnected || localPeerIP != "" {
		return Connecting, "daemon full status is partially connected"
	}
	return Disconnected, fallbackMessage
}

func fullStatusFailure(full *proto.FullStatus) (string, bool) {
	if full == nil {
		return "", false
	}

	if message := managementFailureMessage(full); message != "" {
		return message, true
	}
	if message := signalFailureMessage(full); message != "" {
		return message, true
	}
	if message := relayFailureMessage(full); message != "" {
		return message, true
	}
	if message := dnsFailureMessage(full); message != "" {
		return message, true
	}
	if message := systemEventFailureMessage(full); message != "" {
		return message, true
	}
	return "", false
}

func managementFailureMessage(full *proto.FullStatus) string {
	if message := strings.TrimSpace(full.GetManagementState().GetError()); message != "" {
		return fmt.Sprintf("management connection error: %s", message)
	}
	return ""
}

func signalFailureMessage(full *proto.FullStatus) string {
	if message := strings.TrimSpace(full.GetSignalState().GetError()); message != "" {
		return fmt.Sprintf("signal connection error: %s", message)
	}
	return ""
}

func relayFailureMessage(full *proto.FullStatus) string {
	for _, relay := range full.GetRelays() {
		if message := strings.TrimSpace(relay.GetError()); message != "" {
			return fmt.Sprintf("relay %s error: %s", relay.GetURI(), message)
		}
	}
	return ""
}

func dnsFailureMessage(full *proto.FullStatus) string {
	for _, group := range full.GetDnsServers() {
		if message := strings.TrimSpace(group.GetError()); message != "" {
			return fmt.Sprintf("dns server error: %s", message)
		}
	}
	return ""
}

func systemEventFailureMessage(full *proto.FullStatus) string {
	for _, event := range full.GetEvents() {
		if !isFailureEvent(event) {
			continue
		}
		if message := strings.TrimSpace(event.GetUserMessage()); message != "" {
			return message
		}
		if message := strings.TrimSpace(event.GetMessage()); message != "" {
			return message
		}
	}
	return ""
}

func isFailureEvent(event *proto.SystemEvent) bool {
	switch event.GetSeverity() {
	case proto.SystemEvent_ERROR, proto.SystemEvent_CRITICAL:
		return true
	default:
		return false
	}
}

func normalize(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer("_", " ", "-", " ", ".", " ", ":", " ", ";", " ", ",", " ")
	value = replacer.Replace(value)
	return strings.Join(strings.Fields(value), " ")
}

func containsAny(value string, needles ...string) bool {
	fields := strings.Fields(value)
	for _, needle := range needles {
		needle = normalize(needle)
		if strings.Contains(needle, " ") {
			if strings.Contains(value, needle) {
				return true
			}
			continue
		}
		if slices.Contains(fields, needle) {
			return true
		}
	}
	return false
}
