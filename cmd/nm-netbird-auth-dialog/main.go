// Command nm-netbird-auth-dialog handles NetworkManager VPN auth-dialog requests for NetBird.
package main

import (
	"os"

	"github.com/netbirdio/network-manager-plugin/internal/nmauthdialog"
)

func main() {
	os.Exit(nmauthdialog.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
