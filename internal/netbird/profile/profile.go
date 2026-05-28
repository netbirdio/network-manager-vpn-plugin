// Package profile maps NetworkManager connection metadata to NetBird daemon profile references.
package profile

import (
	"context"
	"errors"
	"fmt"

	"github.com/netbirdio/network-manager-plugin/internal/netbird/daemonclient"
	"github.com/netbirdio/network-manager-plugin/internal/netbird/daemonproto"
	nbstatus "github.com/netbirdio/network-manager-plugin/internal/netbird/status"
)

var (
	ErrConflict = errors.New("conflicting active NetBird profile")
	ErrNotFound = errors.New("NetBird profile not found")
)

// Client is the daemon profile/status subset used for profile selection.
type Client interface {
	GetFeatures(ctx context.Context) (daemonclient.Features, error)
	GetActiveProfile(ctx context.Context) (daemonclient.ProfileRef, error)
	ListProfiles(ctx context.Context, username string) ([]daemonclient.Profile, error)
	Status(ctx context.Context, options daemonclient.StatusOptions) (*daemonproto.StatusResponse, error)
}

// PrepareActivation normalizes the requested profile before Login/Up. It clears
// the profile when daemon profiles are disabled and fails when the requested
// profile would switch away from a different daemon profile that is currently
// connected or connecting. It intentionally avoids profile existence validation
// so callers can run it before Login creates or updates a profile.
func PrepareActivation(ctx context.Context, client Client, desired daemonclient.ProfileRef) (daemonclient.ProfileRef, error) {
	features, err := client.GetFeatures(ctx)
	if err != nil {
		return daemonclient.ProfileRef{}, fmt.Errorf("resolve profile features: %w", err)
	}
	if features.DisableProfiles {
		return daemonclient.ProfileRef{}, nil
	}
	if desired.Empty() {
		return desired, nil
	}

	active, err := client.GetActiveProfile(ctx)
	if err != nil {
		return daemonclient.ProfileRef{}, fmt.Errorf("resolve active profile: %w", err)
	}

	if err := checkRunningConflict(ctx, client, active, desired); err != nil {
		return daemonclient.ProfileRef{}, err
	}
	return fillMissingProfileFields(active, desired), nil
}

// Resolve maps a NetworkManager activation to a daemon profile. When daemon
// profiles are disabled, NetworkManager profile metadata is ignored and the
// daemon singleton/default profile is used. When profiles are enabled, a
// different selected daemon profile is allowed only while the daemon engine is
// not running; a connected or connecting different profile remains a safe
// failure instead of an implicit active-session switch.
func Resolve(ctx context.Context, client Client, desired daemonclient.ProfileRef) (daemonclient.ProfileRef, error) {
	features, err := client.GetFeatures(ctx)
	if err != nil {
		return daemonclient.ProfileRef{}, fmt.Errorf("resolve profile features: %w", err)
	}
	if features.DisableProfiles {
		return daemonclient.ProfileRef{}, nil
	}

	active, err := client.GetActiveProfile(ctx)
	if err != nil {
		return daemonclient.ProfileRef{}, fmt.Errorf("resolve active profile: %w", err)
	}
	if desired.Empty() {
		return active, nil
	}

	if err := checkRunningConflict(ctx, client, active, desired); err != nil {
		return daemonclient.ProfileRef{}, err
	}
	if err := validateProfileExists(ctx, client, desired); err != nil {
		return daemonclient.ProfileRef{}, err
	}

	return fillMissingProfileFields(active, desired), nil
}

func validateProfileExists(ctx context.Context, client Client, desired daemonclient.ProfileRef) error {
	if desired.ProfileName == "" || desired.Username == "" {
		return nil
	}

	profiles, err := client.ListProfiles(ctx, desired.Username)
	if err != nil {
		return fmt.Errorf("validate profile %s: %w", desired, err)
	}
	if len(profiles) > 0 && !containsProfile(profiles, desired.ProfileName) {
		return fmt.Errorf("%w: %s", ErrNotFound, desired)
	}
	return nil
}

func fillMissingProfileFields(active, desired daemonclient.ProfileRef) daemonclient.ProfileRef {
	if active.Empty() || !active.MatchesDesired(desired) {
		return desired
	}
	if desired.ProfileName == "" {
		desired.ProfileName = active.ProfileName
	}
	if desired.Username == "" {
		desired.Username = active.Username
	}
	return desired
}

func checkRunningConflict(ctx context.Context, client Client, active, desired daemonclient.ProfileRef) error {
	if active.Empty() || active.MatchesDesired(desired) {
		return nil
	}

	running, err := runningEngine(ctx, client)
	if err != nil {
		return fmt.Errorf("check active profile status: %w", err)
	}
	if running {
		return fmt.Errorf("%w: active %s, requested %s", ErrConflict, active, desired)
	}
	return nil
}

func runningEngine(ctx context.Context, client Client) (bool, error) {
	resp, err := client.Status(ctx, daemonclient.StatusOptions{GetFullPeerStatus: true})
	if err != nil {
		return false, err
	}

	switch nbstatus.Map(resp).State {
	case nbstatus.Disconnected, nbstatus.Failed:
		return false, nil
	default:
		return true, nil
	}
}

func containsProfile(profiles []daemonclient.Profile, name string) bool {
	for _, profile := range profiles {
		if profile.Name == name {
			return true
		}
	}
	return false
}
