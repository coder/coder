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
	p, err := aibridge.NewAnthropicProvider(context.Background(), cfg, bedrockCfg)
	require.NoError(t, err)
	return p
}
