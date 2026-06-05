// Package nmplugin implements the NetworkManager VPN plugin D-Bus service for NetBird.
package nmplugin

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"net"
	"net/netip"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
	"github.com/netbirdio/network-manager-plugin/internal/netbird/daemonclient"
	"github.com/netbirdio/network-manager-plugin/internal/netbird/profile"
	nbstatus "github.com/netbirdio/network-manager-plugin/internal/netbird/status"
)

const (
	dbusErrorPropertyReadOnly = "org.freedesktop.DBus.Error.PropertyReadOnly"
	dbusErrorUnknownInterface = "org.freedesktop.DBus.Error.UnknownInterface"
	dbusErrorUnknownProperty  = "org.freedesktop.DBus.Error.UnknownProperty"
	dbusErrorVPNFailed        = "org.freedesktop.NetworkManager.VPN.Error.Failed"
	dbusErrorVPNAlreadyActive = "org.freedesktop.NetworkManager.VPN.Error.AlreadyStarted"

	nmVPNConfigGateway = "gateway"
	nmVPNConfigTUNDev  = "tundev"
	nmVPNConfigHasIP4  = "has-ip4"
	nmVPNConfigHasIP6  = "has-ip6"
)

const (
	defaultActivationTimeout  = 90 * time.Second
	defaultSSOWaitTimeout     = 10 * time.Minute
	defaultOperationTimeout   = 20 * time.Second
	defaultReadyPollInterval  = 500 * time.Millisecond
	defaultStatusPollInterval = 5 * time.Second
	defaultStatusCallTimeout  = 5 * time.Second
)

const (
	interactiveSSORequiredMessage = "This profile needs interactive SSO; rerun with nmcli connection up <name> --ask, or run netbird login first."
	setupKeyRequiredMessage       = "NetBird setup key required."
	ssoRequiredMessage            = "NetBird SSO login required."
)

var (
	errMissingSetupKey      = errors.New("setup-key authentication requested but no setup-key secret was provided")
	errInteractiveSSONeeded = errors.New("interactive SSO required")
	errPromptUnavailable    = errors.New("activation prompt is no longer available")
)

// ServiceOptions configures daemon integration behavior.
type ServiceOptions struct {
	ClientFactory      daemonclient.Factory
	ActivationTimeout  time.Duration
	SSOWaitTimeout     time.Duration
	OperationTimeout   time.Duration
	ReadyPollInterval  time.Duration
	StatusPollInterval time.Duration
	StatusCallTimeout  time.Duration
}

// Service exports a NetworkManager VPN plugin over D-Bus and controls the
// NetBird daemon over its local gRPC API.
type Service struct {
	conn   *dbus.Conn
	path   dbus.ObjectPath
	logger *log.Logger
	debug  bool

	mu    sync.RWMutex
	state ServiceState

	clientFactory      daemonclient.Factory
	activationTimeout  time.Duration
	ssoWaitTimeout     time.Duration
	operationTimeout   time.Duration
	readyPollInterval  time.Duration
	statusPollInterval time.Duration
	statusCallTimeout  time.Duration

	lifecycleMu      sync.Mutex
	activating       bool
	activationCancel context.CancelFunc
	monitorCancel    context.CancelFunc
	client           daemonclient.Client
	activeProfile    daemonclient.ProfileRef
	sessionID        uint64
	activationID     uint64
	prompt           *activationPrompt
}

type activationPromptKind string

const (
	activationPromptSetupKey activationPromptKind = "setup-key"
	activationPromptSSO      activationPromptKind = "sso"
)

type activationPrompt struct {
	activationID uint64
	kind         activationPromptKind
	result       chan activationSettings
}

// NewService creates an initialized VPN plugin service. Call Export before
// requesting the well-known bus name.
func NewService(conn *dbus.Conn, logger *log.Logger, debug bool, options ServiceOptions) *Service {
	if logger == nil {
		logger = log.Default()
	}
	options = normalizeServiceOptions(options, logger)

	return &Service{
		conn:               conn,
		path:               ObjectPath,
		logger:             logger,
		debug:              debug,
		state:              ServiceStateInit,
		clientFactory:      options.ClientFactory,
		activationTimeout:  options.ActivationTimeout,
		ssoWaitTimeout:     options.SSOWaitTimeout,
		operationTimeout:   options.OperationTimeout,
		readyPollInterval:  options.ReadyPollInterval,
		statusPollInterval: options.StatusPollInterval,
		statusCallTimeout:  options.StatusCallTimeout,
	}
}

// Export publishes the plugin, properties, and introspection interfaces at the
// NetworkManager VPN plugin object path.
func (s *Service) Export() error {
	if s.conn == nil {
		return fmt.Errorf("dbus connection is nil")
	}

	if err := s.conn.Export(s, s.path, Interface); err != nil {
		return fmt.Errorf("export %s: %w", Interface, err)
	}

	if err := s.conn.Export(&properties{service: s}, s.path, PropertiesInterface); err != nil {
		return fmt.Errorf("export %s: %w", PropertiesInterface, err)
	}

	if err := s.conn.Export(introspect.Introspectable(IntrospectionXML), s.path, "org.freedesktop.DBus.Introspectable"); err != nil {
		return fmt.Errorf("export introspection: %w", err)
	}

	return nil
}

