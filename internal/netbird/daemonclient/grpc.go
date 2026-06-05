package daemonclient

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/netbirdio/network-manager-plugin/internal/envconfig"
	"github.com/netbirdio/network-manager-plugin/internal/netbird/daemonproto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

const (
	defaultDialTimeout = 3 * time.Second
	defaultRPCTimeout  = 15 * time.Second
)

// Starter requests that the configured init system start the NetBird daemon service.
type Starter interface {
	Start(ctx context.Context, serviceName string) error
}

// Options configures the concrete gRPC daemon client factory.
type Options struct {
	Address       string
	DialTimeout   time.Duration
	RPCTimeout    time.Duration
	StartDaemon   bool
	DaemonService string
	Starter       Starter
	Logger        *log.Logger
}

// DefaultOptionsFromEnv returns production defaults with environment overrides.
func DefaultOptionsFromEnv() Options {
	options := Options{
		Address:       DefaultAddress,
		DialTimeout:   defaultDialTimeout,
		RPCTimeout:    defaultRPCTimeout,
		DaemonService: DefaultDaemonService,
	}

	if value := envconfig.String(EnvDaemonAddress); value != "" {
		options.Address = value
	}
	if value := envconfig.String(EnvDaemonDialTimeout); value != "" {
		if parsed, err := time.ParseDuration(value); err == nil {
			options.DialTimeout = parsed
		}
	}
	if value := envconfig.String(EnvDaemonRPCTimeout); value != "" {
		if parsed, err := time.ParseDuration(value); err == nil {
			options.RPCTimeout = parsed
		}
	}
	if value := envconfig.String(EnvStartDaemon); value != "" {
		if parsed, err := strconv.ParseBool(value); err == nil {
			options.StartDaemon = parsed
		}
	}
	if value := envconfig.String(EnvDaemonService); value != "" {
		options.DaemonService = value
	}

	return options
}

// NewFactory creates a daemon client factory from options.
func NewFactory(options Options) *GRPCFactory {
	options = normalizeOptions(options)
	return &GRPCFactory{options: options}
}

// GRPCFactory dials the local NetBird daemon on demand.
type GRPCFactory struct {
	options Options
}

func (f *GRPCFactory) NewClient(ctx context.Context) (Client, error) {
	client, err := dial(ctx, f.options)
	if err == nil {
		return client, nil
	}
	if !f.options.StartDaemon {
		return nil, err
	}

	starter := f.options.Starter
	if starter == nil {
		return nil, fmt.Errorf("dial netbird daemon at %s: %w; daemon autostart requested but no init-system starter is configured", normalizeAddress(f.options.Address), err)
	}

	if f.options.Logger != nil {
		f.options.Logger.Printf("netbird daemon unavailable at %s; requesting daemon service start for %s", normalizeAddress(f.options.Address), f.options.DaemonService)
	}
	if startErr := starter.Start(ctx, f.options.DaemonService); startErr != nil {
		return nil, fmt.Errorf("dial netbird daemon at %s: %w; start %s: %v", normalizeAddress(f.options.Address), err, f.options.DaemonService, startErr)
	}

	client, retryErr := dial(ctx, f.options)
	if retryErr != nil {
		return nil, fmt.Errorf("dial netbird daemon at %s after starting %s: %w", normalizeAddress(f.options.Address), f.options.DaemonService, retryErr)
	}
	return client, nil
}

type grpcClient struct {
	conn       *grpc.ClientConn
	client     daemonproto.DaemonServiceClient
	rpcTimeout time.Duration
}

func dial(ctx context.Context, options Options) (*grpcClient, error) {
	options = normalizeOptions(options)
	if options.DialTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, options.DialTimeout)
		defer cancel()
	}

	address := normalizeAddress(options.Address)
	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("create netbird daemon client for %s: %w", address, err)
	}
	if err := waitForReady(ctx, conn); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("dial netbird daemon at %s: %w", address, err)
	}
	return &grpcClient{
		conn:       conn,
		client:     daemonproto.NewDaemonServiceClient(conn),
		rpcTimeout: options.RPCTimeout,
	}, nil
}

