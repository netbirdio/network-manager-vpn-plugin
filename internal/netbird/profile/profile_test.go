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

func TestPrepareActivationFillsMissingUsernameAndIDFromActiveProfile(t *testing.T) {
	client := &fakeProfileClient{active: daemonclient.ProfileRef{ID: "profile-id", ProfileName: "prod", Username: "alice"}}
	got, err := profile.PrepareActivation(context.Background(), client, daemonclient.ProfileRef{ProfileName: "prod"})
	require.NoError(t, err)
	require.Equal(t, daemonclient.ProfileRef{ID: "profile-id", ProfileName: "prod", Username: "alice"}, got)
}

func TestEnsureExistsCreatesMissingProfileWithGeneratedID(t *testing.T) {
	client := &fakeProfileClient{addResponse: daemonclient.ProfileRef{ID: "generated-id"}}
	desired := daemonclient.ProfileRef{ProfileName: "nm-uuid", Username: "alice"}
	got, err := profile.EnsureExists(context.Background(), client, desired)
	require.NoError(t, err)
	require.Equal(t, daemonclient.ProfileRef{ID: "generated-id", ProfileName: "nm-uuid", Username: "alice"}, got)
	require.Equal(t, []daemonclient.ProfileRef{desired}, client.addRequests)
	require.Equal(t, []string{"alice"}, client.listUsernames)
}

func TestEnsureExistsReusesExistingProfileID(t *testing.T) {
	client := &fakeProfileClient{profiles: []daemonclient.Profile{{ID: "existing-id", Name: "nm-uuid"}}}
	got, err := profile.EnsureExists(context.Background(), client, daemonclient.ProfileRef{ProfileName: "nm-uuid", Username: "alice"})
	require.NoError(t, err)
	require.Equal(t, daemonclient.ProfileRef{ID: "existing-id", ProfileName: "nm-uuid", Username: "alice"}, got)
	require.Empty(t, client.addRequests)
}

func TestEnsureExistsFallsBackToDisplayNameWhenGeneratedIDIsEmpty(t *testing.T) {
	client := &fakeProfileClient{}
	desired := daemonclient.ProfileRef{ProfileName: "nm-uuid", Username: "alice"}
	got, err := profile.EnsureExists(context.Background(), client, desired)
	require.NoError(t, err)
	require.Equal(t, desired, got)
	require.Equal(t, []daemonclient.ProfileRef{desired}, client.addRequests)
}

func TestEnsureExistsDuplicateDisplayNamesFailSafely(t *testing.T) {
	client := &fakeProfileClient{profiles: []daemonclient.Profile{
		{ID: "id-1", Name: "nm-uuid"},
		{ID: "id-2", Name: "nm-uuid"},
	}}
	_, err := profile.EnsureExists(context.Background(), client, daemonclient.ProfileRef{ProfileName: "nm-uuid", Username: "alice"})
	require.Error(t, err)
	require.True(t, errors.Is(err, profile.ErrAmbiguous))
	require.Empty(t, client.addRequests)
}

func TestEnsureExistsDuplicateDisplayNamesFailEvenWhenDesiredHasID(t *testing.T) {
	client := &fakeProfileClient{profiles: []daemonclient.Profile{
		{ID: "id-1", Name: "nm-uuid"},
		{ID: "id-2", Name: "nm-uuid"},
	}}
	_, err := profile.EnsureExists(context.Background(), client, daemonclient.ProfileRef{ID: "id-1", ProfileName: "nm-uuid", Username: "alice"})
	require.Error(t, err)
	require.True(t, errors.Is(err, profile.ErrAmbiguous))
	require.Empty(t, client.addRequests)
}

func TestEnsureExistsProfilesDisabledDoesNotListOrAdd(t *testing.T) {
	client := &fakeProfileClient{features: daemonclient.Features{DisableProfiles: true}}
	got, err := profile.EnsureExists(context.Background(), client, daemonclient.ProfileRef{ProfileName: "nm-uuid", Username: "alice"})
	require.NoError(t, err)
	require.True(t, got.Empty())
	require.Empty(t, client.listUsernames)
	require.Empty(t, client.addRequests)
}