// State returns the current NetworkManager VPN service state.
func (s *Service) State() ServiceState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state
}

// Connect is called by NetworkManager to start a non-interactive VPN connection.
func (s *Service) Connect(connection ConnectionSettings) *dbus.Error {
	s.logf("Connect(%s)", summarizeSettings(connection))
	return s.startActivation(connection, nil, false)
}

// ConnectInteractive is called by NetworkManager to start a VPN connection and
// allows the plugin to request additional secrets during activation.
func (s *Service) ConnectInteractive(connection ConnectionSettings, details VariantMap) *dbus.Error {
	s.logf("ConnectInteractive(%s, details=%s)", summarizeSettings(connection), summarizeVariantMap(details))
	return s.startActivation(connection, details, true)
}

// NeedSecrets asks whether this connection needs more secrets. The plugin only
// requires NetworkManager secrets for explicitly configured setup-key auth.
func (s *Service) NeedSecrets(settings ConnectionSettings) (string, *dbus.Error) {
	s.logf("NeedSecrets(%s)", summarizeSettings(settings))
	activationSettings := parseActivationSettings(settings)
	if activationSettings.needsSetupKeySecret() || activationSettings.needsSSOHintPrompt() {
		return vpnSettingName, nil
	}
	return "", nil
}

// NewSecrets delivers additional settings after a SecretsRequired signal. During
// an activation, the service uses this to resume a setup-key prompt or to cancel
// an SSO wait when a frontend returns an explicit cancellation marker.
func (s *Service) NewSecrets(connection ConnectionSettings) *dbus.Error {
	s.logf("NewSecrets(%s)", summarizeSettings(connection))
	settings := parseActivationSettings(connection)

	s.lifecycleMu.Lock()
	prompt := s.prompt
	if prompt == nil {
		s.lifecycleMu.Unlock()
		return nil
	}

	if settings.PromptActivationID == "" {
		s.logf("NewSecrets missing activation id; accepting for current in-flight prompt")
	} else if settings.PromptActivationID != formatActivationID(prompt.activationID) {
		s.lifecycleMu.Unlock()
		s.logf("ignoring stale NewSecrets for activation %s", settings.PromptActivationID)
		return nil
	}

	s.handleNewSecretsLocked(prompt, settings)
	s.lifecycleMu.Unlock()
	return nil
}

func (s *Service) handleNewSecretsLocked(prompt *activationPrompt, settings activationSettings) {
	switch prompt.kind {
	case activationPromptSetupKey:
		s.prompt = nil
		select {
		case prompt.result <- settings:
		default:
		}
	case activationPromptSSO:
		if !settings.SSOContinue && !settings.SSOCancel {
			return
		}
		s.prompt = nil
		if settings.SSOCancel && s.activationCancel != nil {
			s.activationCancel()
		}
	}
}

// Disconnect is called by NetworkManager to stop the VPN service. NetBird's
// daemon proto has a global DownRequest, so this intentionally disconnects the
// single daemon engine rather than a profile-scoped instance.
func (s *Service) Disconnect() *dbus.Error {
	s.logf("Disconnect()")
	s.setState(ServiceStateStopping)

	var client daemonclient.Client
	s.lifecycleMu.Lock()
	if s.activationCancel != nil {
		s.activationCancel()
	}
	if s.monitorCancel != nil {
		s.monitorCancel()
		s.monitorCancel = nil
	}
	client = s.client
	s.client = nil
	s.activeProfile = daemonclient.ProfileRef{}
	s.prompt = nil
	s.sessionID++
	s.activationID++
	s.lifecycleMu.Unlock()

	if client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), s.operationTimeout)
		err := client.Down(ctx)
		cancel()
		if closeErr := client.Close(); closeErr != nil {
			s.logger.Printf("close netbird daemon client after disconnect: %v", closeErr)
		}
		if err != nil {
			s.logger.Printf("netbird down failed: %v", err)
			_ = s.EmitFailure(PluginFailureConnectFailed)
			s.setState(ServiceStateStopped)
			return newDBusError(dbusErrorVPNFailed, "netbird down failed: %v", err)
		}
	}

	s.setState(ServiceStateStopped)
	return nil
}

// SetConfig is part of the NetworkManager VPN plugin interface. NetworkManager
// typically consumes Config signals from the plugin; NetBird owns routes/DNS and
// interface state, so the service does not accept NetworkManager IP config.
func (s *Service) SetConfig(config VariantMap) *dbus.Error {
	s.logf("SetConfig(%s)", summarizeVariantMap(config))
	return nil
}

// SetIp4Config logs IPv4 config method calls.
//
//nolint:revive // D-Bus method name must match NetworkManager's SetIp4Config spelling.
func (s *Service) SetIp4Config(config VariantMap) *dbus.Error {
	s.logf("SetIp4Config(%s)", summarizeVariantMap(config))
	return nil
}

