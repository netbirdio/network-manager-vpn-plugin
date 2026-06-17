module github.com/netbirdio/network-manager-plugin

go 1.26.2

require (
	github.com/godbus/dbus/v5 v5.2.2
	google.golang.org/grpc v1.80.0
	google.golang.org/protobuf v1.36.11
)

require go.uber.org/goleak v1.3.0

require github.com/netbirdio/netbird v0.72.4 // indirect

require (
	github.com/go-openapi/testify/v2 v2.5.0
	golang.org/x/net v0.53.0 // indirect
	golang.org/x/sys v0.43.0 // indirect
	golang.org/x/text v0.36.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260401024825-9d38bb4040a9 // indirect
)
