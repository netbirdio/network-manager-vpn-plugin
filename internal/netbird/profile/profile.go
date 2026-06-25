// Package profile maps NetworkManager connection metadata to NetBird daemon profile references.
package profile

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/netbirdio/netbird/client/proto"
	"github.com/netbirdio/network-manager-plugin/internal/netbird/daemonclient"
	nbstatus "github.com/netbirdio/network-manager-plugin/internal/netbird/status"
)

var (
	ErrConflict  = errors.New("conflicting active NetBird profile")
	ErrNotFound  = errors.New("NetBird profile not found")
	ErrAmbiguous = errors.New("ambiguous NetBird profile")
)

// Client is the daemon profile/status subset used for profile selection.
type Client interface {
	GetFeatures(ctx context.Context) (daemonclient.Features, error)
	GetActiveProfile(ctx context.Context) (daemonclient.ProfileRef, error)
	ListProfiles(ctx context.Context, username string) ([]daemonclient.Profile, error)
	AddProfile(ctx context.Context, profile daemonclient.ProfileRef) (daemonclient.ProfileRef, error)
	Status(ctx context.Context, options daemonclient.StatusOptions) (*proto.StatusResponse, error)
}

// PrepareActivation normalizes the requested profile before Login/Up. It clears
// the profile when daemon profiles are disabled and fails when the requested
// profile would switch away from a different daemon profile that is currently
// connected or connecting. It intentionally avoids profile creation so callers
// can fill fallback usernames before EnsureExists creates missing profiles.
func PrepareActivation(ctx context.Context, client Client, desired daemonclient.ProfileRef) (daemonclient.ProfileRef, error) {
	desired = trimProfileRef(desired)

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
	active = trimProfileRef(active)

	if err := checkRunningConflict(ctx, client, active, desired); err != nil {
		return daemonclient.ProfileRef{}, err
	}
	return fillMissingProfileFields(active, desired), nil
}

// EnsureExists resolves an existing daemon profile or creates it when missing.
// NetBird v0.73 profile RPCs require the profile handle to exist before
// SwitchProfile/Login/SetConfig/Up/GetConfig can use it. The returned reference
// keeps the stable display name and uses the daemon-generated ID when available.
func EnsureExists(ctx context.Context, client Client, desired daemonclient.ProfileRef) (daemonclient.ProfileRef, error) {
	desired = trimProfileRef(desired)

	features, err := client.GetFeatures(ctx)
	if err != nil {
		return daemonclient.ProfileRef{}, fmt.Errorf("resolve profile features: %w", err)
	}
	if features.DisableProfiles {
		return daemonclient.ProfileRef{}, nil
	}
	if desired.Empty() || desired.ProfileName == "" || desired.Username == "" {
		return desired, nil
	}

	resolved, err := resolveExistingProfile(ctx, client, desired)
	if err == nil {
		return resolved, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return daemonclient.ProfileRef{}, err
	}

	created, err := client.AddProfile(ctx, desired)
	if err != nil {
		return daemonclient.ProfileRef{}, fmt.Errorf("create profile %s: %w", desired, err)
	}
	created = trimProfileRef(created)
	if created.ProfileName == "" {
		created.ProfileName = desired.ProfileName
	}
	if created.Username == "" {
		created.Username = desired.Username
	}
	return created, nil
}

// Resolve maps a NetworkManager activation to a daemon profile. When daemon
// profiles are disabled, NetworkManager profile metadata is ignored and the
// daemon singleton/default profile is used. When profiles are enabled, a
// different selected daemon profile is allowed only while the daemon engine is
// not running; a connected or connecting different profile remains a safe
// failure instead of an implicit active-session switch.
func Resolve(ctx context.Context, client Client, desired daemonclient.ProfileRef) (daemonclient.ProfileRef, error) {
	desired = trimProfileRef(desired)

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
	active = trimProfileRef(active)
	if desired.Empty() {
		return active, nil
	}

	if err := checkRunningConflict(ctx, client, active, desired); err != nil {
		return daemonclient.ProfileRef{}, err
	}
	prepared := fillMissingProfileFields(active, desired)
	if active.MatchesDesired(prepared) {
		return prepared, nil
	}
	resolved, err := resolveExistingProfile(ctx, client, prepared)
	if err != nil {
		return daemonclient.ProfileRef{}, err
	}

	return fillMissingProfileFields(active, resolved), nil
}

