// Package nmplugin implements the NetworkManager VPN plugin D-Bus service for NetBird.
package nmplugin

import (
	"context"
	"errors"
	"fmt"
	"log"
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
	defaultActivationTimeout   = 90 * time.Second
	defaultSSOWaitTimeout      = 10 * time.Minute
	defaultOperationTimeout    = 20 * time.Second
	defaultReadyPollInterval   = 500 * time.Millisecond
	defaultStatusPollInterval  = 5 * time.Second
	defaultStatusCallTimeout   = 5 * time.Second
	statusPollFailureThreshold = 3
)

// ServiceOptions configures daemon integration behavior.
// Non-positive duration values are replaced with defaults; timeouts cannot be disabled.
type ServiceOptions struct {
	ClientFactory          daemonclient.Factory
	ActivationTimeout      time.Duration
	SSOWaitTimeout         time.Duration
	OperationTimeout       time.Duration
	ReadyPollInterval      time.Duration
	StatusPollInterval     time.Duration
	StatusCallTimeout      time.Duration
	BeforeThresholdFailure func()
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

	clientFactory          daemonclient.Factory
	activationTimeout      time.Duration
	ssoWaitTimeout         time.Duration
	operationTimeout       time.Duration
	readyPollInterval      time.Duration
	statusPollInterval     time.Duration
	statusCallTimeout      time.Duration
	beforeThresholdFailure func()

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

// NewService creates an initialized VPN plugin service. Call Export before
// requesting the well-known bus name.
func NewService(conn *dbus.Conn, logger *log.Logger, debug bool, options ServiceOptions) *Service {
	if logger == nil {
		logger = log.Default()
	}
	options = normalizeServiceOptions(options, logger)

	return &Service{
		conn:                   conn,
		path:                   ObjectPath,
		logger:                 logger,
		debug:                  debug,
		state:                  ServiceStateInit,
		clientFactory:          options.ClientFactory,
		activationTimeout:      options.ActivationTimeout,
		ssoWaitTimeout:         options.SSOWaitTimeout,
		operationTimeout:       options.OperationTimeout,
		readyPollInterval:      options.ReadyPollInterval,
		statusPollInterval:     options.StatusPollInterval,
		statusCallTimeout:      options.StatusCallTimeout,
		beforeThresholdFailure: options.BeforeThresholdFailure,
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
	if activationSettings.needsSetupKeySecret() {
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

// Disconnect is called by NetworkManager to stop the VPN service. NetBird's
// daemon proto has a global DownRequest, so this intentionally disconnects the
// single daemon engine rather than a profile-scoped instance.
func (s *Service) Disconnect() *dbus.Error {
	s.logf("Disconnect()")

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

	s.setState(ServiceStateStopping)

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
	go s.runActivation(activationAttempt{
		ctx:         activationCtx,
		cancel:      activationCancel,
		id:          activationID,
		connection:  connection,
		details:     details,
		interactive: interactive,
	})
	return nil
}

type activationAttempt struct {
	ctx         context.Context
	cancel      context.CancelFunc
	id          uint64
	connection  ConnectionSettings
	details     VariantMap
	interactive bool
}

type activationStatus struct {
	success  bool
	daemonUp bool
}

func (s *Service) runActivation(
	activationAttempt activationAttempt,
) {
	ctx, timeoutCancel := timeoutCtxWithDefault(activationAttempt.ctx, s.activationTimeout, defaultActivationTimeout)

	var client daemonclient.Client

	status := new(activationStatus)

	defer func() {
		s.finishActivationAttempt(activationAttempt, status, client, timeoutCancel)
	}()

	settings := parseActivationSettings(activationAttempt.connection).mergeDetails(activationAttempt.details)

	if !settings.isAuthModeValid() {
		s.failActivation(PluginFailureLoginFailed, fmt.Errorf("unsupported auth mode %q (must be setup-key or sso)", settings.AuthMode))
		return
	}

	if settings.AuthMode == "sso" && !activationAttempt.interactive {
		s.failActivation(PluginFailureLoginFailed, errInteractiveSSONeeded)
		return
	}

	var err error
	client, err = s.clientFactory.NewClient(ctx)
	if err != nil {
		s.failActivation(PluginFailureConnectFailed, fmt.Errorf("connect to netbird daemon: %w", err))
		return
	}

	settings, err = s.prepareDaemonProfile(ctx, client, settings)
	if err != nil {
		s.failActivation(PluginFailureConnectFailed, err)
		return
	}

	waitedForSSO, err := s.authenticate(ctx, activationAttempt, client, settings)
	if err != nil {
		s.failActivation(PluginFailureLoginFailed, err)
		return
	}

	// restart the activation phase timeout if we waited for SSO, since the SSO wait can be longer than the activation phase timeout.
	if waitedForSSO {
		timeoutCancel()
		ctx, timeoutCancel = timeoutCtxWithDefault(activationAttempt.ctx, s.activationTimeout, defaultActivationTimeout)
	}

	if err := s.updateDaemonProfile(ctx, client, settings); err != nil {
		s.failActivation(PluginFailureConnectFailed, err)
		return
	}

	resolvedProfile, err := profile.Resolve(ctx, client, settings.Profile)
	if err != nil {
		s.failActivation(PluginFailureConnectFailed, err)
		return
	}

	if err := client.Up(ctx, resolvedProfile); err != nil {
		s.failActivation(classifyUpFailure(err), err)
		return
	}
	status.daemonUp = true

	if err := s.waitForReady(ctx, client); err != nil {
		s.failActivation(PluginFailureConnectFailed, err)
		return
	}

	if err := s.emitNetworkManagerConfig(ctx, client, resolvedProfile, settings); err != nil {
		s.failActivation(PluginFailureBadIPConfig, err)
		return
	}

	sessionID, completed := s.completeActivation(activationAttempt, client, resolvedProfile)
	if !completed {
		return
	}
	status.success = true
	s.setStateForActiveSession(sessionID, ServiceStateStarted)
}

func (s *Service) finishActivationAttempt(
	activationAttempt activationAttempt,
	status *activationStatus,
	client daemonclient.Client,
	timeoutCancel context.CancelFunc,
) {
	timeoutCancel()
	s.clearPromptForActivation(activationAttempt.id)
	if !status.success && status.daemonUp && client != nil {
		downCtx, downCancel := context.WithTimeout(context.Background(), s.operationTimeout)
		if err := client.Down(downCtx); err != nil {
			s.logger.Printf("netbird down after failed activation: %v", err)
		}
		downCancel()
	}

	activationAttempt.cancel()
	if status.success {
		return
	}
	s.finishFailedActivation()
	if client != nil {
		_ = client.Close()
	}
}

func (s *Service) prepareDaemonProfile(ctx context.Context, client daemonclient.Client, settings activationSettings) (activationSettings, error) {
	if settings.Profile.ProfileName != "" && settings.Profile.Username == "" {
		settings.Profile.Username = currentProcessUsername()
	}
	preparedProfile, err := profile.PrepareActivation(ctx, client, settings.Profile)
	if err != nil {
		return settings, err
	}
	settings.Profile = preparedProfile
	settings.Profile, err = profile.EnsureExists(ctx, client, settings.Profile)
	if err != nil {
		return settings, err
	}
	settings.Profile, err = s.switchDaemonProfile(ctx, client, settings.Profile)
	if err != nil {
		return settings, err
	}
	return settings, nil
}

func (s *Service) switchDaemonProfile(ctx context.Context, client daemonclient.Client, profile daemonclient.ProfileRef) (daemonclient.ProfileRef, error) {
	if profile.Empty() {
		return profile, nil
	}
	resolved, err := client.SwitchProfile(ctx, profile)
	if err != nil {
		return daemonclient.ProfileRef{}, fmt.Errorf("switch daemon profile: %w", err)
	}
	return resolved, nil
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
	activationAttempt activationAttempt,
	client daemonclient.Client,
	settings activationSettings,
) (bool, error) {
	if !settings.shouldLogin(activationAttempt.interactive) {
		return false, nil
	}

	if settings.AuthMode == "setup-key" && strings.TrimSpace(settings.SetupKey) == "" {
		if !activationAttempt.interactive {
			return false, errMissingSetupKey
		}
		var err error
		settings, err = s.waitForSetupKeySecret(ctx, activationAttempt.id, settings)
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
	if !activationAttempt.interactive {
		return false, errInteractiveSSONeeded
	}

	if err := s.EmitLoginBanner(formatSSOLoginBanner(response)); err != nil {
		s.logger.Printf("emit SSO login banner failed: %v", err)
	}
	s.startSSOPrompt(activationAttempt.id, response)
	// The vendored LoginResponse does not expose a device-code expiry, so use the
	// configured SSO wait timeout instead of the shorter activation phase timeout.

	ssoCtx, cancel := timeoutCtxWithDefault(activationAttempt.ctx, s.ssoWaitTimeout, defaultSSOWaitTimeout)
	defer cancel()
	_, err = client.WaitSSOLogin(ssoCtx, daemonclient.WaitSSOLoginRequest{
		UserCode: response.UserCode,
		Hostname: settings.daemonLoginRequest().Hostname,
	})
	if err != nil {
		if ssoCtx.Err() != nil && activationAttempt.ctx.Err() == nil {
			return true, fmt.Errorf("timeout waiting for SSO login: %w", err)
		}
		return true, err
	}
	return true, nil
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

func (s *Service) logDaemonStatus(mapped nbstatus.Mapping) {
	if mapped.DaemonVersion == "" {
		return
	}

	s.logf("netbird daemon version %s status=%s", mapped.DaemonVersion, mapped.State)
}

// reserveActivation claims the service's single activation slot.
//
// NetworkManager expects Connect/ConnectInteractive to return quickly, so the
// real daemon work runs later in a goroutine. Reserving the slot before the
// caller returns prevents a second Connect from racing with the in-flight
// activation or with an already active daemon client.
//
// The returned context/cancel pair lets Disconnect, SetFailure, and cleanup
// paths stop the asynchronous activation. The activation ID is used to match
// interactive SecretsRequired/NewSecrets prompts with the activation that
// created them, so stale prompt responses can be ignored safely.
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

func (s *Service) completeActivation(activationAttempt activationAttempt, client daemonclient.Client, activeProfile daemonclient.ProfileRef) (uint64, bool) {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()

	if activationAttempt.ctx.Err() != nil || !s.activating {
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

	consecutiveFailures := 0
	for {
		if contextCancelled(ctx) {
			return
		}

		keepGoing, failed := s.pollDaemonStatus(ctx, client, sessionID)
		if !keepGoing {
			return
		}
		consecutiveFailures = nextConsecutiveStatusFailures(consecutiveFailures, failed)
		if failed && s.stopAfterStatusPollFailureThreshold(ctx, sessionID, consecutiveFailures) {
			return
		}

		if !waitForStatusPollInterval(ctx, ticker.C) {
			return
		}
	}
}

func (s *Service) stopAfterStatusPollFailureThreshold(ctx context.Context, sessionID uint64, consecutiveFailures int) bool {
	if consecutiveFailures < statusPollFailureThreshold {
		return false
	}
	if ctx.Err() != nil {
		return true
	}
	if s.beforeThresholdFailure != nil {
		s.beforeThresholdFailure()
	}
	_ = s.failActiveSessionAfterStatusThreshold(sessionID, consecutiveFailures)
	return true
}

func (s *Service) pollDaemonStatus(ctx context.Context, client daemonclient.Client, sessionID uint64) (bool, bool) {
	callTimeout := s.statusCallTimeout
	if callTimeout <= 0 {
		callTimeout = defaultStatusCallTimeout
	}
	callCtx, cancel := context.WithTimeout(ctx, callTimeout)
	resp, err := client.Status(callCtx, daemonclient.StatusOptions{GetFullPeerStatus: true})
	cancel()
	if err != nil {
		if ctx.Err() != nil {
			return false, false
		}
		s.logger.Printf("netbird status poll failed: %v", err)
		return true, true
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
		return false, false
	case nbstatus.Failed:
		s.logger.Printf("netbird daemon reported failure: %s", mapped.Message)
		_ = s.EmitFailure(PluginFailureConnectFailed)
		s.setState(ServiceStateStopped)
		s.clearActiveSession(sessionID, true)
		return false, false
	}
	return true, false
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

func (s *Service) emit(signal string, args ...any) error {
	name := Interface + "." + signal
	if s.debug {
		s.logger.Printf("emit %s", name)
	}
	return s.conn.Emit(s.path, name, args...)
}
func (s *Service) failActiveSessionAfterStatusThreshold(sessionID uint64, consecutiveFailures int) bool {
	var client daemonclient.Client
	var monitorCancel context.CancelFunc
	var changed bool

	s.lifecycleMu.Lock()
	if s.sessionID != sessionID || s.client == nil {
		s.lifecycleMu.Unlock()
		return false
	}

	monitorCancel = s.monitorCancel
	s.monitorCancel = nil
	client = s.client
	s.client = nil
	s.activeProfile = daemonclient.ProfileRef{}
	changed = s.setStateValue(ServiceStateStopped)
	s.lifecycleMu.Unlock()

	if monitorCancel != nil {
		monitorCancel()
	}
	s.logger.Printf("netbird status poll failed %d consecutive times; failing active session", consecutiveFailures)
	_ = s.EmitFailure(PluginFailureConnectFailed)
	if changed {
		s.emitStateChanged(ServiceStateStopped)
	}

	if client != nil {
		if err := client.Close(); err != nil {
			s.logger.Printf("close netbird daemon client: %v", err)
		}
	}
	return true
}

func (s *Service) failActivation(failure PluginFailure, err error) {
	s.logger.Printf("netbird activation failed: %v", err)
	if errors.Is(err, errInteractiveSSONeeded) {
		_ = s.EmitLoginBanner(interactiveSSORequiredMessage)
	}
	_ = s.EmitFailure(failure)
	s.setState(ServiceStateStopped)
}
