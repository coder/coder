package aibridgetest

import (
	"context"

	"github.com/coder/coder/v2/aibridge"
)

// MustNewAnthropicProvider builds an Anthropic provider for tests, panicking if
// credential resolution fails.
func MustNewAnthropicProvider(cfg aibridge.AnthropicConfig, bedrockCfg *aibridge.AWSBedrockConfig) aibridge.Provider {
	p, err := aibridge.NewAnthropicProvider(context.Background(), cfg, bedrockCfg)
	if err != nil {
		panic("build anthropic provider: " + err.Error())
	}
	return p
}
