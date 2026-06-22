package nmplugin

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strings"

	"github.com/godbus/dbus/v5"
	"github.com/netbirdio/network-manager-plugin/internal/netbird/daemonclient"
)

const (
	nmVPNConfigGateway = "gateway"
	nmVPNConfigTUNDev  = "tundev"
	nmVPNConfigHasIP4  = "has-ip4"
	nmVPNConfigHasIP6  = "has-ip6"
)

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
		return dbus.MakeVariant(binary.BigEndian.Uint32(bytes[:])), true
	}
	if addr.Is6() {
		bytes := addr.As16()
		return dbus.MakeVariant(append([]byte(nil), bytes[:]...)), true
	}
	return dbus.Variant{}, false
}
