package chatprovider

import (
	"context"
	"sort"
	"strings"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	fantasyazure "charm.land/fantasy/providers/azure"
	fantasybedrock "charm.land/fantasy/providers/bedrock"
	fantasygoogle "charm.land/fantasy/providers/google"
	fantasyopenai "charm.land/fantasy/providers/openai"
	fantasyopenaicompat "charm.land/fantasy/providers/openaicompat"
	fantasyopenrouter "charm.land/fantasy/providers/openrouter"
	fantasyvercel "charm.land/fantasy/providers/vercel"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
)

var supportedProviderNames = []string{
	fantasyanthropic.Name,
	fantasyazure.Name,
	fantasybedrock.Name,
	fantasygoogle.Name,
	fantasyopenai.Name,
	fantasyopenaicompat.Name,
	fantasyopenrouter.Name,
	fantasyvercel.Name,
}

var envPresetProviderNames = []string{
	fantasyopenai.Name,
	fantasyanthropic.Name,
}

var providerDisplayNameByName = map[string]string{
	fantasyanthropic.Name:    "Anthropic",
	fantasyazure.Name:        "Azure OpenAI",
	fantasybedrock.Name:      "AWS Bedrock",
	fantasygoogle.Name:       "Google",
	fantasyopenai.Name:       "OpenAI",
	fantasyopenaicompat.Name: "OpenAI Compatible",
	fantasyopenrouter.Name:   "OpenRouter",
	fantasyvercel.Name:       "Vercel AI Gateway",
}

// SupportedProviders returns all chat providers supported by Fantasy.
func SupportedProviders() []string {
	return append([]string(nil), supportedProviderNames...)
}

// IsEnvPresetProvider reports whether provider supports env presets.
func IsEnvPresetProvider(provider string) bool {
	normalized := NormalizeProvider(provider)
	for _, candidate := range envPresetProviderNames {
		if candidate == normalized {
			return true
		}
	}
	return false
}

// ProviderDisplayName returns a default display name for a provider.
func ProviderDisplayName(provider string) string {
	normalized := NormalizeProvider(provider)
	if displayName, ok := providerDisplayNameByName[normalized]; ok {
		return displayName
	}
	return normalized
}

// ProviderAPIKeys contains API keys for provider calls.
type ProviderAPIKeys struct {
	OpenAI            string
	Anthropic         string
	ByProvider        map[string]string
	BaseURLByProvider map[string]string
}

// ConfiguredProvider is an enabled provider loaded from database config.
type ConfiguredProvider struct {
	Provider string
	APIKey   string
	BaseURL  string
}

// ConfiguredModel is an enabled model loaded from database config.
type ConfiguredModel struct {
	Provider    string
	Model       string
	DisplayName string
}

// APIKey returns the effective API key for a provider.
func (k ProviderAPIKeys) APIKey(provider string) string {
	normalized := NormalizeProvider(provider)
	if normalized == "" {
		return ""
	}

	if k.ByProvider != nil {
		if key := strings.TrimSpace(k.ByProvider[normalized]); key != "" {
			return key
		}
	}

	switch normalized {
	case fantasyopenai.Name:
		return strings.TrimSpace(k.OpenAI)
	case fantasyanthropic.Name:
		return strings.TrimSpace(k.Anthropic)
	default:
		return ""
	}
}

//nolint:revive // Intentional: apiKey is the unexported helper for APIKey.
func (k ProviderAPIKeys) apiKey(provider string) string {
	return k.APIKey(provider)
}

// BaseURL returns the configured base URL for a provider.
func (k ProviderAPIKeys) BaseURL(provider string) string {
	normalized := NormalizeProvider(provider)
	if normalized == "" || k.BaseURLByProvider == nil {
		return ""
	}
	return strings.TrimSpace(k.BaseURLByProvider[normalized])
}

// MergeProviderAPIKeys overlays configured provider keys over fallback keys.
func MergeProviderAPIKeys(fallback ProviderAPIKeys, providers []ConfiguredProvider) ProviderAPIKeys {
	merged := ProviderAPIKeys{
		OpenAI:            strings.TrimSpace(fallback.OpenAI),
		Anthropic:         strings.TrimSpace(fallback.Anthropic),
		ByProvider:        map[string]string{},
		BaseURLByProvider: map[string]string{},
	}
	for provider, apiKey := range fallback.ByProvider {
		normalizedProvider := NormalizeProvider(provider)
		if normalizedProvider == "" {
			continue
		}
		if key := strings.TrimSpace(apiKey); key != "" {
			merged.ByProvider[normalizedProvider] = key
		}
	}
	for provider, baseURL := range fallback.BaseURLByProvider {
		normalizedProvider := NormalizeProvider(provider)
		if normalizedProvider == "" {
			continue
		}
		if url := strings.TrimSpace(baseURL); url != "" {
			merged.BaseURLByProvider[normalizedProvider] = url
		}
	}

	if merged.OpenAI != "" {
		merged.ByProvider[fantasyopenai.Name] = merged.OpenAI
	}
	if merged.Anthropic != "" {
		merged.ByProvider[fantasyanthropic.Name] = merged.Anthropic
	}

	for _, provider := range providers {
		normalizedProvider := NormalizeProvider(provider.Provider)
		if normalizedProvider == "" {
			continue
		}

		if key := strings.TrimSpace(provider.APIKey); key != "" {
			merged.ByProvider[normalizedProvider] = key
		}
		if url := strings.TrimSpace(provider.BaseURL); url != "" {
			merged.BaseURLByProvider[normalizedProvider] = url
		}

		switch normalizedProvider {
		case fantasyopenai.Name:
			if key := strings.TrimSpace(provider.APIKey); key != "" {
				merged.OpenAI = key
			}
		case fantasyanthropic.Name:
			if key := strings.TrimSpace(provider.APIKey); key != "" {
				merged.Anthropic = key
			}
		}
	}

	return merged
}

