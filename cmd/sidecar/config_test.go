package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSidecarConfig_Defaults(t *testing.T) {
	t.Setenv("PI_SIDECAR_LISTEN_ADDR", "127.0.0.1:8080")
	c, err := loadSidecarConfig()
	require.NoError(t, err)
	require.Equal(t, "127.0.0.1:8080", c.ListenAddr)
}

func TestSidecarConfig_MissingListenAddr(t *testing.T) {
	t.Setenv("PI_SIDECAR_LISTEN_ADDR", "")
	_, err := loadSidecarConfig()
	require.ErrorContains(t, err, "PI_SIDECAR_LISTEN_ADDR")
}
