package chatopenai

import (
	"regexp"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
)

// gpt-daybreak-blue-latest is OpenAI's alias for gpt-5.6-sol.
var reasoningModeModels = regexp.MustCompile(`^((gpt-5\.6(-(sol|terra|luna))?|gpt-6-astra)(-[0-9]{4}-[0-9]{2}-[0-9]{2})?|gpt-daybreak-blue-latest)$`)

// ValidateReasoningMode checks explicit modes against the provider, model,
// and transport.
func ValidateReasoningMode(provider, modelID string, config *codersdk.ChatModelCallConfig) error {
	if config == nil || config.ProviderOptions == nil || config.ProviderOptions.OpenAI == nil || config.ProviderOptions.OpenAI.ReasoningMode == nil {
		return nil
	}
	mode := *config.ProviderOptions.OpenAI.ReasoningMode
	if mode != "standard" && mode != "pro" {
		return xerrors.New("provider_options.openai.reasoning_mode must be one of standard, pro")
	}
	if provider != "openai" || !reasoningModeModels.MatchString(strings.TrimSpace(modelID)) {
		return xerrors.New("provider_options.openai.reasoning_mode requires a supported OpenAI GPT-5.6 or GPT-6 Astra model")
	}
	var override *bool
	if config.OpenAIConfig != nil {
		override = config.OpenAIConfig.UseResponsesAPI
	}
	if !UsesResponsesAPI(modelID, override) {
		return xerrors.New("provider_options.openai.reasoning_mode requires the Responses API")
	}
	return nil
}