// SetIp6Config logs IPv6 config method calls.
//
//nolint:revive // D-Bus method name must match NetworkManager's SetIp6Config spelling.
func (s *Service) SetIp6Config(config VariantMap) *dbus.Error {
	s.logf("SetIp6Config(%s)", summarizeVariantMap(config))
	return nil
}

// SetFailure logs a failure string supplied over D-Bus and cancels any pending
// interactive prompt for the current activation.
func (s *Service) SetFailure(reason string) *dbus.Error {
	s.logf("SetFailure(%q)", reason)

	s.lifecycleMu.Lock()
	if s.activationCancel != nil {
		s.activationCancel()
	}
	s.prompt = nil
	s.lifecycleMu.Unlock()
	return nil
}

// EmitSecretsRequired emits the SecretsRequired signal.
func (s *Service) EmitSecretsRequired(message string, secrets []string) error {
	return s.emit("SecretsRequired", message, secrets)
}

// EmitConfig emits the generic VPN Config signal.
func (s *Service) EmitConfig(config VariantMap) error {
	return s.emit("Config", config)
}

// EmitIp4Config emits the VPN IPv4 Config signal.
//
//nolint:revive // Signal helper follows NetworkManager's Ip4Config spelling.
func (s *Service) EmitIp4Config(config VariantMap) error {
	return s.emit("Ip4Config", config)
}

// EmitIp6Config emits the VPN IPv6 Config signal.
//
//nolint:revive // Signal helper follows NetworkManager's Ip6Config spelling.
func (s *Service) EmitIp6Config(config VariantMap) error {
	return s.emit("Ip6Config", config)
}

// EmitLoginBanner emits the LoginBanner signal.
func (s *Service) EmitLoginBanner(banner string) error {
	return s.emit("LoginBanner", banner)
}

// EmitFailure emits the Failure signal.
func (s *Service) EmitFailure(reason PluginFailure) error {
	return s.emit("Failure", uint32(reason))
}

// startActivation reserves the activation slot and returns to NetworkManager
// immediately. The long-running daemon work continues in runActivation and
// reports progress via StateChanged, Config, LoginBanner, and Failure signals.
func (s *Service) startActivation(connection ConnectionSettings, details VariantMap, interactive bool) *dbus.Error {
	activationCtx, activationCancel, activationID, dbusErr := s.reserveActivation()
	if dbusErr != nil {
		return dbusErr
	}

	s.setState(ServiceStateStarting)
	go s.runActivation(activationCtx, activationCancel, activationID, connection, details, interactive)
	return nil
}

func (s *Service) runActivation(
	activationCtx context.Context,
	activationCancel context.CancelFunc,
	activationID uint64,
	connection ConnectionSettings,
	details VariantMap,
	interactive bool,
) {
	ctx, timeoutCancel := s.activationPhaseContext(activationCtx)
	var client daemonclient.Client
	success := false
	daemonUp := false
	defer func() {
		s.finishActivationAttempt(activationID, success, daemonUp, client, activationCancel, timeoutCancel)
	}()

	settings := mergeActivationDetails(parseActivationSettings(connection), details)
	if settings.AuthMode == "sso" && !interactive {
		s.failActivation(PluginFailureLoginFailed, errInteractiveSSONeeded)
		return
	}

	var err error
	client, err = s.clientFactory.NewClient(ctx)
	if err != nil {
		s.failActivation(PluginFailureConnectFailed, fmt.Errorf("connect to netbird daemon: %w", err))
		return
	}

	preparedProfile, err := profile.PrepareActivation(ctx, client, settings.Profile)
	if err != nil {
		s.failActivation(PluginFailureConnectFailed, err)
		return
	}
	settings.Profile = preparedProfile
	if settings.Profile.ProfileName != "" && settings.Profile.Username == "" {
		settings.Profile.Username = currentProcessUsername()
	}

	waitedForSSO, err := s.authenticate(ctx, activationCtx, activationID, client, settings, interactive)
	if err != nil {
		s.failActivation(PluginFailureLoginFailed, err)
		return
	}
	ctx, timeoutCancel = s.resetActivationPhaseAfterSSO(activationCtx, ctx, timeoutCancel, waitedForSSO)

	if err := s.updateDaemonProfile(ctx, client, settings); err != nil {
		s.failActivation(PluginFailureConnectFailed, err)
		return
	}

	resolvedProfile, err := profile.Resolve(ctx, client, settings.Profile)
	if err != nil {
		s.failActivation(PluginFailureConnectFailed, err)
		return
	}

	var failure PluginFailure
	ctx, timeoutCancel, failure, err = s.upWithAuthenticationRetry(ctx, activationCtx, activationID, timeoutCancel, client, resolvedProfile, settings, interactive)
	if err != nil {
		s.failActivation(failure, err)
		return
	}
	daemonUp = true

	if err := s.waitForReady(ctx, client); err != nil {
		s.failActivation(PluginFailureConnectFailed, err)
		return
	}

	if err := s.emitNetworkManagerConfig(ctx, client, resolvedProfile, settings); err != nil {
		s.failActivation(PluginFailureBadIPConfig, err)
		return
	}

	sessionID, completed := s.completeActivation(activationCtx, client, resolvedProfile)
	if !completed {
		return
	}
	success = true
	s.setStateForActiveSession(sessionID, ServiceStateStarted)
}

