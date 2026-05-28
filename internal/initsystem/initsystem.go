// Package initsystem provides init-system abstractions for optional daemon autostart.
package initsystem

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

const (
	// DefaultSystem lets the plugin pick the first supported init-system backend.
	DefaultSystem = "auto"

	// EnvSystem is the environment variable overriding the init system used for
	// daemon autostart.
	EnvSystem = "NM_NETBIRD_DAEMON_INIT_SYSTEM"
)

// Starter starts a service through an init system.
type Starter interface {
	Start(ctx context.Context, serviceName string) error
}

// NewStarter returns a starter for name. Supported values are "auto" and
// "systemd"; additional init systems can be added here without changing the
// daemon client.
func NewStarter(name string) (Starter, error) {
	switch normalize(name) {
	case "", "auto":
		return AutoStarter{}, nil
	case "systemd":
		return SystemdStarter{}, nil
	default:
		return nil, fmt.Errorf("unsupported init system %q", name)
	}
}

// AutoStarter detects a supported init system when Start is called.
type AutoStarter struct{}

func (AutoStarter) Start(ctx context.Context, serviceName string) error {
	starter, err := detectStarter()
	if err != nil {
		return err
	}
	return starter.Start(ctx, serviceName)
}

// SystemdStarter starts services with systemctl.
type SystemdStarter struct {
	SystemctlPath string
}

func (s SystemdStarter) Start(ctx context.Context, serviceName string) error {
	serviceName = strings.TrimSpace(serviceName)
	if serviceName == "" {
		return fmt.Errorf("service name is empty")
	}

	systemctl := strings.TrimSpace(s.SystemctlPath)
	if systemctl == "" {
		systemctl = "systemctl"
	}

	cmd := exec.CommandContext(ctx, systemctl, "start", serviceName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		text := strings.TrimSpace(string(output))
		if text == "" {
			return fmt.Errorf("systemd start %s: %w", serviceName, err)
		}
		return fmt.Errorf("systemd start %s: %w: %s", serviceName, err, text)
	}
	return nil
}

func detectStarter() (Starter, error) {
	if _, err := exec.LookPath("systemctl"); err == nil {
		return SystemdStarter{}, nil
	}
	return nil, fmt.Errorf("no supported init system found for daemon autostart")
}

func normalize(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
