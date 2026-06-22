package nmplugin

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/netbirdio/network-manager-plugin/internal/netbird/daemonclient"
)

const (
	setupKeyRequiredMessage = "NetBird setup key required."
	ssoRequiredMessage      = "NetBird SSO login required."
)

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

func (s *Service) startSSOPrompt(activationID uint64, response daemonclient.LoginResponse) {
	prompt := &activationPrompt{
		activationID: activationID,
		kind:         activationPromptSSO,
	}
	if !s.registerActivationPrompt(prompt) {
		return
	}
	if err := s.EmitSecretsRequired(ssoRequiredMessage, ssoPromptHints(activationID, response)); err != nil {
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
	return current
}

func setupKeyPromptHints(activationID uint64) []string {
	return []string{
		"setup-key",
		formatPromptHint(netbirdPromptActivationID, formatActivationID(activationID)),
	}
}

func ssoPromptHints(activationID uint64, response daemonclient.LoginResponse) []string {
	hints := []string{
		formatPromptHint(netbirdPromptActivationID, formatActivationID(activationID)),
		formatPromptHint(netbirdSSOHint, "true"),
		netbirdSSOContinue,
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
	return hints
}

func formatPromptHint(key string, value string) string {
	return key + "=" + value
}

func formatActivationID(id uint64) string {
	return strconv.FormatUint(id, 10)
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