func (s *Service) finishActivationAttempt(
	activationID uint64,
	success bool,
	daemonUp bool,
	client daemonclient.Client,
	activationCancel context.CancelFunc,
	timeoutCancel context.CancelFunc,
) {
	timeoutCancel()
	s.clearPromptForActivation(activationID)
	if !success && daemonUp && client != nil {
		downCtx, downCancel := context.WithTimeout(context.Background(), s.operationTimeout)
		if err := client.Down(downCtx); err != nil {
			s.logger.Printf("netbird down after failed activation: %v", err)
		}
		downCancel()
	}

	activationCancel()
	if success {
		return
	}
	s.finishFailedActivation()
	if client != nil {
		_ = client.Close()
	}
}

func (s *Service) upWithAuthenticationRetry(
	ctx context.Context,
	activationCtx context.Context,
	activationID uint64,
	timeoutCancel context.CancelFunc,
	client daemonclient.Client,
	resolvedProfile daemonclient.ProfileRef,
	settings activationSettings,
	interactive bool,
) (context.Context, context.CancelFunc, PluginFailure, error) {
	err := client.Up(ctx, resolvedProfile)
	if err == nil {
		return ctx, timeoutCancel, 0, nil
	}
	if !shouldRetryUpWithSSO(err, settings, interactive) {
		return ctx, timeoutCancel, PluginFailureConnectFailed, err
	}

	settings.AuthMode = "sso"
	waitedForSSO, loginErr := s.authenticate(ctx, activationCtx, activationID, client, settings, interactive)
	if loginErr != nil {
		return ctx, timeoutCancel, PluginFailureLoginFailed, loginErr
	}
	ctx, timeoutCancel = s.resetActivationPhaseAfterSSO(activationCtx, ctx, timeoutCancel, waitedForSSO)

	if err := client.Up(ctx, resolvedProfile); err != nil {
		return ctx, timeoutCancel, PluginFailureConnectFailed, err
	}
	return ctx, timeoutCancel, 0, nil
}

func shouldRetryUpWithSSO(err error, settings activationSettings, interactive bool) bool {
	return interactive && !settings.shouldLogin(interactive) && errors.Is(err, daemonclient.ErrAuthenticationRequired)
}

func (s *Service) resetActivationPhaseAfterSSO(
	activationCtx context.Context,
	currentCtx context.Context,
	currentCancel context.CancelFunc,
	waitedForSSO bool,
) (context.Context, context.CancelFunc) {
	if !waitedForSSO {
		return currentCtx, currentCancel
	}
	currentCancel()
	return s.activationPhaseContext(activationCtx)
}

func (s *Service) updateDaemonProfile(ctx context.Context, client daemonclient.Client, settings activationSettings) error {
	if !settings.shouldUpdateProfile() {
		return nil
	}

	features, err := client.GetFeatures(ctx)
	if err != nil {
		return fmt.Errorf("check daemon profile update support: %w", err)
	}
	if features.DisableUpdateSettings {
		return nil
	}
	if err := client.UpdateProfile(ctx, settings.daemonUpdateProfileRequest()); err != nil {
		return fmt.Errorf("update daemon profile from NetworkManager settings: %w", err)
	}
	return nil
}

func (s *Service) authenticate(
	ctx context.Context,
	activationCtx context.Context,
	activationID uint64,
	client daemonclient.Client,
	settings activationSettings,
	interactive bool,
) (bool, error) {
	if !settings.shouldLogin(interactive) {
		return false, nil
	}
	if settings.AuthMode == "setup-key" && strings.TrimSpace(settings.SetupKey) == "" {
		if !interactive {
			return false, errMissingSetupKey
		}
		var err error
		settings, err = s.waitForSetupKeySecret(ctx, activationID, settings)
		if err != nil {
			return false, err
		}
	}

	response, err := client.Login(ctx, settings.daemonLoginRequest())
	if err != nil {
		return false, err
	}
	if !response.NeedsSSOLogin {
		return false, nil
	}
	if !interactive {
		return false, errInteractiveSSONeeded
	}

	if err := s.EmitLoginBanner(formatSSOLoginBanner(response)); err != nil {
		s.logger.Printf("emit SSO login banner failed: %v", err)
	}
	s.startSSOPrompt(activationID, response, settings)
	// The vendored LoginResponse does not expose a device-code expiry, so use the
	// configured SSO wait timeout instead of the shorter activation phase timeout.
	ssoCtx, cancel := s.ssoWaitContext(activationCtx)
	defer cancel()
	_, err = client.WaitSSOLogin(ssoCtx, daemonclient.WaitSSOLoginRequest{
		UserCode: response.UserCode,
		Hostname: settings.daemonLoginRequest().Hostname,
	})
	if err != nil {
		if ssoCtx.Err() != nil && activationCtx.Err() == nil {
			return true, fmt.Errorf("timeout waiting for SSO login: %w", err)
		}
		return true, err
	}
	return true, nil
}

