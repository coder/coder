package chatopenai

import (
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
)

// ReasoningMode returns the explicit OpenAI reasoning mode in config, or nil.
func ReasoningMode(config *codersdk.ChatModelCallConfig) *string {
	if config == nil || config.ProviderOptions == nil || config.ProviderOptions.OpenAI == nil {
		return nil
	}
	return config.ProviderOptions.OpenAI.ReasoningMode
}

// ValidateReasoningMode checks an explicit mode against the provider and the
// OpenAI overrides it cannot coexist with. Model support is not checked here:
// OpenAI rejects the request when the model lacks the mode, and a model
// released after this code still works.
func ValidateReasoningMode(provider string, config *codersdk.ChatModelCallConfig) error {
	mode := ReasoningMode(config)
	if mode == nil {
		return nil
	}
	if *mode != "pro" {
		return xerrors.New("provider_options.openai.reasoning_mode must be pro")
	}
	if provider != "openai" {
		return xerrors.New("provider_options.openai.reasoning_mode requires an OpenAI provider")
	}
	if openAIConfig := config.OpenAIConfig; openAIConfig != nil {
		if openAIConfig.ReasoningModel != nil && !*openAIConfig.ReasoningModel {
			return xerrors.New("provider_options.openai.reasoning_mode requires a reasoning model")
		}
		if openAIConfig.UseResponsesAPI != nil && !*openAIConfig.UseResponsesAPI {
			return xerrors.New("provider_options.openai.reasoning_mode requires the Responses API")
		}
	}
	return nil
}