func waitForReady(ctx context.Context, conn *grpc.ClientConn) error {
	conn.Connect()
	for {
		state := conn.GetState()
		switch state {
		case connectivity.Ready:
			return nil
		case connectivity.Shutdown:
			return fmt.Errorf("connection shutdown")
		}
		if !conn.WaitForStateChange(ctx, state) {
			if err := ctx.Err(); err != nil {
				return err
			}
			return fmt.Errorf("connection state did not change from %s", state)
		}
	}
}

func (c *grpcClient) Login(ctx context.Context, request LoginRequest) (LoginResponse, error) {
	ctx, cancel := c.callContext(ctx)
	defer cancel()

	req := &daemonproto.LoginRequest{
		SetupKey:            request.SetupKey,
		ManagementUrl:       request.ManagementURL,
		AdminURL:            request.AdminURL,
		Hostname:            request.Hostname,
		IsUnixDesktopClient: true,
	}
	if request.InterfaceName != "" {
		req.InterfaceName = stringPtr(request.InterfaceName)
	}
	if request.PreSharedKey != "" {
		req.OptionalPreSharedKey = stringPtr(request.PreSharedKey)
	}
	if request.Profile.ProfileName != "" {
		req.ProfileName = stringPtr(request.Profile.ProfileName)
	}
	if request.Profile.Username != "" {
		req.Username = stringPtr(request.Profile.Username)
	}
	if request.Hint != "" {
		req.Hint = stringPtr(request.Hint)
	}

	resp, err := c.client.Login(ctx, req)
	if err != nil {
		return LoginResponse{}, daemonError("login", err)
	}
	return LoginResponse{
		NeedsSSOLogin:           resp.GetNeedsSSOLogin(),
		UserCode:                resp.GetUserCode(),
		VerificationURI:         resp.GetVerificationURI(),
		VerificationURIComplete: resp.GetVerificationURIComplete(),
	}, nil
}

func (c *grpcClient) WaitSSOLogin(ctx context.Context, request WaitSSOLoginRequest) (WaitSSOLoginResponse, error) {
	ctx, cancel := c.callContext(ctx)
	defer cancel()

	resp, err := c.client.WaitSSOLogin(ctx, &daemonproto.WaitSSOLoginRequest{
		UserCode: request.UserCode,
		Hostname: request.Hostname,
	})
	if err != nil {
		return WaitSSOLoginResponse{}, daemonError("wait sso login", err)
	}
	return WaitSSOLoginResponse{Email: resp.GetEmail()}, nil
}

func (c *grpcClient) UpdateProfile(ctx context.Context, request UpdateProfileRequest) error {
	ctx, cancel := c.callContext(ctx)
	defer cancel()

	req := &daemonproto.SetConfigRequest{
		Username:      request.Profile.Username,
		ProfileName:   request.Profile.ProfileName,
		ManagementUrl: request.ManagementURL,
		AdminURL:      request.AdminURL,
	}
	if request.InterfaceName != "" {
		req.InterfaceName = stringPtr(request.InterfaceName)
	}
	if request.PreSharedKey != "" {
		req.OptionalPreSharedKey = stringPtr(request.PreSharedKey)
	}
	_, err := c.client.SetConfig(ctx, req)
	if err != nil {
		return daemonError("update profile", err)
	}
	return nil
}

func (c *grpcClient) Up(ctx context.Context, profile ProfileRef) error {
	ctx, cancel := c.callContext(ctx)
	defer cancel()

	req := &daemonproto.UpRequest{}
	if profile.ProfileName != "" {
		req.ProfileName = stringPtr(profile.ProfileName)
	}
	if profile.Username != "" {
		req.Username = stringPtr(profile.Username)
	}
	_, err := c.client.Up(ctx, req)
	if err != nil {
		return daemonError("up", err)
	}
	return nil
}

func (c *grpcClient) Down(ctx context.Context) error {
	ctx, cancel := c.callContext(ctx)
	defer cancel()

	_, err := c.client.Down(ctx, &daemonproto.DownRequest{})
	if err != nil {
		return daemonError("down", err)
	}
	return nil
}

