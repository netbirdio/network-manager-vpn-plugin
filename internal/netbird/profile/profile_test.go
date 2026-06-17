package profile_test

import (
	"context"
	"errors"
	"testing"

	"github.com/go-openapi/testify/v2/require"
	"github.com/netbirdio/netbird/client/proto"
	"github.com/netbirdio/network-manager-plugin/internal/netbird/daemonclient"
	"github.com/netbirdio/network-manager-plugin/internal/netbird/profile"
)

func TestPrepareActivationFillsMissingUsernameFromActiveProfile(t *testing.T) {
	client := fakeProfileClient{active: daemonclient.ProfileRef{ProfileName: "prod", Username: "alice"}}
	got, err := profile.PrepareActivation(context.Background(), client, daemonclient.ProfileRef{ProfileName: "prod"})
	require.NoError(t, err)
	require.Equal(t, daemonclient.ProfileRef{ProfileName: "prod", Username: "alice"}, got)
}

func TestResolveProfilesDisabled(t *testing.T) {
	client := fakeProfileClient{features: daemonclient.Features{DisableProfiles: true}}
	got, err := profile.Resolve(context.Background(), client, daemonclient.ProfileRef{ProfileName: "prod", Username: "alice"})
	require.NoError(t, err)
	require.True(t, got.Empty())
}

func TestResolveMissingProfileMetadataUsesActiveProfile(t *testing.T) {
	client := fakeProfileClient{active: daemonclient.ProfileRef{ProfileName: "prod", Username: "alice"}}
	got, err := profile.Resolve(context.Background(), client, daemonclient.ProfileRef{})
	require.NoError(t, err)
	require.Equal(t, daemonclient.ProfileRef{ProfileName: "prod", Username: "alice"}, got)
}

func TestResolveActiveMatchingProfile(t *testing.T) {
	client := fakeProfileClient{active: daemonclient.ProfileRef{ProfileName: "prod", Username: "alice"}}
	got, err := profile.Resolve(context.Background(), client, daemonclient.ProfileRef{ProfileName: "prod"})
	require.NoError(t, err)
	require.Equal(t, daemonclient.ProfileRef{ProfileName: "prod", Username: "alice"}, got)
}

func TestResolveActiveConflictingProfile(t *testing.T) {
	client := fakeProfileClient{
		active: daemonclient.ProfileRef{ProfileName: "prod", Username: "alice"},
		status: &proto.StatusResponse{Status: "connected"},
	}
	_, err := profile.Resolve(context.Background(), client, daemonclient.ProfileRef{ProfileName: "dev", Username: "alice"})
	require.Error(t, err)
	require.True(t, errors.Is(err, profile.ErrConflict))
}

func TestResolveAllowsDifferentProfileWhenDaemonIsDisconnected(t *testing.T) {
	client := fakeProfileClient{
		active: daemonclient.ProfileRef{ProfileName: "prod", Username: "alice"},
		status: &proto.StatusResponse{Status: "disconnected"},
	}
	got, err := profile.Resolve(context.Background(), client, daemonclient.ProfileRef{ProfileName: "dev", Username: "alice"})
	require.NoError(t, err)
	require.Equal(t, daemonclient.ProfileRef{ProfileName: "dev", Username: "alice"}, got)
}

func TestResolveProfileNotFound(t *testing.T) {
	client := fakeProfileClient{profiles: []daemonclient.Profile{{Name: "dev"}}}
	_, err := profile.Resolve(context.Background(), client, daemonclient.ProfileRef{ProfileName: "prod", Username: "alice"})
	require.Error(t, err)
	require.True(t, errors.Is(err, profile.ErrNotFound))
}

type fakeProfileClient struct {
	features daemonclient.Features
	active   daemonclient.ProfileRef
	profiles []daemonclient.Profile
	status   *proto.StatusResponse
}

func (f fakeProfileClient) GetFeatures(ctx context.Context) (daemonclient.Features, error) {
	return f.features, nil
}

func (f fakeProfileClient) GetActiveProfile(ctx context.Context) (daemonclient.ProfileRef, error) {
	return f.active, nil
}

func (f fakeProfileClient) ListProfiles(ctx context.Context, username string) ([]daemonclient.Profile, error) {
	return f.profiles, nil
}

func (f fakeProfileClient) Status(ctx context.Context, options daemonclient.StatusOptions) (*proto.StatusResponse, error) {
	return f.status, nil
}