type ModelCatalog struct {
	keys ProviderAPIKeys
}

func NewModelCatalog(keys ProviderAPIKeys) *ModelCatalog {
	return &ModelCatalog{
		keys: keys,
	}
}

// ListConfiguredModels returns a model catalog from enabled DB-backed model
// configs. The second return value reports whether DB-backed models were used.
func (c *ModelCatalog) ListConfiguredModels(
	configuredProviders []ConfiguredProvider,
	configuredModels []ConfiguredModel,
) (codersdk.ChatModelsResponse, bool) {
	if len(configuredModels) == 0 {
		return codersdk.ChatModelsResponse{}, false
	}

	modelsByProvider := make(map[string][]codersdk.ChatModel)
	seenByProvider := make(map[string]map[string]struct{})
	providerSet := make(map[string]struct{})

	for _, provider := range configuredProviders {
		normalized := normalizeProvider(provider.Provider)
		if normalized == "" {
			continue
		}
		providerSet[normalized] = struct{}{}
	}

	for _, model := range configuredModels {
		provider, modelID, err := ResolveModelWithProviderHint(model.Model, model.Provider)
		if err != nil {
			continue
		}

		providerSet[provider] = struct{}{}
		if seenByProvider[provider] == nil {
			seenByProvider[provider] = make(map[string]struct{})
		}
		normalizedModelID := strings.ToLower(strings.TrimSpace(modelID))
		if _, ok := seenByProvider[provider][normalizedModelID]; ok {
			continue
		}
		seenByProvider[provider][normalizedModelID] = struct{}{}
		modelsByProvider[provider] = append(
			modelsByProvider[provider],
			newChatModel(provider, modelID, model.DisplayName),
		)
	}

	providers := orderProviders(providerSet)
	if len(providers) == 0 {
		return codersdk.ChatModelsResponse{}, false
	}

	keys := MergeProviderAPIKeys(c.keys, configuredProviders)
	response := codersdk.ChatModelsResponse{
		Providers: make([]codersdk.ChatModelProvider, 0, len(providers)),
	}
	for _, provider := range providers {
		models := modelsByProvider[provider]
		sortChatModels(models)

		result := codersdk.ChatModelProvider{
			Provider: provider,
			Models:   models,
		}
		if keys.apiKey(provider) == "" {
			result.Available = false
			result.UnavailableReason = codersdk.ChatModelProviderUnavailableMissingAPIKey
		} else {
			result.Available = true
		}

		response.Providers = append(response.Providers, result)
	}

	return response, true
}

// ListConfiguredProviderAvailability returns provider availability derived from
// deployment/env keys merged with enabled DB provider keys.
func (c *ModelCatalog) ListConfiguredProviderAvailability(
	configuredProviders []ConfiguredProvider,
) codersdk.ChatModelsResponse {
	keys := MergeProviderAPIKeys(c.keys, configuredProviders)
	response := codersdk.ChatModelsResponse{
		Providers: make([]codersdk.ChatModelProvider, 0, len(supportedProviderNames)),
	}

	for _, provider := range supportedProviderNames {
		result := codersdk.ChatModelProvider{
			Provider: provider,
			Models:   []codersdk.ChatModel{},
		}
		if keys.apiKey(provider) == "" {
			result.Available = false
			result.UnavailableReason = codersdk.ChatModelProviderUnavailableMissingAPIKey
		} else {
			result.Available = true
		}

		response.Providers = append(response.Providers, result)
	}

	return response
}

func newChatModel(provider, modelID, displayName string) codersdk.ChatModel {
	name := strings.TrimSpace(displayName)
	if name == "" {
		name = modelID
	}

	return codersdk.ChatModel{
		ID:          canonicalModelID(provider, modelID),
		Provider:    provider,
		Model:       modelID,
		DisplayName: name,
	}
}

func sortChatModels(models []codersdk.ChatModel) {
	sort.Slice(models, func(i, j int) bool {
		return models[i].Model < models[j].Model
	})
}

func canonicalModelID(provider, modelID string) string {
	return NormalizeProvider(provider) + ":" + strings.TrimSpace(modelID)
}

func orderProviders(providerSet map[string]struct{}) []string {
	if len(providerSet) == 0 {
		return nil
	}

	ordered := make([]string, 0, len(providerSet))
	for _, provider := range supportedProviderNames {
		if _, ok := providerSet[provider]; ok {
			ordered = append(ordered, provider)
		}
	}

	// Unknown providers are dropped. The providerSet keys are
	// already normalized, so any provider not in
	// supportedProviderNames is silently excluded.
	return ordered
}

// NormalizeProvider canonicalizes a provider name.
func NormalizeProvider(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case fantasyanthropic.Name:
		return fantasyanthropic.Name
	case fantasyazure.Name:
		return fantasyazure.Name
	case fantasybedrock.Name:
		return fantasybedrock.Name
	case fantasygoogle.Name:
		return fantasygoogle.Name
	case fantasyopenai.Name:
		return fantasyopenai.Name
	case fantasyopenaicompat.Name:
		return fantasyopenaicompat.Name
	case fantasyopenrouter.Name:
		return fantasyopenrouter.Name
	case fantasyvercel.Name:
		return fantasyvercel.Name
	default:
		return ""
	}
}

//nolint:revive // Intentional: normalizeProvider is the unexported helper for NormalizeProvider.
func normalizeProvider(provider string) string {
	return NormalizeProvider(provider)
}

