package cli

import (
	"slices"
	"strconv"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/aibridge"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

// aibridgeProviderEnvPrefix is the environment variable prefix for indexed
// AI Bridge provider configuration.
const aibridgeProviderEnvPrefix = "CODER_AIBRIDGE_PROVIDER_"

// ReadAIBridgeProvidersFromEnv parses CODER_AIBRIDGE_PROVIDER_<N>_<KEY>
// environment variables into a slice of AIBridgeProviderConfig.
// This follows the same indexed pattern as ReadExternalAuthProvidersFromEnv.
func ReadAIBridgeProvidersFromEnv(environ []string) ([]codersdk.AIBridgeProviderConfig, error) {
	// The index numbers must be in-order.
	slices.Sort(environ)

	var providers []codersdk.AIBridgeProviderConfig
	for _, v := range serpent.ParseEnviron(environ, aibridgeProviderEnvPrefix) {
		tokens := strings.SplitN(v.Name, "_", 2)
		if len(tokens) != 2 {
			return nil, xerrors.Errorf("invalid env var: %s", v.Name)
		}

		providerNum, err := strconv.Atoi(tokens[0])
		if err != nil {
			return nil, xerrors.Errorf("parse number: %s", v.Name)
		}

		var provider codersdk.AIBridgeProviderConfig
		switch {
		case len(providers) < providerNum:
			return nil, xerrors.Errorf(
				"provider num %v skipped: %s",
				len(providers),
				v.Name,
			)
		case len(providers) == providerNum:
			providers = append(providers, provider)
		case len(providers) == providerNum+1:
			provider = providers[providerNum]
		}

		key := tokens[1]
		switch key {
		case "TYPE":
			provider.Type = v.Value
		case "NAME":
			provider.Name = v.Value
		case "KEY":
			provider.Key = v.Value
		case "BASE_URL":
			provider.BaseURL = v.Value
		case "BEDROCK_BASE_URL":
			provider.BedrockBaseURL = v.Value
		case "BEDROCK_REGION":
			provider.BedrockRegion = v.Value
		case "BEDROCK_ACCESS_KEY":
			provider.BedrockAccessKey = v.Value
		case "BEDROCK_ACCESS_KEY_SECRET":
			provider.BedrockAccessKeySecret = v.Value
		case "BEDROCK_MODEL":
			provider.BedrockModel = v.Value
		case "BEDROCK_SMALL_FAST_MODEL":
			provider.BedrockSmallFastModel = v.Value
		}
		providers[providerNum] = provider
	}

	// Post-parse validation.
	names := make(map[string]struct{}, len(providers))
	for i := range providers {
		p := &providers[i]
		if p.Type == "" {
			return nil, xerrors.Errorf("provider %d: TYPE is required", i)
		}
		switch p.Type {
		case aibridge.ProviderOpenAI, aibridge.ProviderAnthropic, aibridge.ProviderCopilot:
		default:
			return nil, xerrors.Errorf("provider %d: unknown TYPE %q (must be %s, %s, or %s)",
				i, p.Type, aibridge.ProviderOpenAI, aibridge.ProviderAnthropic, aibridge.ProviderCopilot)
		}
		if p.Name == "" {
			p.Name = p.Type
		}
		if _, exists := names[p.Name]; exists {
			return nil, xerrors.Errorf("provider %d: duplicate NAME %q (multiple providers of the same type require explicit NAME fields)", i, p.Name)
		}
		names[p.Name] = struct{}{}
	}

	return providers, nil
}