func resolveExistingProfile(ctx context.Context, client Client, desired daemonclient.ProfileRef) (daemonclient.ProfileRef, error) {
	desired = trimProfileRef(desired)
	if desired.Handle() == "" || desired.Username == "" {
		return desired, nil
	}

	profiles, err := client.ListProfiles(ctx, desired.Username)
	if err != nil {
		return daemonclient.ProfileRef{}, fmt.Errorf("validate profile %s: %w", desired, err)
	}

	if desired.ProfileName != "" {
		matches := matchingProfilesByName(profiles, desired.ProfileName)
		switch len(matches) {
		case 0:
			return daemonclient.ProfileRef{}, fmt.Errorf("%w: %s", ErrNotFound, desired)
		case 1:
			return profileRefFromMatch(matches[0], desired.Username), nil
		default:
			return daemonclient.ProfileRef{}, fmt.Errorf("%w: %s has %d matches", ErrAmbiguous, desired, len(matches))
		}
	}

	matches := matchingProfilesByID(profiles, desired.ID)
	switch len(matches) {
	case 0:
		return daemonclient.ProfileRef{}, fmt.Errorf("%w: %s", ErrNotFound, desired)
	case 1:
		return profileRefFromMatch(matches[0], desired.Username), nil
	default:
		return daemonclient.ProfileRef{}, fmt.Errorf("%w: %s has %d matches", ErrAmbiguous, desired, len(matches))
	}
}

func matchingProfilesByName(profiles []daemonclient.Profile, name string) []daemonclient.Profile {
	name = strings.TrimSpace(name)
	matches := []daemonclient.Profile{}
	for _, profile := range profiles {
		if strings.TrimSpace(profile.Name) == name {
			matches = append(matches, profile)
		}
	}
	return matches
}

func matchingProfilesByID(profiles []daemonclient.Profile, id string) []daemonclient.Profile {
	id = strings.TrimSpace(id)
	matches := []daemonclient.Profile{}
	for _, profile := range profiles {
		if strings.TrimSpace(profile.ID) == id {
			matches = append(matches, profile)
		}
	}
	return matches
}

func profileRefFromMatch(match daemonclient.Profile, username string) daemonclient.ProfileRef {
	return daemonclient.ProfileRef{
		ID:          strings.TrimSpace(match.ID),
		ProfileName: strings.TrimSpace(match.Name),
		Username:    strings.TrimSpace(username),
	}
}

func fillMissingProfileFields(active, desired daemonclient.ProfileRef) daemonclient.ProfileRef {
	active = trimProfileRef(active)
	desired = trimProfileRef(desired)
	if active.Empty() || !active.MatchesDesired(desired) {
		return desired
	}
	if desired.ID == "" {
		desired.ID = active.ID
	}
	if desired.ProfileName == "" {
		desired.ProfileName = active.ProfileName
	}
	if desired.Username == "" {
		desired.Username = active.Username
	}
	return desired
}

func trimProfileRef(ref daemonclient.ProfileRef) daemonclient.ProfileRef {
	ref.ID = strings.TrimSpace(ref.ID)
	ref.ProfileName = strings.TrimSpace(ref.ProfileName)
	ref.Username = strings.TrimSpace(ref.Username)
	return ref
}

func checkRunningConflict(ctx context.Context, client Client, active, desired daemonclient.ProfileRef) error {
	active = trimProfileRef(active)
	desired = trimProfileRef(desired)
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