func ResolveModelWithProviderHint(modelName, providerHint string) (provider string, model string, err error) {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return "", "", xerrors.New("model is required")
	}

	if provider, modelID, ok := parseCanonicalModelRef(modelName); ok {
		return provider, modelID, nil
	}

	if provider := normalizeProvider(providerHint); provider != "" {
		return provider, modelName, nil
	}

	normalized := strings.ToLower(modelName)
	switch normalized {
	case "claude-opus-4-6":
		return fantasyanthropic.Name, "claude-opus-4-6", nil
	case "gpt-5.2":
		return fantasyopenai.Name, "gpt-5.2", nil
	case "gemini-2.5-flash":
		return fantasygoogle.Name, "gemini-2.5-flash", nil
	}

	if isChatModelForProvider(fantasyanthropic.Name, normalized) {
		return fantasyanthropic.Name, modelName, nil
	}
	if isChatModelForProvider(fantasyopenai.Name, normalized) {
		return fantasyopenai.Name, modelName, nil
	}

	return "", "", xerrors.Errorf("unknown model %q", modelName)
}

func parseCanonicalModelRef(modelRef string) (provider string, model string, ok bool) {
	modelRef = strings.TrimSpace(modelRef)
	if modelRef == "" {
		return "", "", false
	}

	for _, separator := range []string{":", "/"} {
		parts := strings.SplitN(modelRef, separator, 2)
		if len(parts) != 2 {
			continue
		}

		provider := normalizeProvider(parts[0])
		modelID := strings.TrimSpace(parts[1])
		if provider != "" && modelID != "" {
			return provider, modelID, true
		}
	}

	return "", "", false
}

func isChatModelForProvider(provider, modelID string) bool {
	normalizedProvider := normalizeProvider(provider)
	normalizedModel := strings.ToLower(strings.TrimSpace(modelID))
	switch normalizedProvider {
	case fantasyopenai.Name:
		return strings.HasPrefix(normalizedModel, "gpt-") ||
			strings.HasPrefix(normalizedModel, "chatgpt-") ||
			isOpenAIReasoningModel(normalizedModel)
	case fantasyanthropic.Name:
		return strings.HasPrefix(normalizedModel, "claude-")
	case fantasygoogle.Name:
		return strings.HasPrefix(normalizedModel, "gemini-") ||
			strings.HasPrefix(normalizedModel, "gemma-")
	default:
		return false
	}
}

func isOpenAIReasoningModel(modelID string) bool {
	if len(modelID) < 2 || modelID[0] != 'o' {
		return false
	}

	index := 1
	for index < len(modelID) && modelID[index] >= '0' && modelID[index] <= '9' {
		index++
	}
	if index == 1 {
		return false
	}

	if index == len(modelID) {
		return true
	}
	return modelID[index] == '-' || modelID[index] == '.'
}

// ReasoningEffortFromChat normalizes chat-config reasoning effort values for a
// provider and returns the canonical provider effort value.
func ReasoningEffortFromChat(provider string, value *string) *string {
	if value == nil {
		return nil
	}

	normalized := strings.ToLower(strings.TrimSpace(*value))
	if normalized == "" {
		return nil
	}

	switch NormalizeProvider(provider) {
	case fantasyopenai.Name:
		return normalizedEnumValue(
			normalized,
			string(fantasyopenai.ReasoningEffortMinimal),
			string(fantasyopenai.ReasoningEffortLow),
			string(fantasyopenai.ReasoningEffortMedium),
			string(fantasyopenai.ReasoningEffortHigh),
		)
	case fantasyanthropic.Name:
		return normalizedEnumValue(
			normalized,
			string(fantasyanthropic.EffortLow),
			string(fantasyanthropic.EffortMedium),
			string(fantasyanthropic.EffortHigh),
			string(fantasyanthropic.EffortMax),
		)
	case fantasyopenrouter.Name:
		return normalizedEnumValue(
			normalized,
			string(fantasyopenrouter.ReasoningEffortLow),
			string(fantasyopenrouter.ReasoningEffortMedium),
			string(fantasyopenrouter.ReasoningEffortHigh),
		)
	case fantasyvercel.Name:
		return normalizedEnumValue(
			normalized,
			string(fantasyvercel.ReasoningEffortNone),
			string(fantasyvercel.ReasoningEffortMinimal),
			string(fantasyvercel.ReasoningEffortLow),
			string(fantasyvercel.ReasoningEffortMedium),
			string(fantasyvercel.ReasoningEffortHigh),
			string(fantasyvercel.ReasoningEffortXHigh),
		)
	default:
		return nil
	}
}

// OpenAITextVerbosityFromChat normalizes chat-config text verbosity values for
// OpenAI and returns the canonical provider verbosity value.
func OpenAITextVerbosityFromChat(value *string) *fantasyopenai.TextVerbosity {
	if value == nil {
		return nil
	}

	normalized := strings.ToLower(strings.TrimSpace(*value))
	if normalized == "" {
		return nil
	}

	verbosity := normalizedEnumValue(
		normalized,
		string(fantasyopenai.TextVerbosityLow),
		string(fantasyopenai.TextVerbosityMedium),
		string(fantasyopenai.TextVerbosityHigh),
	)
	if verbosity == nil {
		return nil
	}
	valueCopy := fantasyopenai.TextVerbosity(*verbosity)
	return &valueCopy
}

func normalizedEnumValue(value string, allowed ...string) *string {
	for _, candidate := range allowed {
		if value == strings.ToLower(candidate) {
			match := candidate
			return &match
		}
	}
	return nil
}

// MergeMissingCallConfig fills unset call config values from defaults.
func MergeMissingCallConfig(
	dst *codersdk.ChatModelCallConfig,
	defaults codersdk.ChatModelCallConfig,
) {
	if dst.MaxOutputTokens == nil {
		dst.MaxOutputTokens = defaults.MaxOutputTokens
	}
	if dst.Temperature == nil {
		dst.Temperature = defaults.Temperature
	}
	if dst.TopP == nil {
		dst.TopP = defaults.TopP
	}
	if dst.TopK == nil {
		dst.TopK = defaults.TopK
	}
	if dst.PresencePenalty == nil {
		dst.PresencePenalty = defaults.PresencePenalty
	}
	if dst.FrequencyPenalty == nil {
		dst.FrequencyPenalty = defaults.FrequencyPenalty
	}
	MergeMissingProviderOptions(&dst.ProviderOptions, defaults.ProviderOptions)
}