func TestResolveProfilesDisabled(t *testing.T) {
	client := &fakeProfileClient{features: daemonclient.Features{DisableProfiles: true}}
	got, err := profile.Resolve(context.Background(), client, daemonclient.ProfileRef{ProfileName: "prod", Username: "alice"})
	require.NoError(t, err)
	require.True(t, got.Empty())
}

func TestResolveMissingProfileMetadataUsesActiveProfile(t *testing.T) {
	client := &fakeProfileClient{active: daemonclient.ProfileRef{ID: "profile-id", ProfileName: "prod", Username: "alice"}}
	got, err := profile.Resolve(context.Background(), client, daemonclient.ProfileRef{})
	require.NoError(t, err)
	require.Equal(t, daemonclient.ProfileRef{ID: "profile-id", ProfileName: "prod", Username: "alice"}, got)
}

func TestResolveActiveMatchingProfile(t *testing.T) {
	client := &fakeProfileClient{active: daemonclient.ProfileRef{ID: "profile-id", ProfileName: "prod", Username: "alice"}}
	got, err := profile.Resolve(context.Background(), client, daemonclient.ProfileRef{ProfileName: "prod"})
	require.NoError(t, err)
	require.Equal(t, daemonclient.ProfileRef{ID: "profile-id", ProfileName: "prod", Username: "alice"}, got)
}

func TestResolveActiveConflictingProfile(t *testing.T) {
	client := &fakeProfileClient{
		active: daemonclient.ProfileRef{ProfileName: "prod", Username: "alice"},
		status: &proto.StatusResponse{Status: "connected"},
	}
	_, err := profile.Resolve(context.Background(), client, daemonclient.ProfileRef{ProfileName: "dev", Username: "alice"})
	require.Error(t, err)
	require.True(t, errors.Is(err, profile.ErrConflict))
}

func TestResolveAllowsDifferentProfileWhenDaemonIsDisconnected(t *testing.T) {
	client := &fakeProfileClient{
		active:   daemonclient.ProfileRef{ProfileName: "prod", Username: "alice"},
		profiles: []daemonclient.Profile{{ID: "dev-id", Name: "dev"}},
		status:   &proto.StatusResponse{Status: "disconnected"},
	}
	got, err := profile.Resolve(context.Background(), client, daemonclient.ProfileRef{ProfileName: "dev", Username: "alice"})
	require.NoError(t, err)
	require.Equal(t, daemonclient.ProfileRef{ID: "dev-id", ProfileName: "dev", Username: "alice"}, got)
}

func TestResolveProfileNotFound(t *testing.T) {
	client := &fakeProfileClient{profiles: []daemonclient.Profile{{Name: "dev"}}}
	_, err := profile.Resolve(context.Background(), client, daemonclient.ProfileRef{ProfileName: "prod", Username: "alice"})
	require.Error(t, err)
	require.True(t, errors.Is(err, profile.ErrNotFound))
}

type fakeProfileClient struct {
	features      daemonclient.Features
	active        daemonclient.ProfileRef
	profiles      []daemonclient.Profile
	status        *proto.StatusResponse
	addResponse   daemonclient.ProfileRef
	addRequests   []daemonclient.ProfileRef
	listUsernames []string
}

func (f *fakeProfileClient) GetFeatures(ctx context.Context) (daemonclient.Features, error) {
	return f.features, nil
}

func (f *fakeProfileClient) GetActiveProfile(ctx context.Context) (daemonclient.ProfileRef, error) {
	return f.active, nil
}

func (f *fakeProfileClient) ListProfiles(ctx context.Context, username string) ([]daemonclient.Profile, error) {
	f.listUsernames = append(f.listUsernames, username)
	return f.profiles, nil
}

func (f *fakeProfileClient) AddProfile(ctx context.Context, ref daemonclient.ProfileRef) (daemonclient.ProfileRef, error) {
	f.addRequests = append(f.addRequests, ref)
	if f.addResponse.ID != "" || f.addResponse.ProfileName != "" || f.addResponse.Username != "" {
		response := f.addResponse
		if response.ProfileName == "" {
			response.ProfileName = ref.ProfileName
		}
		if response.Username == "" {
			response.Username = ref.Username
		}
		return response, nil
	}
	return ref, nil
}

func (f *fakeProfileClient) Status(ctx context.Context, options daemonclient.StatusOptions) (*proto.StatusResponse, error) {
	return f.status, nil
}