func (c *grpcClient) Status(ctx context.Context, options StatusOptions) (*daemonproto.StatusResponse, error) {
	ctx, cancel := c.callContext(ctx)
	defer cancel()

	req := &daemonproto.StatusRequest{
		GetFullPeerStatus: options.GetFullPeerStatus,
		ShouldRunProbes:   options.ShouldRunProbes,
	}
	if options.WaitForReady {
		req.WaitForReady = boolPtr(true)
	}
	resp, err := c.client.Status(ctx, req)
	if err != nil {
		return nil, daemonError("status", err)
	}
	return resp, nil
}

func (c *grpcClient) GetConfig(ctx context.Context, profile ProfileRef) (*daemonproto.GetConfigResponse, error) {
	ctx, cancel := c.callContext(ctx)
	defer cancel()

	resp, err := c.client.GetConfig(ctx, &daemonproto.GetConfigRequest{
		ProfileName: profile.ProfileName,
		Username:    profile.Username,
	})
	if err != nil {
		return nil, daemonError("get config", err)
	}
	return resp, nil
}

func (c *grpcClient) GetFeatures(ctx context.Context) (Features, error) {
	ctx, cancel := c.callContext(ctx)
	defer cancel()

	resp, err := c.client.GetFeatures(ctx, &daemonproto.GetFeaturesRequest{})
	if err != nil {
		return Features{}, daemonError("get features", err)
	}
	return Features{
		DisableProfiles:       resp.GetDisableProfiles(),
		DisableUpdateSettings: resp.GetDisableUpdateSettings(),
		DisableNetworks:       resp.GetDisableNetworks(),
	}, nil
}

func (c *grpcClient) GetActiveProfile(ctx context.Context) (ProfileRef, error) {
	ctx, cancel := c.callContext(ctx)
	defer cancel()

	resp, err := c.client.GetActiveProfile(ctx, &daemonproto.GetActiveProfileRequest{})
	if err != nil {
		return ProfileRef{}, daemonError("get active profile", err)
	}
	return ProfileRef{ProfileName: resp.GetProfileName(), Username: resp.GetUsername()}, nil
}

func (c *grpcClient) ListProfiles(ctx context.Context, username string) ([]Profile, error) {
	ctx, cancel := c.callContext(ctx)
	defer cancel()

	resp, err := c.client.ListProfiles(ctx, &daemonproto.ListProfilesRequest{Username: username})
	if err != nil {
		return nil, daemonError("list profiles", err)
	}
	profiles := make([]Profile, 0, len(resp.GetProfiles()))
	for _, profile := range resp.GetProfiles() {
		profiles = append(profiles, Profile{Name: profile.GetName(), IsActive: profile.GetIsActive()})
	}
	return profiles, nil
}

func daemonError(operation string, err error) error {
	if status.Code(err) == codes.Unauthenticated {
		err = fmt.Errorf("%w: %w", ErrAuthenticationRequired, err)
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func (c *grpcClient) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

func (c *grpcClient) callContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if c.rpcTimeout <= 0 {
		return context.WithCancel(ctx)
	}
	if _, ok := ctx.Deadline(); ok {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, c.rpcTimeout)
}

func normalizeOptions(options Options) Options {
	if strings.TrimSpace(options.Address) == "" {
		options.Address = DefaultAddress
	}
	if options.DialTimeout == 0 {
		options.DialTimeout = defaultDialTimeout
	}
	if options.RPCTimeout == 0 {
		options.RPCTimeout = defaultRPCTimeout
	}
	if strings.TrimSpace(options.DaemonService) == "" {
		options.DaemonService = DefaultDaemonService
	}
	return options
}

func normalizeAddress(address string) string {
	address = strings.TrimSpace(address)
	if address == "" {
		return DefaultAddress
	}
	if strings.HasPrefix(address, "/") {
		return "unix://" + address
	}
	if strings.HasPrefix(address, "unix:") && !strings.HasPrefix(address, "unix://") {
		path := strings.TrimPrefix(address, "unix:")
		if strings.HasPrefix(path, "//") {
			return "unix:" + path
		}
		if strings.HasPrefix(path, "/") {
			return "unix://" + path
		}
		return "unix:///" + path
	}
	return address
}

func stringPtr(value string) *string { return &value }
func boolPtr(value bool) *bool       { return &value }