// MergeMissingProviderOptions fills unset provider option fields from defaults.
func MergeMissingProviderOptions(
	dst **codersdk.ChatModelProviderOptions,
	defaults *codersdk.ChatModelProviderOptions,
) {
	if defaults == nil {
		return
	}
	if *dst == nil {
		copied := *defaults
		*dst = &copied
		return
	}

	current := *dst
	for _, provider := range []string{
		fantasyopenai.Name,
		fantasyanthropic.Name,
		fantasygoogle.Name,
		fantasyopenaicompat.Name,
		fantasyopenrouter.Name,
		fantasyvercel.Name,
	} {
		switch provider {
		case fantasyopenai.Name:
			if defaults.OpenAI == nil {
				continue
			}
			if current.OpenAI == nil {
				copied := *defaults.OpenAI
				current.OpenAI = &copied
				continue
			}
			dstOpenAI := current.OpenAI
			defaultOpenAI := defaults.OpenAI
			if dstOpenAI.Include == nil {
				dstOpenAI.Include = defaultOpenAI.Include
			}
			if dstOpenAI.Instructions == nil {
				dstOpenAI.Instructions = defaultOpenAI.Instructions
			}
			if dstOpenAI.LogitBias == nil {
				dstOpenAI.LogitBias = defaultOpenAI.LogitBias
			}
			if dstOpenAI.LogProbs == nil {
				dstOpenAI.LogProbs = defaultOpenAI.LogProbs
			}
			if dstOpenAI.TopLogProbs == nil {
				dstOpenAI.TopLogProbs = defaultOpenAI.TopLogProbs
			}
			if dstOpenAI.MaxToolCalls == nil {
				dstOpenAI.MaxToolCalls = defaultOpenAI.MaxToolCalls
			}
			if dstOpenAI.ParallelToolCalls == nil {
				dstOpenAI.ParallelToolCalls = defaultOpenAI.ParallelToolCalls
			}
			if dstOpenAI.User == nil {
				dstOpenAI.User = defaultOpenAI.User
			}
			if dstOpenAI.ReasoningEffort == nil {
				dstOpenAI.ReasoningEffort = defaultOpenAI.ReasoningEffort
			}
			if dstOpenAI.ReasoningSummary == nil {
				dstOpenAI.ReasoningSummary = defaultOpenAI.ReasoningSummary
			}
			if dstOpenAI.MaxCompletionTokens == nil {
				dstOpenAI.MaxCompletionTokens = defaultOpenAI.MaxCompletionTokens
			}
			if dstOpenAI.TextVerbosity == nil {
				dstOpenAI.TextVerbosity = defaultOpenAI.TextVerbosity
			}
			if dstOpenAI.Prediction == nil {
				dstOpenAI.Prediction = defaultOpenAI.Prediction
			}
			if dstOpenAI.Store == nil {
				dstOpenAI.Store = defaultOpenAI.Store
			}
			if dstOpenAI.Metadata == nil {
				dstOpenAI.Metadata = defaultOpenAI.Metadata
			}
			if dstOpenAI.PromptCacheKey == nil {
				dstOpenAI.PromptCacheKey = defaultOpenAI.PromptCacheKey
			}
			if dstOpenAI.SafetyIdentifier == nil {
				dstOpenAI.SafetyIdentifier = defaultOpenAI.SafetyIdentifier
			}
			if dstOpenAI.ServiceTier == nil {
				dstOpenAI.ServiceTier = defaultOpenAI.ServiceTier
			}
			if dstOpenAI.StructuredOutputs == nil {
				dstOpenAI.StructuredOutputs = defaultOpenAI.StructuredOutputs
			}
			if dstOpenAI.StrictJSONSchema == nil {
				dstOpenAI.StrictJSONSchema = defaultOpenAI.StrictJSONSchema
			}

		case fantasyanthropic.Name:
			if defaults.Anthropic == nil {
				continue
			}
			if current.Anthropic == nil {
				copied := *defaults.Anthropic
				current.Anthropic = &copied
				continue
			}
			dstAnthropic := current.Anthropic
			defaultAnthropic := defaults.Anthropic
			if dstAnthropic.SendReasoning == nil {
				dstAnthropic.SendReasoning = defaultAnthropic.SendReasoning
			}
			if dstAnthropic.Thinking == nil {
				dstAnthropic.Thinking = defaultAnthropic.Thinking
			} else if defaultAnthropic.Thinking != nil &&
				dstAnthropic.Thinking.BudgetTokens == nil {
				dstAnthropic.Thinking.BudgetTokens = defaultAnthropic.Thinking.BudgetTokens
			}
			if dstAnthropic.Effort == nil {
				dstAnthropic.Effort = defaultAnthropic.Effort
			}
			if dstAnthropic.DisableParallelToolUse == nil {
				dstAnthropic.DisableParallelToolUse = defaultAnthropic.DisableParallelToolUse
			}

		case fantasygoogle.Name:
			if defaults.Google == nil {
				continue
			}
			if current.Google == nil {
				copied := *defaults.Google
				current.Google = &copied
				continue
			}
			dstGoogle := current.Google
			defaultGoogle := defaults.Google
			if dstGoogle.ThinkingConfig == nil {
				dstGoogle.ThinkingConfig = defaultGoogle.ThinkingConfig
			} else if defaultGoogle.ThinkingConfig != nil {
				if dstGoogle.ThinkingConfig.ThinkingBudget == nil {
					dstGoogle.ThinkingConfig.ThinkingBudget = defaultGoogle.ThinkingConfig.ThinkingBudget
				}
				if dstGoogle.ThinkingConfig.IncludeThoughts == nil {
					dstGoogle.ThinkingConfig.IncludeThoughts = defaultGoogle.ThinkingConfig.IncludeThoughts
				}
			}
			if strings.TrimSpace(dstGoogle.CachedContent) == "" {
				dstGoogle.CachedContent = defaultGoogle.CachedContent
			}
			if dstGoogle.SafetySettings == nil {
				dstGoogle.SafetySettings = defaultGoogle.SafetySettings
			}
			if strings.TrimSpace(dstGoogle.Threshold) == "" {
				dstGoogle.Threshold = defaultGoogle.Threshold
			}

		case fantasyopenaicompat.Name:
			if defaults.OpenAICompat == nil {
				continue
			}
			if current.OpenAICompat == nil {
				copied := *defaults.OpenAICompat
				current.OpenAICompat = &copied
				continue
			}
			dstCompat := current.OpenAICompat
			defaultCompat := defaults.OpenAICompat
			if dstCompat.User == nil {
				dstCompat.User = defaultCompat.User
			}
			if dstCompat.ReasoningEffort == nil {
				dstCompat.ReasoningEffort = defaultCompat.ReasoningEffort
			}

		case fantasyopenrouter.Name:
			if defaults.OpenRouter == nil {
				continue
			}
			if current.OpenRouter == nil {
				copied := *defaults.OpenRouter
				current.OpenRouter = &copied
				continue
			}
			dstRouter := current.OpenRouter
			defaultRouter := defaults.OpenRouter
			if dstRouter.Reasoning == nil {
				dstRouter.Reasoning = defaultRouter.Reasoning
			} else if defaultRouter.Reasoning != nil {
				if dstRouter.Reasoning.Enabled == nil {
					dstRouter.Reasoning.Enabled = defaultRouter.Reasoning.Enabled
				}
				if dstRouter.Reasoning.Exclude == nil {
					dstRouter.Reasoning.Exclude = defaultRouter.Reasoning.Exclude
				}
				if dstRouter.Reasoning.MaxTokens == nil {
					dstRouter.Reasoning.MaxTokens = defaultRouter.Reasoning.MaxTokens
				}
				if dstRouter.Reasoning.Effort == nil {
					dstRouter.Reasoning.Effort = defaultRouter.Reasoning.Effort
				}
			}
			if dstRouter.ExtraBody == nil {
				dstRouter.ExtraBody = defaultRouter.ExtraBody
			}
			if dstRouter.IncludeUsage == nil {
				dstRouter.IncludeUsage = defaultRouter.IncludeUsage
			}
			if dstRouter.LogitBias == nil {
				dstRouter.LogitBias = defaultRouter.LogitBias
			}
			if dstRouter.LogProbs == nil {
				dstRouter.LogProbs = defaultRouter.LogProbs
			}
			if dstRouter.ParallelToolCalls == nil {
				dstRouter.ParallelToolCalls = defaultRouter.ParallelToolCalls
			}
			if dstRouter.User == nil {
				dstRouter.User = defaultRouter.User
			}
			if dstRouter.Provider == nil {
				dstRouter.Provider = defaultRouter.Provider
			} else if defaultRouter.Provider != nil {
				if dstRouter.Provider.Order == nil {
					dstRouter.Provider.Order = defaultRouter.Provider.Order
				}
				if dstRouter.Provider.AllowFallbacks == nil {
					dstRouter.Provider.AllowFallbacks = defaultRouter.Provider.AllowFallbacks
				}
				if dstRouter.Provider.RequireParameters == nil {
					dstRouter.Provider.RequireParameters = defaultRouter.Provider.RequireParameters
				}
				if dstRouter.Provider.DataCollection == nil {
					dstRouter.Provider.DataCollection = defaultRouter.Provider.DataCollection
				}
				if dstRouter.Provider.Only == nil {
					dstRouter.Provider.Only = defaultRouter.Provider.Only
				}
				if dstRouter.Provider.Ignore == nil {
					dstRouter.Provider.Ignore = defaultRouter.Provider.Ignore
				}
				if dstRouter.Provider.Quantizations == nil {
					dstRouter.Provider.Quantizations = defaultRouter.Provider.Quantizations
				}
				if dstRouter.Provider.Sort == nil {
					dstRouter.Provider.Sort = defaultRouter.Provider.Sort
				}
			}

		case fantasyvercel.Name:
			if defaults.Vercel == nil {
				continue
			}
			if current.Vercel == nil {
				copied := *defaults.Vercel
				current.Vercel = &copied
				continue
			}
			dstVercel := current.Vercel
			defaultVercel := defaults.Vercel
			if dstVercel.Reasoning == nil {
				dstVercel.Reasoning = defaultVercel.Reasoning
			} else if defaultVercel.Reasoning != nil {
				if dstVercel.Reasoning.Enabled == nil {
					dstVercel.Reasoning.Enabled = defaultVercel.Reasoning.Enabled
				}
				if dstVercel.Reasoning.MaxTokens == nil {
					dstVercel.Reasoning.MaxTokens = defaultVercel.Reasoning.MaxTokens
				}
				if dstVercel.Reasoning.Effort == nil {
					dstVercel.Reasoning.Effort = defaultVercel.Reasoning.Effort
				}
				if dstVercel.Reasoning.Exclude == nil {
					dstVercel.Reasoning.Exclude = defaultVercel.Reasoning.Exclude
				}
			}
			if dstVercel.ProviderOptions == nil {
				dstVercel.ProviderOptions = defaultVercel.ProviderOptions
			} else if defaultVercel.ProviderOptions != nil {
				if dstVercel.ProviderOptions.Order == nil {
					dstVercel.ProviderOptions.Order = defaultVercel.ProviderOptions.Order
				}
				if dstVercel.ProviderOptions.Models == nil {
					dstVercel.ProviderOptions.Models = defaultVercel.ProviderOptions.Models
				}
			}
			if dstVercel.User == nil {
				dstVercel.User = defaultVercel.User
			}
			if dstVercel.LogitBias == nil {
				dstVercel.LogitBias = defaultVercel.LogitBias
			}
			if dstVercel.LogProbs == nil {
				dstVercel.LogProbs = defaultVercel.LogProbs
			}
			if dstVercel.TopLogProbs == nil {
				dstVercel.TopLogProbs = defaultVercel.TopLogProbs
			}
			if dstVercel.ParallelToolCalls == nil {
				dstVercel.ParallelToolCalls = defaultVercel.ParallelToolCalls
			}
			if dstVercel.ExtraBody == nil {
				dstVercel.ExtraBody = defaultVercel.ExtraBody
			}
		}
	}
}

