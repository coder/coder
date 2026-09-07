package aibridgetest

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge"
)

// NewAnthropicProvider builds an Anthropic provider for tests, failing the test
// if credential resolution fails.
func NewAnthropicProvider(t testing.TB, cfg aibridge.AnthropicConfig, bedrockCfg *aibridge.AWSBedrockConfig) aibridge.Provider {
	t.Helper()
	p, err := aibridge.NewAnthropicProvider(context.Background(), cfg, bedrockCfg, nil)
	require.NoError(t, err)
	return p
}

// NewClaudePlatformProvider builds an Anthropic provider configured for Claude
// Platform for AWS, failing the test if credential resolution fails.
func NewClaudePlatformProvider(t testing.TB, cfg aibridge.AnthropicConfig, claudePlatformCfg *aibridge.AWSClaudePlatformConfig) aibridge.Provider {
	t.Helper()
	p, err := aibridge.NewAnthropicProvider(context.Background(), cfg, nil, claudePlatformCfg)
	require.NoError(t, err)
	return p
}
