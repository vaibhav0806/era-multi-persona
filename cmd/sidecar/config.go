package main

import (
	"errors"
	"os"
)

type sidecarConfig struct {
	ListenAddr string
}

func loadSidecarConfig() (*sidecarConfig, error) {
	c := &sidecarConfig{ListenAddr: os.Getenv("PI_SIDECAR_LISTEN_ADDR")}
	if c.ListenAddr == "" {
		return nil, errors.New("PI_SIDECAR_LISTEN_ADDR is required")
	}
	return c, nil
}