func (s *Service) waitForSetupKeySecret(ctx context.Context, activationID uint64, settings activationSettings) (activationSettings, error) {
	prompt := &activationPrompt{
		activationID: activationID,
		kind:         activationPromptSetupKey,
		result:       make(chan activationSettings, 1),
	}
	if !s.registerActivationPrompt(prompt) {
		return settings, errPromptUnavailable
	}
	if err := s.EmitSecretsRequired(setupKeyRequiredMessage, setupKeyPromptHints(activationID)); err != nil {
		s.logger.Printf("emit setup-key SecretsRequired failed: %v", err)
	}

	select {
	case delivered := <-prompt.result:
		settings = mergePromptSettings(settings, delivered)
		if strings.TrimSpace(settings.SetupKey) == "" {
			return settings, errMissingSetupKey
		}
		return settings, nil
	case <-ctx.Done():
		return settings, fmt.Errorf("timeout waiting for setup-key secret: %w", ctx.Err())
	}
}

func (s *Service) startSSOPrompt(activationID uint64, response daemonclient.LoginResponse, settings activationSettings) {
	prompt := &activationPrompt{
		activationID: activationID,
		kind:         activationPromptSSO,
	}
	if !s.registerActivationPrompt(prompt) {
		return
	}
	if err := s.EmitSecretsRequired(ssoRequiredMessage, ssoPromptHints(activationID, response, settings)); err != nil {
		s.logger.Printf("emit SSO SecretsRequired failed: %v", err)
	}
}

func (s *Service) registerActivationPrompt(prompt *activationPrompt) bool {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()

	if !s.activating || prompt.activationID != s.activationID {
		return false
	}
	s.prompt = prompt
	return true
}

func (s *Service) clearPromptForActivation(activationID uint64) {
	s.lifecycleMu.Lock()
	if s.prompt != nil && s.prompt.activationID == activationID {
		s.prompt = nil
	}
	s.lifecycleMu.Unlock()
}

func mergePromptSettings(current activationSettings, delivered activationSettings) activationSettings {
	if delivered.SetupKey != "" {
		current.SetupKey = delivered.SetupKey
	}
	if delivered.Hint != "" {
		current.Hint = delivered.Hint
	}
	return current
}

func setupKeyPromptHints(activationID uint64) []string {
	return []string{
		"setup-key",
		formatPromptHint(netbirdPromptActivationID, formatActivationID(activationID)),
	}
}

func ssoPromptHints(activationID uint64, response daemonclient.LoginResponse, settings activationSettings) []string {
	hints := []string{
		formatPromptHint(netbirdPromptActivationID, formatActivationID(activationID)),
		formatPromptHint(netbirdSSOHint, "true"),
	}
	if response.VerificationURI != "" {
		hints = append(hints, formatPromptHint(netbirdSSOVerificationURIHint, response.VerificationURI))
	}
	if response.VerificationURIComplete != "" {
		hints = append(hints, formatPromptHint(netbirdSSOVerificationURIComplete, response.VerificationURIComplete))
	}
	if response.UserCode != "" {
		hints = append(hints, formatPromptHint(netbirdSSOUserCodeHint, response.UserCode))
	}
	if settings.Hint != "" {
		hints = append(hints, formatPromptHint(netbirdSSOLoginHint, settings.Hint))
	}
	return hints
}

func formatPromptHint(key string, value string) string {
	return key + "=" + value
}

func formatActivationID(id uint64) string {
	return strconv.FormatUint(id, 10)
}

func (s *Service) activationPhaseContext(parent context.Context) (context.Context, context.CancelFunc) {
	if s.activationTimeout <= 0 {
		return context.WithCancel(parent)
	}
	return context.WithTimeout(parent, s.activationTimeout)
}

func (s *Service) ssoWaitContext(parent context.Context) (context.Context, context.CancelFunc) {
	if s.ssoWaitTimeout <= 0 {
		return context.WithCancel(parent)
	}
	return context.WithTimeout(parent, s.ssoWaitTimeout)
}

func (s *Service) emitNetworkManagerConfig(ctx context.Context, client daemonclient.Client, profile daemonclient.ProfileRef, settings activationSettings) error {
	interfaceName, gatewayURL := s.networkManagerConfigMetadata(ctx, client, profile, settings)

	gateway, err := networkManagerGatewayVariant(ctx, gatewayURL)
	if err != nil {
		return err
	}

	// NetworkManager does not consider a VPN activation complete until the plugin
	// sends configuration. NetBird owns the interface, addresses, routes, DNS, and
	// firewall state, so send a minimal generic config that says no NM-managed IP
	// configuration is needed while still identifying the daemon-created tunnel
	// interface when it is known. NetworkManager still requires the generic
	// external gateway metadata; a configured NetBird control-plane endpoint is
	// the stable external address available from the daemon config/profile.
	config := VariantMap{
		nmVPNConfigGateway: gateway,
		nmVPNConfigHasIP4:  dbus.MakeVariant(false),
		nmVPNConfigHasIP6:  dbus.MakeVariant(false),
	}
	if interfaceName != "" {
		config[nmVPNConfigTUNDev] = dbus.MakeVariant(interfaceName)
	}

	return s.EmitConfig(config)
}

