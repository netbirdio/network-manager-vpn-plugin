// Package daemonclient wraps the NetBird daemon gRPC API behind the narrow surface needed by the NetworkManager plugin.
package daemonclient

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/netbirdio/network-manager-plugin/internal/netbird/daemonproto"
)

var (
	// ErrAuthenticationRequired marks daemon errors that can be resolved by
	// logging in before retrying the operation.
	ErrAuthenticationRequired = errors.New("netbird daemon authentication required")
)

const (
	// DefaultAddress is the local NetBird daemon gRPC endpoint used by the
	// upstream Linux CLI/service integration. It can be overridden with
	// EnvDaemonAddress.
	DefaultAddress = "unix:///var/run/netbird.sock"

	// DefaultDaemonService is the service identifier passed to the configured init
	// system when daemon autostart is enabled.
	DefaultDaemonService = "netbird"

	// EnvDaemonAddress is the environment variable overriding DefaultAddress.
	EnvDaemonAddress = "NM_NETBIRD_DAEMON_ADDRESS"

	// EnvDaemonDialTimeout is the environment variable overriding the daemon dial timeout.
	EnvDaemonDialTimeout = "NM_NETBIRD_DAEMON_DIAL_TIMEOUT"

	// EnvDaemonRPCTimeout is the environment variable overriding the per-RPC timeout.
	EnvDaemonRPCTimeout = "NM_NETBIRD_DAEMON_RPC_TIMEOUT"

	// EnvStartDaemon is the environment variable enabling daemon autostart.
	EnvStartDaemon = "NM_NETBIRD_START_DAEMON"

	// EnvDaemonService is the environment variable overriding DefaultDaemonService.
	EnvDaemonService = "NM_NETBIRD_DAEMON_SERVICE"
)

// Factory creates short-lived clients connected to the local NetBird daemon.
type Factory interface {
	NewClient(ctx context.Context) (Client, error)
}

// Client is the small daemon surface the NetworkManager plugin needs. Keeping
// this interface narrow prevents D-Bus code from depending on generated gRPC
// bindings directly and makes activation/status behavior testable.
type Client interface {
	Login(ctx context.Context, request LoginRequest) (LoginResponse, error)
	WaitSSOLogin(ctx context.Context, request WaitSSOLoginRequest) (WaitSSOLoginResponse, error)
	UpdateProfile(ctx context.Context, request UpdateProfileRequest) error
	Up(ctx context.Context, profile ProfileRef) error
	Down(ctx context.Context) error
	Status(ctx context.Context, options StatusOptions) (*daemonproto.StatusResponse, error)
	GetConfig(ctx context.Context, profile ProfileRef) (*daemonproto.GetConfigResponse, error)
	GetFeatures(ctx context.Context) (Features, error)
	GetActiveProfile(ctx context.Context) (ProfileRef, error)
	ListProfiles(ctx context.Context, username string) ([]Profile, error)
	Close() error
}

// ProfileRef identifies a NetBird daemon profile. Empty fields mean the daemon
// default/singleton profile should be used.
type ProfileRef struct {
	ProfileName string
	Username    string
}

func (r ProfileRef) Empty() bool {
	return strings.TrimSpace(r.ProfileName) == "" && strings.TrimSpace(r.Username) == ""
}

func (r ProfileRef) Equal(other ProfileRef) bool {
	return strings.TrimSpace(r.ProfileName) == strings.TrimSpace(other.ProfileName) &&
		strings.TrimSpace(r.Username) == strings.TrimSpace(other.Username)
}

// MatchesDesired reports whether r satisfies every non-empty field in desired.
func (r ProfileRef) MatchesDesired(desired ProfileRef) bool {
	if strings.TrimSpace(desired.ProfileName) != "" && strings.TrimSpace(r.ProfileName) != strings.TrimSpace(desired.ProfileName) {
		return false
	}
	if strings.TrimSpace(desired.Username) != "" && strings.TrimSpace(r.Username) != strings.TrimSpace(desired.Username) {
		return false
	}
	return true
}

func (r ProfileRef) String() string {
	profileName := strings.TrimSpace(r.ProfileName)
	username := strings.TrimSpace(r.Username)
	switch {
	case profileName == "" && username == "":
		return "default"
	case username == "":
		return profileName
	case profileName == "":
		return fmt.Sprintf("user %s", username)
	default:
		return fmt.Sprintf("%s/%s", username, profileName)
	}
}

// Profile is a daemon profile returned by ListProfiles.
type Profile struct {
	Name     string
	IsActive bool
}

// Features mirrors the daemon feature flags relevant to this plugin.
type Features struct {
	DisableProfiles       bool
	DisableUpdateSettings bool
	DisableNetworks       bool
}

// LoginRequest contains daemon login data sourced from the NetworkManager VPN
// profile and secrets. Only fields needed for NetworkManager activation are
// represented here.
type LoginRequest struct {
	SetupKey      string
	ManagementURL string
	AdminURL      string
	Hostname      string
	InterfaceName string
	PreSharedKey  string
	Profile       ProfileRef
	Hint          string
}

// LoginResponse is the daemon login result used to drive SSO UX.
type LoginResponse struct {
	NeedsSSOLogin           bool
	UserCode                string
	VerificationURI         string
	VerificationURIComplete string
}

type WaitSSOLoginRequest struct {
	UserCode string
	Hostname string
}

type WaitSSOLoginResponse struct {
	Email string
}

// UpdateProfileRequest contains daemon profile settings sourced from the
// NetworkManager VPN profile. Empty management/admin URLs are interpreted by
// the plugin before calling UpdateProfile.
type UpdateProfileRequest struct {
	Profile       ProfileRef
	ManagementURL string
	AdminURL      string
	InterfaceName string
	PreSharedKey  string
}

// StatusOptions controls daemon status calls.
type StatusOptions struct {
	GetFullPeerStatus bool
	ShouldRunProbes   bool
	WaitForReady      bool
}
