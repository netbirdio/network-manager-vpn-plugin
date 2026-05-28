package initsystem

import (
	"context"
	"testing"

	"github.com/go-openapi/testify/v2/require"
)

func TestNewStarter(t *testing.T) {
	_, err := NewStarter("auto")
	require.NoError(t, err)

	_, err = NewStarter("systemd")
	require.NoError(t, err)

	_, err = NewStarter("unsupported")
	require.Error(t, err)
}

func TestSystemdStarterRequiresServiceName(t *testing.T) {
	err := (SystemdStarter{}).Start(context.Background(), " ")
	require.Error(t, err)
}