func (s *Service) networkManagerConfigMetadata(ctx context.Context, client daemonclient.Client, profile daemonclient.ProfileRef, settings activationSettings) (string, string) {
	interfaceName := strings.TrimSpace(settings.InterfaceName)
	gatewayURL := strings.TrimSpace(settings.ManagementURL)
	if gatewayURL == "" {
		gatewayURL = strings.TrimSpace(settings.AdminURL)
	}
	if interfaceName != "" && gatewayURL != "" {
		return interfaceName, gatewayURL
	}

	config, err := client.GetConfig(ctx, profile)
	if err != nil {
		s.logger.Printf("read netbird daemon config for NetworkManager metadata failed: %v", err)
		return interfaceName, gatewayURL
	}
	if config == nil {
		return interfaceName, gatewayURL
	}
	if interfaceName == "" {
		interfaceName = normalizeInterfaceName(config.GetInterfaceName())
	}
	if gatewayURL == "" {
		gatewayURL = strings.TrimSpace(config.GetManagementUrl())
	}
	if gatewayURL == "" {
		gatewayURL = strings.TrimSpace(config.GetAdminURL())
	}
	return interfaceName, gatewayURL
}

func networkManagerGatewayVariant(ctx context.Context, rawURL string) (dbus.Variant, error) {
	host := networkManagerGatewayHost(rawURL)
	if host == "" {
		return dbus.Variant{}, fmt.Errorf("management-url or admin-url is required for NetworkManager VPN gateway metadata")
	}

	if addr, err := netip.ParseAddr(host); err == nil {
		if gateway, ok := gatewayVariantForAddr(addr); ok {
			return gateway, nil
		}
		return dbus.Variant{}, fmt.Errorf("management-url host %q is not a usable NetworkManager VPN gateway address", host)
	}

	addrs, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return dbus.Variant{}, fmt.Errorf("resolve management-url host %q for NetworkManager VPN gateway metadata: %w", host, err)
	}
	for _, addr := range addrs {
		if gateway, ok := gatewayVariantForAddr(addr); ok {
			return gateway, nil
		}
	}
	return dbus.Variant{}, fmt.Errorf("resolve management-url host %q for NetworkManager VPN gateway metadata: no usable IP address", host)
}

func networkManagerGatewayHost(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}

	if parsed, err := url.Parse(rawURL); err == nil {
		if host := strings.TrimSpace(parsed.Hostname()); host != "" {
			return strings.Trim(host, "[]")
		}
	}
	if parsed, err := url.Parse("//" + rawURL); err == nil {
		if host := strings.TrimSpace(parsed.Hostname()); host != "" {
			return strings.Trim(host, "[]")
		}
	}
	if host, _, err := net.SplitHostPort(rawURL); err == nil {
		return strings.Trim(strings.TrimSpace(host), "[]")
	}
	return strings.Trim(rawURL, "[]")
}

func gatewayVariantForAddr(addr netip.Addr) (dbus.Variant, bool) {
	addr = addr.Unmap()
	if !addr.IsValid() || addr.IsUnspecified() {
		return dbus.Variant{}, false
	}
	if addr.Is4() {
		bytes := addr.As4()
		return dbus.MakeVariant(binary.NativeEndian.Uint32(bytes[:])), true
	}
	if addr.Is6() {
		bytes := addr.As16()
		return dbus.MakeVariant(append([]byte(nil), bytes[:]...)), true
	}
	return dbus.Variant{}, false
}

