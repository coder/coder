package chatprovider

import (
	"sort"
	"strings"

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

	extras := make([]string, 0, len(providerSet))
	for provider := range providerSet {
		if NormalizeProvider(provider) != "" {
			continue
		}
		extras = append(extras, provider)
	}
	sort.Strings(extras)

	return append(ordered, extras...)
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

func normalizeProvider(provider string) string {
	return NormalizeProvider(provider)
}

func ResolveModelWithProviderHint(modelName, providerHint string) (string, string, error) {
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

func parseCanonicalModelRef(modelRef string) (string, string, bool) {
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

func normalizedEnumValue(value string, allowed ...string) *string {
	for _, candidate := range allowed {
		if value == strings.ToLower(candidate) {
			match := candidate
			return &match
		}
	}
	return nil
}
