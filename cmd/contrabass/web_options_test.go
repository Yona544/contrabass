package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/junhoyeo/contrabass/internal/config"
)

func TestResolveWebOptionsDisabledWithoutPortOrListen(t *testing.T) {
	opts, err := resolveWebOptions(&config.WorkflowConfig{}, "", 0)
	require.NoError(t, err)
	assert.False(t, opts.Enabled)
}

func TestResolveWebOptionsPortDerivesLoopbackAddr(t *testing.T) {
	opts, err := resolveWebOptions(&config.WorkflowConfig{}, "", 8080)
	require.NoError(t, err)
	assert.True(t, opts.Enabled)
	assert.Equal(t, "localhost:8080", opts.ListenAddr)
	assert.Empty(t, opts.AuthToken)
}

func TestResolveWebOptionsFlagOverridesConfig(t *testing.T) {
	cfg := &config.WorkflowConfig{Web: config.WebConfig{Listen: "127.0.0.1:9000"}}
	opts, err := resolveWebOptions(cfg, "127.0.0.1:9999", 8080)
	require.NoError(t, err)
	assert.Equal(t, "127.0.0.1:9999", opts.ListenAddr)
}

func TestResolveWebOptionsNonLoopbackRequiresToken(t *testing.T) {
	_, err := resolveWebOptions(&config.WorkflowConfig{}, "0.0.0.0:8080", 0)
	require.Error(t, err)
	assert.ErrorContains(t, err, "non-loopback")

	cfg := &config.WorkflowConfig{Web: config.WebConfig{AuthToken: "team-secret"}}
	opts, err := resolveWebOptions(cfg, "0.0.0.0:8080", 0)
	require.NoError(t, err)
	assert.Equal(t, "team-secret", opts.AuthToken)
	assert.Contains(t, opts.dashboardURL(), "?token=team-secret")
}

func TestResolveWebOptionsRejectsBarePortListen(t *testing.T) {
	_, err := resolveWebOptions(&config.WorkflowConfig{}, ":8080", 0)
	require.Error(t, err)
	assert.ErrorContains(t, err, "ambiguous listen address")
}

func TestWebAuthTokenPrefersEnvironment(t *testing.T) {
	t.Setenv("CONTRABASS_DASHBOARD_TOKEN", "env-secret")
	cfg := &config.WorkflowConfig{Web: config.WebConfig{AuthToken: "file-secret"}}
	assert.Equal(t, "env-secret", webAuthToken(cfg))

	t.Setenv("CONTRABASS_DASHBOARD_TOKEN", "")
	assert.Equal(t, "file-secret", webAuthToken(cfg))
}