func (s *Service) waitForReady(ctx context.Context, client daemonclient.Client) error {
	interval := s.readyPollInterval
	if interval <= 0 {
		interval = defaultReadyPollInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var lastErr error
	var lastMessage string
	for {
		resp, err := client.Status(ctx, daemonclient.StatusOptions{GetFullPeerStatus: true})
		if err != nil {
			lastErr = err
		} else {
			mapped := nbstatus.Map(resp)
			lastMessage = mapped.Message
			s.logDaemonStatus(mapped)
			if mapped.Ready() {
				return nil
			}
			if mapped.Failed() {
				return fmt.Errorf("daemon reported failure while connecting: %s", mapped.Message)
			}
		}

		select {
		case <-ctx.Done():
			return readyWaitError(ctx.Err(), lastErr, lastMessage)
		case <-ticker.C:
		}
	}
}

func readyWaitError(ctxErr error, lastErr error, lastMessage string) error {
	if lastErr != nil {
		return fmt.Errorf("timeout waiting for netbird ready: %w (last error: %v)", ctxErr, lastErr)
	}
	if lastMessage != "" {
		return fmt.Errorf("timeout waiting for netbird ready: %w (last status: %s)", ctxErr, lastMessage)
	}
	return fmt.Errorf("timeout waiting for netbird ready: %w", ctxErr)
}

func (s *Service) logDaemonStatus(mapped nbstatus.Mapping) {
	if mapped.DaemonVersion != "" {
		s.logf("netbird daemon version %s status=%s", mapped.DaemonVersion, mapped.State)
	}
}

func (s *Service) reserveActivation() (context.Context, context.CancelFunc, uint64, *dbus.Error) {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()

	if s.activating || s.client != nil {
		return nil, nil, 0, newDBusError(dbusErrorVPNAlreadyActive, "a NetBird activation is already in progress or active")
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.activationID++
	activationID := s.activationID
	s.activating = true
	s.activationCancel = cancel
	s.prompt = nil
	return ctx, cancel, activationID, nil
}

func (s *Service) finishFailedActivation() {
	s.lifecycleMu.Lock()
	s.activating = false
	s.activationCancel = nil
	s.prompt = nil
	s.lifecycleMu.Unlock()
}

func (s *Service) completeActivation(activationCtx context.Context, client daemonclient.Client, activeProfile daemonclient.ProfileRef) (uint64, bool) {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()

	if activationCtx.Err() != nil || !s.activating {
		return 0, false
	}

	s.activating = false
	s.activationCancel = nil
	s.prompt = nil
	s.client = client
	s.activeProfile = activeProfile
	s.sessionID++
	sessionID := s.sessionID
	s.startMonitorLocked(client, sessionID)
	return sessionID, true
}

func (s *Service) setStateForActiveSession(sessionID uint64, state ServiceState) bool {
	s.lifecycleMu.Lock()
	if s.sessionID != sessionID || s.client == nil {
		s.lifecycleMu.Unlock()
		return false
	}

	changed := s.setStateValue(state)
	s.lifecycleMu.Unlock()

	if changed {
		s.emitStateChanged(state)
	}
	return true
}

func (s *Service) startMonitorLocked(client daemonclient.Client, sessionID uint64) {
	if s.monitorCancel != nil {
		s.monitorCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.monitorCancel = cancel
	go s.monitorStatus(ctx, client, sessionID)
}

func (s *Service) monitorStatus(ctx context.Context, client daemonclient.Client, sessionID uint64) {
	interval := s.statusPollInterval
	if interval <= 0 {
		interval = defaultStatusPollInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if keepGoing := s.pollDaemonStatus(ctx, client, sessionID); !keepGoing {
			return
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Service) pollDaemonStatus(ctx context.Context, client daemonclient.Client, sessionID uint64) bool {
	callTimeout := s.statusCallTimeout
	if callTimeout <= 0 {
		callTimeout = defaultStatusCallTimeout
	}
	callCtx, cancel := context.WithTimeout(ctx, callTimeout)
	resp, err := client.Status(callCtx, daemonclient.StatusOptions{GetFullPeerStatus: true})
	cancel()
	if err != nil {
		if ctx.Err() == nil {
			s.logger.Printf("netbird status poll failed: %v", err)
		}
		return true
	}

	mapped := nbstatus.Map(resp)
	s.logDaemonStatus(mapped)

	switch mapped.State {
	case nbstatus.Connected:
		s.setState(ServiceStateStarted)
	case nbstatus.Connecting:
		s.setState(ServiceStateStarting)
	case nbstatus.Disconnected:
		s.setState(ServiceStateStopped)
		s.clearActiveSession(sessionID, true)
		return false
	case nbstatus.Failed:
		s.logger.Printf("netbird daemon reported failure: %s", mapped.Message)
		_ = s.EmitFailure(PluginFailureConnectFailed)
		s.setState(ServiceStateStopped)
		s.clearActiveSession(sessionID, true)
		return false
	}
	return true
}

func (s *Service) clearActiveSession(sessionID uint64, closeClient bool) {
	var client daemonclient.Client
	s.lifecycleMu.Lock()
	if s.sessionID == sessionID {
		if s.monitorCancel != nil {
			s.monitorCancel()
			s.monitorCancel = nil
		}
		client = s.client
		s.client = nil
		s.activeProfile = daemonclient.ProfileRef{}
	}
	s.lifecycleMu.Unlock()

	if closeClient && client != nil {
		if err := client.Close(); err != nil {
			s.logger.Printf("close netbird daemon client: %v", err)
		}
	}
}

func (s *Service) failActivation(failure PluginFailure, err error) {
	s.logger.Printf("netbird activation failed: %v", err)
	if errors.Is(err, errInteractiveSSONeeded) {
		_ = s.EmitLoginBanner(interactiveSSORequiredMessage)
	}
	_ = s.EmitFailure(failure)
	s.setState(ServiceStateStopped)
}

func (s *Service) setState(state ServiceState) {
	if s.setStateValue(state) {
		s.emitStateChanged(state)
	}
}

func (s *Service) setStateValue(state ServiceState) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state == state {
		return false
	}
	s.state = state
	return true
}

func (s *Service) emitStateChanged(state ServiceState) {
	if err := s.emit("StateChanged", uint32(state)); err != nil {
		s.logger.Printf("emit StateChanged failed: %v", err)
	}

	changed := map[string]dbus.Variant{
		"State": dbus.MakeVariant(uint32(state)),
	}
	if err := s.conn.Emit(s.path, PropertiesInterface+".PropertiesChanged", Interface, changed, []string{}); err != nil {
		s.logger.Printf("emit PropertiesChanged failed: %v", err)
	}
}

func (s *Service) emit(signal string, args ...interface{}) error {
	name := Interface + "." + signal
	if s.debug {
		s.logger.Printf("emit %s", name)
	}
	return s.conn.Emit(s.path, name, args...)
}

func (s *Service) logf(format string, args ...interface{}) {
	if s.debug {
		s.logger.Printf(format, args...)
		return
	}

	// Keep non-debug mode quiet but still useful for lifecycle visibility.
	if strings.HasPrefix(format, "Connect") || strings.HasPrefix(format, "Disconnect") {
		s.logger.Printf(format, args...)
	}
}

func normalizeServiceOptions(options ServiceOptions, logger *log.Logger) ServiceOptions {
	if options.ClientFactory == nil {
		clientOptions := daemonclient.DefaultOptionsFromEnv()
		clientOptions.Logger = logger
		options.ClientFactory = daemonclient.NewFactory(clientOptions)
	}
	if options.ActivationTimeout == 0 {
		options.ActivationTimeout = defaultActivationTimeout
	}
	if options.SSOWaitTimeout == 0 {
		options.SSOWaitTimeout = defaultSSOWaitTimeout
	}
	if options.OperationTimeout == 0 {
		options.OperationTimeout = defaultOperationTimeout
	}
	if options.ReadyPollInterval == 0 {
		options.ReadyPollInterval = defaultReadyPollInterval
	}
	if options.StatusPollInterval == 0 {
		options.StatusPollInterval = defaultStatusPollInterval
	}
	if options.StatusCallTimeout == 0 {
		options.StatusCallTimeout = defaultStatusCallTimeout
	}
	return options
}

func formatSSOLoginBanner(response daemonclient.LoginResponse) string {
	parts := []string{"NetBird SSO login required."}
	if response.VerificationURIComplete != "" {
		parts = append(parts, "Open: "+response.VerificationURIComplete)
	} else if response.VerificationURI != "" {
		parts = append(parts, "Open: "+response.VerificationURI)
	}
	if response.UserCode != "" {
		parts = append(parts, "Code: "+response.UserCode)
	}
	return strings.Join(parts, " ")
}

type properties struct {
	service *Service
}

func (p *properties) Get(interfaceName string, propertyName string) (dbus.Variant, *dbus.Error) {
	if interfaceName != Interface {
		return dbus.Variant{}, newDBusError(dbusErrorUnknownInterface, "unknown interface %q", interfaceName)
	}

	switch propertyName {
	case "State":
		return dbus.MakeVariant(uint32(p.service.State())), nil
	default:
		return dbus.Variant{}, newDBusError(dbusErrorUnknownProperty, "unknown property %q", propertyName)
	}
}

func (p *properties) GetAll(interfaceName string) (map[string]dbus.Variant, *dbus.Error) {
	if interfaceName != Interface {
		return nil, newDBusError(dbusErrorUnknownInterface, "unknown interface %q", interfaceName)
	}

	return map[string]dbus.Variant{
		"State": dbus.MakeVariant(uint32(p.service.State())),
	}, nil
}

func (p *properties) Set(interfaceName string, propertyName string, value dbus.Variant) *dbus.Error {
	if interfaceName != Interface {
		return newDBusError(dbusErrorUnknownInterface, "unknown interface %q", interfaceName)
	}
	if propertyName != "State" {
		return newDBusError(dbusErrorUnknownProperty, "unknown property %q", propertyName)
	}
	return newDBusError(dbusErrorPropertyReadOnly, "property %q is read-only", propertyName)
}

func summarizeSettings(settings ConnectionSettings) string {
	if len(settings) == 0 {
		return "{}"
	}

	sections := make([]string, 0, len(settings))
	for section, values := range settings {
		keys := make([]string, 0, len(values))
		for key := range values {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		sections = append(sections, fmt.Sprintf("%s[%s]", section, strings.Join(keys, ",")))
	}
	sort.Strings(sections)
	return strings.Join(sections, "; ")
}

func summarizeVariantMap(values VariantMap) string {
	if len(values) == 0 {
		return "{}"
	}

	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return "{" + strings.Join(keys, ",") + "}"
}

func newDBusError(name string, format string, args ...interface{}) *dbus.Error {
	return dbus.NewError(name, []interface{}{fmt.Sprintf(format, args...)})
}