// ModelFromConfig resolves a provider/model pair and constructs a fantasy
// language model client using the provided provider credentials.
func ModelFromConfig(
	providerHint string,
	modelName string,
	providerKeys ProviderAPIKeys,
) (fantasy.LanguageModel, error) {
	provider, modelID, err := ResolveModelWithProviderHint(modelName, providerHint)
	if err != nil {
		return nil, err
	}

	apiKey := providerKeys.APIKey(provider)
	if apiKey == "" {
		return nil, missingProviderAPIKeyError(provider)
	}
	baseURL := providerKeys.BaseURL(provider)

	var providerClient fantasy.Provider
	switch provider {
	case fantasyanthropic.Name:
		options := []fantasyanthropic.Option{
			fantasyanthropic.WithAPIKey(apiKey),
		}
		if baseURL != "" {
			options = append(options, fantasyanthropic.WithBaseURL(baseURL))
		}
		providerClient, err = fantasyanthropic.New(options...)
	case fantasyazure.Name:
		if baseURL == "" {
			return nil, xerrors.New("AZURE_OPENAI_BASE_URL is not set")
		}
		providerClient, err = fantasyazure.New(
			fantasyazure.WithAPIKey(apiKey),
			fantasyazure.WithBaseURL(baseURL),
			fantasyazure.WithUseResponsesAPI(),
		)
	case fantasybedrock.Name:
		providerClient, err = fantasybedrock.New(fantasybedrock.WithAPIKey(apiKey))
	case fantasygoogle.Name:
		options := []fantasygoogle.Option{
			fantasygoogle.WithGeminiAPIKey(apiKey),
		}
		if baseURL != "" {
			options = append(options, fantasygoogle.WithBaseURL(baseURL))
		}
		providerClient, err = fantasygoogle.New(options...)
	case fantasyopenai.Name:
		options := []fantasyopenai.Option{
			fantasyopenai.WithAPIKey(apiKey),
			fantasyopenai.WithUseResponsesAPI(),
		}
		if baseURL != "" {
			options = append(options, fantasyopenai.WithBaseURL(baseURL))
		}
		providerClient, err = fantasyopenai.New(options...)
	case fantasyopenaicompat.Name:
		options := []fantasyopenaicompat.Option{
			fantasyopenaicompat.WithAPIKey(apiKey),
		}
		if baseURL != "" {
			options = append(options, fantasyopenaicompat.WithBaseURL(baseURL))
		}
		providerClient, err = fantasyopenaicompat.New(options...)
	case fantasyopenrouter.Name:
		providerClient, err = fantasyopenrouter.New(fantasyopenrouter.WithAPIKey(apiKey))
	case fantasyvercel.Name:
		options := []fantasyvercel.Option{
			fantasyvercel.WithAPIKey(apiKey),
		}
		if baseURL != "" {
			options = append(options, fantasyvercel.WithBaseURL(baseURL))
		}
		providerClient, err = fantasyvercel.New(options...)
	default:
		return nil, xerrors.Errorf("unsupported model provider %q", provider)
	}
	if err != nil {
		return nil, xerrors.Errorf("create %s provider: %w", provider, err)
	}

	model, err := providerClient.LanguageModel(context.Background(), modelID)
	if err != nil {
		return nil, xerrors.Errorf("load %s model: %w", provider, err)
	}
	return model, nil
}

