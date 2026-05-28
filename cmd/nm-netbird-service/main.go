// Command nm-netbird-service exposes the NetBird NetworkManager VPN plugin service over D-Bus.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/netbirdio/network-manager-plugin/internal/envconfig"
	"github.com/netbirdio/network-manager-plugin/internal/initsystem"
	"github.com/netbirdio/network-manager-plugin/internal/netbird/daemonclient"
	"github.com/netbirdio/network-manager-plugin/internal/nmplugin"
)

func main() {
	defaultDaemonOptions := daemonclient.DefaultOptionsFromEnv()

	busType := flag.String("bus", "system", "D-Bus bus to use: system or session")
	debug := flag.Bool("debug", false, "enable verbose logging")
	daemonAddress := flag.String("daemon-address", defaultDaemonOptions.Address, "NetBird daemon gRPC address")
	startDaemon := flag.Bool("start-daemon", defaultDaemonOptions.StartDaemon, "ask the configured init system to start the NetBird daemon if the first dial fails")
	daemonInitSystem := flag.String("daemon-init-system", envconfig.StringDefault(initsystem.EnvSystem, initsystem.DefaultSystem), "init system used with --start-daemon: auto or systemd")
	daemonService := flag.String("daemon-service", defaultDaemonOptions.DaemonService, "daemon service name to start when --start-daemon is enabled")
	dialTimeout := flag.Duration("daemon-dial-timeout", defaultDaemonOptions.DialTimeout, "timeout for dialing the NetBird daemon")
	rpcTimeout := flag.Duration("daemon-rpc-timeout", defaultDaemonOptions.RPCTimeout, "per-RPC timeout for NetBird daemon calls without an existing deadline")
	activationTimeout := flag.Duration("activation-timeout", 90*time.Second, "maximum time to wait for NetBird activation phases other than interactive SSO")
	ssoWaitTimeout := flag.Duration("sso-wait-timeout", 10*time.Minute, "maximum time to wait for interactive NetBird SSO completion")
	flag.Parse()

	logger := log.New(os.Stdout, "nm-netbird-service: ", log.LstdFlags|log.Lmicroseconds)

	conn, err := connectBus(*busType)
	if err != nil {
		logger.Fatalf("connect %s bus: %v", *busType, err)
	}
	defer func() {
		_ = conn.Close()
	}()

	daemonOptions := daemonclient.Options{
		Address:       *daemonAddress,
		DialTimeout:   *dialTimeout,
		RPCTimeout:    *rpcTimeout,
		StartDaemon:   *startDaemon,
		DaemonService: *daemonService,
		Logger:        logger,
	}
	if *startDaemon {
		starter, err := initsystem.NewStarter(*daemonInitSystem)
		if err != nil {
			logger.Fatalf("configure daemon autostart: %v", err)
		}
		daemonOptions.Starter = starter
	}

	service := nmplugin.NewService(conn, logger, *debug, nmplugin.ServiceOptions{
		ClientFactory:     daemonclient.NewFactory(daemonOptions),
		ActivationTimeout: *activationTimeout,
		SSOWaitTimeout:    *ssoWaitTimeout,
	})
	if err := service.Export(); err != nil {
		logger.Fatalf("export service: %v", err)
	}

	reply, err := conn.RequestName(nmplugin.BusName, dbus.NameFlagDoNotQueue)
	if err != nil {
		logger.Fatalf("request bus name %s: %v", nmplugin.BusName, err)
	}
	if reply != dbus.RequestNameReplyPrimaryOwner {
		logger.Fatalf("bus name %s is already owned (reply=%d)", nmplugin.BusName, reply)
	}
	defer func() {
		_, _ = conn.ReleaseName(nmplugin.BusName)
	}()

	logger.Printf("serving %s on %s bus at %s", nmplugin.BusName, normalizedBusName(*busType), nmplugin.ObjectPath)

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	<-signals

	logger.Printf("shutting down")
}

func connectBus(busType string) (*dbus.Conn, error) {
	switch normalizedBusName(busType) {
	case "session":
		return dbus.SessionBus()
	case "system":
		return dbus.SystemBus()
	default:
		return nil, fmt.Errorf("unsupported bus %q; expected session or system", busType)
	}
}

func normalizedBusName(busType string) string {
	return strings.ToLower(strings.TrimSpace(busType))
}
