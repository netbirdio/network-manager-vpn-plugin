package daemonclient_test

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/go-openapi/testify/v2/require"
	"github.com/netbirdio/network-manager-plugin/internal/netbird/daemonclient"
	"github.com/netbirdio/network-manager-plugin/internal/netbird/daemonproto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestDefaultOptionsFromEnv(t *testing.T) {
	t.Setenv(daemonclient.EnvDaemonAddress, "127.0.0.1:33073")
	t.Setenv(daemonclient.EnvDaemonDialTimeout, "5s")
	t.Setenv(daemonclient.EnvDaemonRPCTimeout, "20s")
	t.Setenv(daemonclient.EnvStartDaemon, "true")
	t.Setenv(daemonclient.EnvDaemonService, "netbird-custom")

	options := daemonclient.DefaultOptionsFromEnv()
	require.Equal(t, "127.0.0.1:33073", options.Address)
	require.Equal(t, 5*time.Second, options.DialTimeout)
	require.Equal(t, 20*time.Second, options.RPCTimeout)
	require.True(t, options.StartDaemon)
	require.Equal(t, "netbird-custom", options.DaemonService)
}

func TestGRPCClientClassifiesUnauthenticated(t *testing.T) {
	server := &fakeDaemonServer{upErr: status.Error(codes.Unauthenticated, "login required")}
	address, stop := startFakeDaemonServer(t, server)
	defer stop()

	factory := daemonclient.NewFactory(daemonclient.Options{
		Address:     address,
		DialTimeout: time.Second,
		RPCTimeout:  time.Second,
	})
	client, err := factory.NewClient(context.Background())
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, client.Close())
	})

	err = client.Up(context.Background(), daemonclient.ProfileRef{})
	require.ErrorIs(t, err, daemonclient.ErrAuthenticationRequired)
}

func TestGRPCClientWrapper(t *testing.T) {
	server := &fakeDaemonServer{}
	address, stop := startFakeDaemonServer(t, server)
	defer stop()

	factory := daemonclient.NewFactory(daemonclient.Options{
		Address:     address,
		DialTimeout: time.Second,
		RPCTimeout:  time.Second,
	})
	client, err := factory.NewClient(context.Background())
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, client.Close())
	})

	login, err := client.Login(context.Background(), daemonclient.LoginRequest{
		SetupKey:      "setup",
		ManagementURL: "https://api.example.com",
		Profile:       daemonclient.ProfileRef{ProfileName: "prod", Username: "alice"},
	})
	require.NoError(t, err)
	require.True(t, login.NeedsSSOLogin)
	require.Equal(t, "CODE", login.UserCode)

	require.NoError(t, client.Up(context.Background(), daemonclient.ProfileRef{ProfileName: "prod", Username: "alice"}))

	status, err := client.Status(context.Background(), daemonclient.StatusOptions{GetFullPeerStatus: true, WaitForReady: true})
	require.NoError(t, err)
	require.Equal(t, "connected", status.GetStatus())

	features, err := client.GetFeatures(context.Background())
	require.NoError(t, err)
	require.False(t, features.DisableProfiles)

	active, err := client.GetActiveProfile(context.Background())
	require.NoError(t, err)
	require.Equal(t, daemonclient.ProfileRef{ProfileName: "prod", Username: "alice"}, active)

	profiles, err := client.ListProfiles(context.Background(), "alice")
	require.NoError(t, err)
	require.Equal(t, []daemonclient.Profile{{Name: "prod", IsActive: true}}, profiles)

	require.NoError(t, client.Down(context.Background()))

	server.mu.Lock()
	defer server.mu.Unlock()
	require.Equal(t, "setup", server.login.GetSetupKey())
	require.Equal(t, "prod", server.up.GetProfileName())
	require.Equal(t, "alice", server.up.GetUsername())
	require.True(t, server.downCalled)
	require.True(t, server.status.GetWaitForReady())
}

func startFakeDaemonServer(t *testing.T, daemon *fakeDaemonServer) (string, func()) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	server := grpc.NewServer()
	daemonproto.RegisterDaemonServiceServer(server, daemon)
	go func() {
		_ = server.Serve(listener)
	}()

	return listener.Addr().String(), server.Stop
}

type fakeDaemonServer struct {
	daemonproto.UnimplementedDaemonServiceServer

	mu         sync.Mutex
	login      *daemonproto.LoginRequest
	setConfig  *daemonproto.SetConfigRequest
	up         *daemonproto.UpRequest
	status     *daemonproto.StatusRequest
	upErr      error
	downCalled bool
}

func (f *fakeDaemonServer) Login(ctx context.Context, req *daemonproto.LoginRequest) (*daemonproto.LoginResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.login = req
	return &daemonproto.LoginResponse{NeedsSSOLogin: true, UserCode: "CODE", VerificationURI: "https://login.example.com"}, nil
}

func (f *fakeDaemonServer) WaitSSOLogin(ctx context.Context, req *daemonproto.WaitSSOLoginRequest) (*daemonproto.WaitSSOLoginResponse, error) {
	return &daemonproto.WaitSSOLoginResponse{Email: "alice@example.com"}, nil
}

func (f *fakeDaemonServer) SetConfig(ctx context.Context, req *daemonproto.SetConfigRequest) (*daemonproto.SetConfigResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.setConfig = req
	return &daemonproto.SetConfigResponse{}, nil
}

func (f *fakeDaemonServer) Up(ctx context.Context, req *daemonproto.UpRequest) (*daemonproto.UpResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.up = req
	if f.upErr != nil {
		return nil, f.upErr
	}
	return &daemonproto.UpResponse{}, nil
}

func (f *fakeDaemonServer) Down(ctx context.Context, req *daemonproto.DownRequest) (*daemonproto.DownResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.downCalled = true
	return &daemonproto.DownResponse{}, nil
}

func (f *fakeDaemonServer) Status(ctx context.Context, req *daemonproto.StatusRequest) (*daemonproto.StatusResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status = req
	return &daemonproto.StatusResponse{Status: "connected", DaemonVersion: "test"}, nil
}

func (f *fakeDaemonServer) GetConfig(ctx context.Context, req *daemonproto.GetConfigRequest) (*daemonproto.GetConfigResponse, error) {
	return &daemonproto.GetConfigResponse{InterfaceName: "wt0"}, nil
}

func (f *fakeDaemonServer) GetFeatures(ctx context.Context, req *daemonproto.GetFeaturesRequest) (*daemonproto.GetFeaturesResponse, error) {
	return &daemonproto.GetFeaturesResponse{}, nil
}

func (f *fakeDaemonServer) GetActiveProfile(ctx context.Context, req *daemonproto.GetActiveProfileRequest) (*daemonproto.GetActiveProfileResponse, error) {
	return &daemonproto.GetActiveProfileResponse{ProfileName: "prod", Username: "alice"}, nil
}

func (f *fakeDaemonServer) ListProfiles(ctx context.Context, req *daemonproto.ListProfilesRequest) (*daemonproto.ListProfilesResponse, error) {
	return &daemonproto.ListProfilesResponse{Profiles: []*daemonproto.Profile{{Name: "prod", IsActive: true}}}, nil
}