func missingProviderAPIKeyError(provider string) error {
	switch provider {
	case fantasyanthropic.Name:
		return xerrors.New("ANTHROPIC_API_KEY is not set")
	case fantasyazure.Name:
		return xerrors.New("AZURE_OPENAI_API_KEY is not set")
	case fantasybedrock.Name:
		return xerrors.New("BEDROCK_API_KEY is not set")
	case fantasygoogle.Name:
		return xerrors.New("GOOGLE_API_KEY is not set")
	case fantasyopenai.Name:
		return xerrors.New("OPENAI_API_KEY is not set")
	case fantasyopenaicompat.Name:
		return xerrors.New("OPENAI_COMPAT_API_KEY is not set")
	case fantasyopenrouter.Name:
		return xerrors.New("OPENROUTER_API_KEY is not set")
	case fantasyvercel.Name:
		return xerrors.New("VERCEL_API_KEY is not set")
	default:
		return xerrors.Errorf("API key for provider %q is not set", provider)
	}
}

// ProviderOptionsFromChatModelConfig converts chat model provider options to
// fantasy provider options used for inference calls.
func ProviderOptionsFromChatModelConfig(
	model fantasy.LanguageModel,
	options *codersdk.ChatModelProviderOptions,
) fantasy.ProviderOptions {
	if options == nil {
		return nil
	}

	result := fantasy.ProviderOptions{}

	if options.OpenAI != nil {
		result[fantasyopenai.Name] = openAIProviderOptionsFromChatConfig(
			model,
			options.OpenAI,
		)
	}
	if options.Anthropic != nil {
		result[fantasyanthropic.Name] = anthropicProviderOptionsFromChatConfig(
			options.Anthropic,
		)
	}
	if options.Google != nil {
		result[fantasygoogle.Name] = googleProviderOptionsFromChatConfig(
			options.Google,
		)
	}
	if options.OpenAICompat != nil {
		result[fantasyopenaicompat.Name] = openAICompatProviderOptionsFromChatConfig(
			options.OpenAICompat,
		)
	}
	if options.OpenRouter != nil {
		result[fantasyopenrouter.Name] = openRouterProviderOptionsFromChatConfig(
			options.OpenRouter,
		)
	}
	if options.Vercel != nil {
		result[fantasyvercel.Name] = vercelProviderOptionsFromChatConfig(
			options.Vercel,
		)
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

func openAIProviderOptionsFromChatConfig(
	model fantasy.LanguageModel,
	options *codersdk.ChatModelOpenAIProviderOptions,
) fantasy.ProviderOptionsData {
	reasoningEffort := openAIReasoningEffortFromChat(options.ReasoningEffort)
	if useOpenAIResponsesOptions(model) {
		include := ensureOpenAIResponseIncludes(openAIIncludeFromChat(options.Include))
		providerOptions := &fantasyopenai.ResponsesProviderOptions{
			Include:           include,
			Instructions:      normalizedStringPointer(options.Instructions),
			Logprobs:          openAIResponsesLogProbsFromChat(options),
			MaxToolCalls:      options.MaxToolCalls,
			Metadata:          options.Metadata,
			ParallelToolCalls: options.ParallelToolCalls,
			PromptCacheKey:    normalizedStringPointer(options.PromptCacheKey),
			ReasoningEffort:   reasoningEffort,
			ReasoningSummary:  normalizedStringPointer(options.ReasoningSummary),
			SafetyIdentifier:  normalizedStringPointer(options.SafetyIdentifier),
			ServiceTier:       openAIServiceTierFromChat(options.ServiceTier),
			StrictJSONSchema:  options.StrictJSONSchema,
			TextVerbosity:     OpenAITextVerbosityFromChat(options.TextVerbosity),
			User:              normalizedStringPointer(options.User),
		}
		return providerOptions
	}

	return &fantasyopenai.ProviderOptions{
		LogitBias:           options.LogitBias,
		LogProbs:            options.LogProbs,
		TopLogProbs:         options.TopLogProbs,
		ParallelToolCalls:   options.ParallelToolCalls,
		User:                normalizedStringPointer(options.User),
		ReasoningEffort:     reasoningEffort,
		MaxCompletionTokens: options.MaxCompletionTokens,
		TextVerbosity:       normalizedStringPointer(options.TextVerbosity),
		Prediction:          options.Prediction,
		Store:               options.Store,
		Metadata:            options.Metadata,
		PromptCacheKey:      normalizedStringPointer(options.PromptCacheKey),
		SafetyIdentifier:    normalizedStringPointer(options.SafetyIdentifier),
		ServiceTier:         normalizedStringPointer(options.ServiceTier),
		StructuredOutputs:   options.StructuredOutputs,
	}
}

func anthropicProviderOptionsFromChatConfig(
	options *codersdk.ChatModelAnthropicProviderOptions,
) *fantasyanthropic.ProviderOptions {
	result := &fantasyanthropic.ProviderOptions{
		SendReasoning:          options.SendReasoning,
		Effort:                 anthropicEffortFromChat(options.Effort),
		DisableParallelToolUse: options.DisableParallelToolUse,
	}
	if options.Thinking != nil && options.Thinking.BudgetTokens != nil {
		result.Thinking = &fantasyanthropic.ThinkingProviderOption{
			BudgetTokens: *options.Thinking.BudgetTokens,
		}
	}
	return result
}

func googleProviderOptionsFromChatConfig(
	options *codersdk.ChatModelGoogleProviderOptions,
) *fantasygoogle.ProviderOptions {
	result := &fantasygoogle.ProviderOptions{
		CachedContent: strings.TrimSpace(options.CachedContent),
		Threshold:     strings.TrimSpace(options.Threshold),
	}
	if options.ThinkingConfig != nil {
		result.ThinkingConfig = &fantasygoogle.ThinkingConfig{
			ThinkingBudget:  options.ThinkingConfig.ThinkingBudget,
			IncludeThoughts: options.ThinkingConfig.IncludeThoughts,
		}
	}
	if options.SafetySettings != nil {
		result.SafetySettings = make(
			[]fantasygoogle.SafetySetting,
			0,
			len(options.SafetySettings),
		)
		for _, setting := range options.SafetySettings {
			result.SafetySettings = append(result.SafetySettings, fantasygoogle.SafetySetting{
				Category:  strings.TrimSpace(setting.Category),
				Threshold: strings.TrimSpace(setting.Threshold),
			})
		}
	}
	return result
}

func openAICompatProviderOptionsFromChatConfig(
	options *codersdk.ChatModelOpenAICompatProviderOptions,
) *fantasyopenaicompat.ProviderOptions {
	return &fantasyopenaicompat.ProviderOptions{
		User:            normalizedStringPointer(options.User),
		ReasoningEffort: openAIReasoningEffortFromChat(options.ReasoningEffort),
	}
}

func openRouterProviderOptionsFromChatConfig(
	options *codersdk.ChatModelOpenRouterProviderOptions,
) *fantasyopenrouter.ProviderOptions {
	result := &fantasyopenrouter.ProviderOptions{
		ExtraBody:         options.ExtraBody,
		IncludeUsage:      options.IncludeUsage,
		LogitBias:         options.LogitBias,
		LogProbs:          options.LogProbs,
		ParallelToolCalls: options.ParallelToolCalls,
		User:              normalizedStringPointer(options.User),
	}
	if options.Reasoning != nil {
		result.Reasoning = &fantasyopenrouter.ReasoningOptions{
			Enabled:   options.Reasoning.Enabled,
			Exclude:   options.Reasoning.Exclude,
			MaxTokens: options.Reasoning.MaxTokens,
			Effort:    openRouterReasoningEffortFromChat(options.Reasoning.Effort),
		}
	}
	if options.Provider != nil {
		result.Provider = &fantasyopenrouter.Provider{
			Order:             options.Provider.Order,
			AllowFallbacks:    options.Provider.AllowFallbacks,
			RequireParameters: options.Provider.RequireParameters,
			DataCollection:    normalizedStringPointer(options.Provider.DataCollection),
			Only:              options.Provider.Only,
			Ignore:            options.Provider.Ignore,
			Quantizations:     options.Provider.Quantizations,
			Sort:              normalizedStringPointer(options.Provider.Sort),
		}
	}
	return result
}

func vercelProviderOptionsFromChatConfig(
	options *codersdk.ChatModelVercelProviderOptions,
) *fantasyvercel.ProviderOptions {
	result := &fantasyvercel.ProviderOptions{
		User:              normalizedStringPointer(options.User),
		LogitBias:         options.LogitBias,
		LogProbs:          options.LogProbs,
		TopLogProbs:       options.TopLogProbs,
		ParallelToolCalls: options.ParallelToolCalls,
		ExtraBody:         options.ExtraBody,
	}
	if options.Reasoning != nil {
		result.Reasoning = &fantasyvercel.ReasoningOptions{
			Enabled:   options.Reasoning.Enabled,
			MaxTokens: options.Reasoning.MaxTokens,
			Effort:    vercelReasoningEffortFromChat(options.Reasoning.Effort),
			Exclude:   options.Reasoning.Exclude,
		}
	}
	if options.ProviderOptions != nil {
		result.ProviderOptions = &fantasyvercel.GatewayProviderOptions{
			Order:  options.ProviderOptions.Order,
			Models: options.ProviderOptions.Models,
		}
	}
	return result
}

func openAIResponsesLogProbsFromChat(
	options *codersdk.ChatModelOpenAIProviderOptions,
) any {
	if options.TopLogProbs != nil {
		return *options.TopLogProbs
	}
	if options.LogProbs != nil {
		return *options.LogProbs
	}
	return nil
}

func openAIIncludeFromChat(values []string) []fantasyopenai.IncludeType {
	if values == nil {
		return nil
	}

	result := make([]fantasyopenai.IncludeType, 0, len(values))
	for _, value := range values {
		switch strings.TrimSpace(value) {
		case string(fantasyopenai.IncludeReasoningEncryptedContent):
			result = append(result, fantasyopenai.IncludeReasoningEncryptedContent)
		case string(fantasyopenai.IncludeFileSearchCallResults):
			result = append(result, fantasyopenai.IncludeFileSearchCallResults)
		case string(fantasyopenai.IncludeMessageOutputTextLogprobs):
			result = append(result, fantasyopenai.IncludeMessageOutputTextLogprobs)
		}
	}
	return result
}

func ensureOpenAIResponseIncludes(
	values []fantasyopenai.IncludeType,
) []fantasyopenai.IncludeType {
	const required = fantasyopenai.IncludeReasoningEncryptedContent

	for _, value := range values {
		if value == required {
			return values
		}
	}
	return append(values, required)
}

func useOpenAIResponsesOptions(model fantasy.LanguageModel) bool {
	if model == nil {
		return false
	}
	switch model.Provider() {
	case fantasyopenai.Name, fantasyazure.Name:
		return fantasyopenai.IsResponsesModel(model.Model())
	default:
		return false
	}
}

func normalizedStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func openAIReasoningEffortFromChat(value *string) *fantasyopenai.ReasoningEffort {
	effort := ReasoningEffortFromChat(fantasyopenai.Name, value)
	if effort == nil {
		return nil
	}
	valueCopy := fantasyopenai.ReasoningEffort(*effort)
	return &valueCopy
}

func anthropicEffortFromChat(value *string) *fantasyanthropic.Effort {
	effort := ReasoningEffortFromChat(fantasyanthropic.Name, value)
	if effort == nil {
		return nil
	}
	valueCopy := fantasyanthropic.Effort(*effort)
	return &valueCopy
}

func openRouterReasoningEffortFromChat(value *string) *fantasyopenrouter.ReasoningEffort {
	effort := ReasoningEffortFromChat(fantasyopenrouter.Name, value)
	if effort == nil {
		return nil
	}
	valueCopy := fantasyopenrouter.ReasoningEffort(*effort)
	return &valueCopy
}

func vercelReasoningEffortFromChat(value *string) *fantasyvercel.ReasoningEffort {
	effort := ReasoningEffortFromChat(fantasyvercel.Name, value)
	if effort == nil {
		return nil
	}
	valueCopy := fantasyvercel.ReasoningEffort(*effort)
	return &valueCopy
}

func openAIServiceTierFromChat(value *string) *fantasyopenai.ServiceTier {
	normalized := normalizedStringPointer(value)
	if normalized == nil {
		return nil
	}
	switch strings.ToLower(*normalized) {
	case string(fantasyopenai.ServiceTierAuto):
		serviceTier := fantasyopenai.ServiceTierAuto
		return &serviceTier
	case string(fantasyopenai.ServiceTierFlex):
		serviceTier := fantasyopenai.ServiceTierFlex
		return &serviceTier
	case string(fantasyopenai.ServiceTierPriority):
		serviceTier := fantasyopenai.ServiceTierPriority
		return &serviceTier
	default:
		return nil
	}
}
