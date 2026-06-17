package status_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/require"
	"github.com/netbirdio/netbird/client/proto"
	nbstatus "github.com/netbirdio/network-manager-plugin/internal/netbird/status"
)

func TestMapStatusStrings(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want nbstatus.State
	}{
		{name: "connected", raw: "Connected", want: nbstatus.Connected},
		{name: "ready", raw: "ready", want: nbstatus.Connected},
		{name: "connecting", raw: "Connecting to management", want: nbstatus.Connecting},
		{name: "not connected", raw: "Not Connected", want: nbstatus.Disconnected},
		{name: "login required", raw: "login_required", want: nbstatus.Disconnected},
		{name: "failure", raw: "authentication failed", want: nbstatus.Failed},
		{name: "setup required is not up", raw: "setup key required", want: nbstatus.Unknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := nbstatus.Map(&proto.StatusResponse{Status: tt.raw})
			require.Equal(t, tt.want, got.State)
		})
	}
}

func TestMapFullStatusFallback(t *testing.T) {
	got := nbstatus.Map(&proto.StatusResponse{FullStatus: &proto.FullStatus{
		ManagementState: &proto.ManagementState{Connected: true},
		SignalState:     &proto.SignalState{Connected: true},
		LocalPeerState:  &proto.LocalPeerState{IP: "100.64.0.1"},
	}})
	require.Equal(t, nbstatus.Connected, got.State)
}

func TestMapFullStatusFailure(t *testing.T) {
	got := nbstatus.Map(&proto.StatusResponse{
		Status: "connecting",
		FullStatus: &proto.FullStatus{
			ManagementState: &proto.ManagementState{Error: "no route to host"},
		},
	})
	require.Equal(t, nbstatus.Failed, got.State)
	require.Contains(t, got.Message, "no route")
}
