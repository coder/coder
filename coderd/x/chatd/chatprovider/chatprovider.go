package chatprovider

import (
	"context"
	"net/http"
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
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatopenai"
	"github.com/coder/coder/v2/coderd/x/chatd/chatutil"
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

// ProviderAllowsAmbientCredentials reports whether provider can use
// ambient credentials from the Coder server instead of an explicit
// API key.
func ProviderAllowsAmbientCredentials(provider string) bool {
	return NormalizeProvider(provider) == fantasybedrock.Name
}

// ProviderAPIKeys contains API keys for provider calls.
type ProviderAPIKeys struct {
	OpenAI            string
	Anthropic         string
	ByProvider        map[string]string
	BaseURLByProvider map[string]string
}

// UserProviderKey is a user-supplied API key for a specific provider.
type UserProviderKey struct {
	ChatProviderID uuid.UUID
	APIKey         string
}

// ProviderAvailability describes whether a provider has a usable
// API key and, if not, why.
type ProviderAvailability struct {
	Available         bool
	UnavailableReason codersdk.ChatModelProviderUnavailableReason
}

// ConfiguredProvider is an enabled provider loaded from database config.
type ConfiguredProvider struct {
	ProviderID                 uuid.UUID
	Provider                   string
	APIKey                     string
	BaseURL                    string
	CentralAPIKeyEnabled       bool
	AllowUserAPIKey            bool
	AllowCentralAPIKeyFallback bool
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

// HasProvider reports whether a provider has an explicit resolved entry
// in the provider key map, even when the resolved key is empty.
func (k ProviderAPIKeys) HasProvider(provider string) bool {
	normalized := NormalizeProvider(provider)
	if normalized == "" || k.ByProvider == nil {
		return false
	}
	_, ok := k.ByProvider[normalized]
	return ok
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

// ResolveUserProviderKeys computes effective API keys and per-provider
// availability for a given user. It considers the provider's credential
// policy flags alongside central (DB/deployment) keys and the user's
// personal keys.
func ResolveUserProviderKeys(
	fallback ProviderAPIKeys,
	providers []ConfiguredProvider,
	userKeys []UserProviderKey,
) (ProviderAPIKeys, map[string]ProviderAvailability) {
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

	userKeyByProviderID := make(map[uuid.UUID]string, len(userKeys))
	for _, userKey := range userKeys {
		if userKey.ChatProviderID == uuid.Nil {
			continue
		}
		if key := strings.TrimSpace(userKey.APIKey); key != "" {
			userKeyByProviderID[userKey.ChatProviderID] = key
		}
	}

	availabilityByProvider := make(map[string]ProviderAvailability, len(providers))
	for _, provider := range providers {
		normalizedProvider := NormalizeProvider(provider.Provider)
		if normalizedProvider == "" {
			continue
		}

		if url := strings.TrimSpace(provider.BaseURL); url != "" {
			merged.BaseURLByProvider[normalizedProvider] = url
		}

		var userKey string
		if provider.ProviderID != uuid.Nil {
			userKey = userKeyByProviderID[provider.ProviderID]
		}

		var centralKey string
		if provider.CentralAPIKeyEnabled {
			if key := strings.TrimSpace(provider.APIKey); key != "" {
				centralKey = key
			} else {
				centralKey = fallback.APIKey(normalizedProvider)
			}
		}

		resolved := ProviderAvailability{}
		chosenKey := ""
		switch {
		case provider.AllowUserAPIKey && userKey != "":
			chosenKey = userKey
			resolved.Available = true
		case centralKey != "":
			if !provider.AllowUserAPIKey || provider.AllowCentralAPIKeyFallback {
				chosenKey = centralKey
				resolved.Available = true
			} else {
				resolved.UnavailableReason = codersdk.ChatModelProviderUnavailableReasonUserAPIKeyRequired
			}
		case normalizedProvider == fantasybedrock.Name && provider.CentralAPIKeyEnabled:
			// Bedrock can use ambient AWS credentials from the Coder server
			// without an explicit key, but only when the credential policy
			// allows central credentials to satisfy the request.
			if !provider.AllowUserAPIKey || provider.AllowCentralAPIKeyFallback {
				resolved.Available = true
			} else {
				resolved.UnavailableReason = codersdk.ChatModelProviderUnavailableReasonUserAPIKeyRequired
			}
		case provider.AllowUserAPIKey && provider.AllowCentralAPIKeyFallback && provider.CentralAPIKeyEnabled:
			// When users can add their own key, a missing central fallback key is
			// still something the user can remedy.
			resolved.UnavailableReason = codersdk.ChatModelProviderUnavailableReasonUserAPIKeyRequired
		case provider.AllowUserAPIKey:
			resolved.UnavailableReason = codersdk.ChatModelProviderUnavailableReasonUserAPIKeyRequired
		default:
			resolved.UnavailableReason = codersdk.ChatModelProviderUnavailableMissingAPIKey
		}

		setResolvedProviderAPIKey(&merged, normalizedProvider, chosenKey, resolved)
		availabilityByProvider[normalizedProvider] = resolved
	}

	return merged, availabilityByProvider
}

// setResolvedProviderAPIKey keeps ByProvider presence aligned with
// resolved provider availability. An empty value means ambient
// credentials may satisfy the provider. An absent entry means the
// provider is not resolvable.
func setResolvedProviderAPIKey(keys *ProviderAPIKeys, provider string, apiKey string, availability ProviderAvailability) {
	normalizedProvider := NormalizeProvider(provider)
	if normalizedProvider == "" {
		return
	}
	if keys.ByProvider == nil {
		keys.ByProvider = map[string]string{}
	}

	delete(keys.ByProvider, normalizedProvider)
	trimmedKey := strings.TrimSpace(apiKey)
	switch normalizedProvider {
	case fantasyopenai.Name:
		keys.OpenAI = trimmedKey
	case fantasyanthropic.Name:
		keys.Anthropic = trimmedKey
	}
	if trimmedKey != "" || (availability.Available && ProviderAllowsAmbientCredentials(normalizedProvider)) {
		keys.ByProvider[normalizedProvider] = trimmedKey
	}
}

type ModelCatalog struct{}

func NewModelCatalog() *ModelCatalog {
	return &ModelCatalog{}
}

// ListConfiguredModels returns a model catalog from enabled DB-backed model
// configs. The second return value reports whether DB-backed models were used.
func (*ModelCatalog) ListConfiguredModels(
	configuredProviders []ConfiguredProvider,
	configuredModels []ConfiguredModel,
	availabilityByProvider map[string]ProviderAvailability,
	enabledProviders map[string]struct{},
) (codersdk.ChatModelsResponse, bool) {
	if len(configuredModels) == 0 {
		return codersdk.ChatModelsResponse{}, false
	}

	modelsByProvider := make(map[string][]codersdk.ChatModel)
	seenByProvider := make(map[string]map[string]struct{})
	providerSet := make(map[string]struct{})

	for _, provider := range configuredProviders {
		normalized := NormalizeProvider(provider.Provider)
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

	response := codersdk.ChatModelsResponse{
		Providers: make([]codersdk.ChatModelProvider, 0, len(providers)),
	}
	for _, provider := range providers {
		if _, ok := enabledProviders[provider]; !ok {
			continue
		}

		models := modelsByProvider[provider]
		sortChatModels(models)

		result := codersdk.ChatModelProvider{
			Provider: provider,
			Models:   models,
		}
		if avail, ok := availabilityByProvider[provider]; ok {
			result.Available = avail.Available
			if !avail.Available {
				result.UnavailableReason = avail.UnavailableReason
			}
		} else {
			result.Available = false
			result.UnavailableReason = codersdk.ChatModelProviderUnavailableMissingAPIKey
		}

		response.Providers = append(response.Providers, result)
	}

	return response, true
}

// ListConfiguredProviderAvailability returns provider availability derived from
// the policy-aware availability map for enabled providers.
func (*ModelCatalog) ListConfiguredProviderAvailability(
	availabilityByProvider map[string]ProviderAvailability,
	enabledProviders map[string]struct{},
) codersdk.ChatModelsResponse {
	response := codersdk.ChatModelsResponse{
		Providers: make([]codersdk.ChatModelProvider, 0, len(supportedProviderNames)),
	}

	for _, provider := range supportedProviderNames {
		if _, ok := enabledProviders[provider]; !ok {
			continue
		}

		result := codersdk.ChatModelProvider{
			Provider: provider,
			Models:   []codersdk.ChatModel{},
		}
		if avail, ok := availabilityByProvider[provider]; ok {
			result.Available = avail.Available
			if !avail.Available {
				result.UnavailableReason = avail.UnavailableReason
			}
		} else {
			result.Available = false
			result.UnavailableReason = codersdk.ChatModelProviderUnavailableMissingAPIKey
		}

		response.Providers = append(response.Providers, result)
	}

	return response
}

// PruneDisabledProviderKeys removes entries from keys that do not
// belong to an enabled provider. It clears ByProvider and
// BaseURLByProvider entries for disabled providers and zeroes the
// legacy OpenAI and Anthropic fields when those providers are not
// enabled.
func PruneDisabledProviderKeys(keys *ProviderAPIKeys, enabledProviders map[string]struct{}) {
	for provider := range keys.ByProvider {
		if _, ok := enabledProviders[provider]; ok {
			continue
		}
		delete(keys.ByProvider, provider)
		delete(keys.BaseURLByProvider, provider)
	}
	if _, ok := enabledProviders[NormalizeProvider("openai")]; !ok {
		keys.OpenAI = ""
	}
	if _, ok := enabledProviders[NormalizeProvider("anthropic")]; !ok {
		keys.Anthropic = ""
	}
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

func ResolveModelWithProviderHint(modelName, providerHint string) (provider string, model string, err error) {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return "", "", xerrors.New("model is required")
	}

	if provider, modelID, ok := parseCanonicalModelRef(modelName); ok {
		return provider, modelID, nil
	}

	if provider := NormalizeProvider(providerHint); provider != "" {
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

		provider := NormalizeProvider(parts[0])
		modelID := strings.TrimSpace(parts[1])
		if provider != "" && modelID != "" {
			return provider, modelID, true
		}
	}

	return "", "", false
}

func isChatModelForProvider(provider, modelID string) bool {
	normalizedProvider := NormalizeProvider(provider)
	normalizedModel := strings.ToLower(strings.TrimSpace(modelID))
	switch normalizedProvider {
	case fantasyopenai.Name:
		return strings.HasPrefix(normalizedModel, "gpt-") ||
			strings.HasPrefix(normalizedModel, "chatgpt-") ||
			chatopenai.IsReasoningModel(normalizedModel)
	case fantasyanthropic.Name:
		return strings.HasPrefix(normalizedModel, "claude-")
	case fantasygoogle.Name:
		return strings.HasPrefix(normalizedModel, "gemini-") ||
			strings.HasPrefix(normalizedModel, "gemma-")
	default:
		return false
	}
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
		effort := chatopenai.ReasoningEffortFromChat(value)
		if effort == nil {
			return nil
		}
		valueCopy := string(*effort)
		return &valueCopy
	case fantasyanthropic.Name:
		return chatutil.NormalizedEnumValue(
			normalized,
			string(fantasyanthropic.EffortLow),
			string(fantasyanthropic.EffortMedium),
			string(fantasyanthropic.EffortHigh),
			string(fantasyanthropic.EffortXHigh),
			string(fantasyanthropic.EffortMax),
		)
	case fantasyopenrouter.Name:
		return chatutil.NormalizedEnumValue(
			normalized,
			string(fantasyopenrouter.ReasoningEffortLow),
			string(fantasyopenrouter.ReasoningEffortMedium),
			string(fantasyopenrouter.ReasoningEffortHigh),
		)
	case fantasyvercel.Name:
		return chatutil.NormalizedEnumValue(
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

// ApplyReasoningEffortToOptions applies the given reasoning_effort to every
// provider entry in providerOptions that understands it. When model is
// non-nil and the options map has no entry for the model's provider, this
// function seeds a minimal provider-specific options struct so the mutation
// still lands. Callers that produced providerOptions from a chat model
// config with no provider_options block would otherwise see
// reasoning_effort silently dropped.
//
// The returned map is the (possibly newly-allocated) providerOptions; the
// input is mutated in-place when non-nil.
func ApplyReasoningEffortToOptions(
	providerOptions fantasy.ProviderOptions,
	model fantasy.LanguageModel,
	reasoningEffort string,
) fantasy.ProviderOptions {
	reasoningEffort = strings.TrimSpace(reasoningEffort)
	if reasoningEffort == "" {
		return providerOptions
	}

	if model != nil {
		providerOptions = seedProviderOptionsForModel(providerOptions, model)
	}
	if providerOptions == nil {
		return nil
	}

	applyReasoningEffortDispatch(providerOptions, reasoningEffort)
	return providerOptions
}

// seedProviderOptionsForModel ensures providerOptions has an entry for the
// given model's provider, allocating a minimal options struct when absent.
// Returns the possibly newly-allocated options map. Unknown providers are
// left untouched so callers get their input back unchanged.
func seedProviderOptionsForModel(
	providerOptions fantasy.ProviderOptions,
	model fantasy.LanguageModel,
) fantasy.ProviderOptions {
	provider := model.Provider()
	var seed fantasy.ProviderOptionsData
	switch provider {
	case fantasyopenai.Name:
		if fantasyopenai.IsResponsesModel(model.Model()) {
			seed = &fantasyopenai.ResponsesProviderOptions{}
		} else {
			seed = &fantasyopenai.ProviderOptions{}
		}
	case fantasyanthropic.Name:
		seed = &fantasyanthropic.ProviderOptions{}
	case fantasyopenaicompat.Name:
		seed = &fantasyopenaicompat.ProviderOptions{}
	case fantasyopenrouter.Name:
		seed = &fantasyopenrouter.ProviderOptions{}
	case fantasyvercel.Name:
		seed = &fantasyvercel.ProviderOptions{}
	default:
		return providerOptions
	}

	if providerOptions == nil {
		providerOptions = fantasy.ProviderOptions{}
	}
	if _, ok := providerOptions[provider]; !ok {
		providerOptions[provider] = seed
	}
	return providerOptions
}

// applyReasoningEffortDispatch routes the normalized reasoning_effort to
// every provider entry present in providerOptions. Adding a new provider
// here (and only here) keeps chatd callers in sync automatically.
func applyReasoningEffortDispatch(
	providerOptions fantasy.ProviderOptions,
	reasoningEffort string,
) {
	if normalized := ReasoningEffortFromChat(
		fantasyopenai.Name,
		&reasoningEffort,
	); normalized != nil {
		effort := fantasyopenai.ReasoningEffort(*normalized)
		if raw, ok := providerOptions[fantasyopenai.Name]; ok {
			switch opts := raw.(type) {
			case *fantasyopenai.ProviderOptions:
				opts.ReasoningEffort = &effort
			case *fantasyopenai.ResponsesProviderOptions:
				opts.ReasoningEffort = &effort
			}
		}
		if raw, ok := providerOptions[fantasyopenaicompat.Name]; ok {
			if opts, ok := raw.(*fantasyopenaicompat.ProviderOptions); ok {
				opts.ReasoningEffort = &effort
			}
		}
	}

	if normalized := ReasoningEffortFromChat(
		fantasyanthropic.Name,
		&reasoningEffort,
	); normalized != nil {
		if raw, ok := providerOptions[fantasyanthropic.Name]; ok {
			if opts, ok := raw.(*fantasyanthropic.ProviderOptions); ok {
				effort := fantasyanthropic.Effort(*normalized)
				opts.Effort = &effort
			}
		}
	}

	if normalized := ReasoningEffortFromChat(
		fantasyopenrouter.Name,
		&reasoningEffort,
	); normalized != nil {
		if raw, ok := providerOptions[fantasyopenrouter.Name]; ok {
			if opts, ok := raw.(*fantasyopenrouter.ProviderOptions); ok {
				if opts.Reasoning == nil {
					opts.Reasoning = &fantasyopenrouter.ReasoningOptions{}
				}
				effort := fantasyopenrouter.ReasoningEffort(*normalized)
				opts.Reasoning.Effort = &effort
			}
		}
	}

	if normalized := ReasoningEffortFromChat(
		fantasyvercel.Name,
		&reasoningEffort,
	); normalized != nil {
		if raw, ok := providerOptions[fantasyvercel.Name]; ok {
			if opts, ok := raw.(*fantasyvercel.ProviderOptions); ok {
				if opts.Reasoning == nil {
					opts.Reasoning = &fantasyvercel.ReasoningOptions{}
				}
				effort := fantasyvercel.ReasoningEffort(*normalized)
				opts.Reasoning.Effort = &effort
			}
		}
	}
}

// MergeMissingModelCostConfig fills unset pricing metadata from defaults.
func MergeMissingModelCostConfig(
	dst **codersdk.ModelCostConfig,
	defaults *codersdk.ModelCostConfig,
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
	if current.InputPricePerMillionTokens == nil {
		current.InputPricePerMillionTokens = defaults.InputPricePerMillionTokens
	}
	if current.OutputPricePerMillionTokens == nil {
		current.OutputPricePerMillionTokens = defaults.OutputPricePerMillionTokens
	}
	if current.CacheReadPricePerMillionTokens == nil {
		current.CacheReadPricePerMillionTokens = defaults.CacheReadPricePerMillionTokens
	}
	if current.CacheWritePricePerMillionTokens == nil {
		current.CacheWritePricePerMillionTokens = defaults.CacheWritePricePerMillionTokens
	}
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

// Header constants sent on upstream LLM API requests so that
// intermediaries (e.g. aibridged) can correlate traffic back to
// Coder entities.
const (
	// HeaderCoderOwnerID identifies the Coder user who owns the chat.
	HeaderCoderOwnerID = "X-Coder-Owner-Id"
	// HeaderCoderChatID identifies the top-level (parent) chat.
	// For root chats this is the chat's own ID; for subchats it
	// is the parent chat's ID.
	HeaderCoderChatID = "X-Coder-Chat-Id"
	// HeaderCoderSubchatID identifies the current subchat. Only
	// present when the request originates from a child chat.
	HeaderCoderSubchatID = "X-Coder-Subchat-Id"
	// HeaderCoderWorkspaceID identifies the workspace associated
	// with the chat, if any.
	HeaderCoderWorkspaceID = "X-Coder-Workspace-Id"
)

// CoderHeaders builds the set of Coder identity headers to attach
// to outgoing LLM API requests for the given chat.
func CoderHeaders(chat database.Chat) map[string]string {
	chatID := chat.ID
	if chat.ParentChatID.Valid {
		chatID = chat.ParentChatID.UUID
	}
	h := map[string]string{
		HeaderCoderOwnerID: chat.OwnerID.String(),
		HeaderCoderChatID:  chatID.String(),
	}
	if chat.ParentChatID.Valid {
		h[HeaderCoderSubchatID] = chat.ID.String()
	}
	if chat.WorkspaceID.Valid {
		h[HeaderCoderWorkspaceID] = chat.WorkspaceID.UUID.String()
	}
	return h
}

// CoderHeadersFromIDs is a convenience form of CoderHeaders for call
// sites that do not have a full database.Chat in scope.
func CoderHeadersFromIDs(
	ownerID uuid.UUID,
	chatID uuid.UUID,
	parentChatID uuid.NullUUID,
	workspaceID uuid.NullUUID,
) map[string]string {
	return CoderHeaders(database.Chat{
		ID:           chatID,
		OwnerID:      ownerID,
		ParentChatID: parentChatID,
		WorkspaceID:  workspaceID,
	})
}

// ModelFromConfig resolves a provider/model pair and constructs a fantasy
// language model client using the provided provider credentials. The
// userAgent is sent as the User-Agent header on every outgoing LLM
// API request. extraHeaders, when non-nil, are sent as additional
// HTTP headers on every request. httpClient, when non-nil, is used for
// all provider HTTP requests.
func ModelFromConfig(
	providerHint string,
	modelName string,
	providerKeys ProviderAPIKeys,
	userAgent string,
	extraHeaders map[string]string,
	httpClient *http.Client,
) (fantasy.LanguageModel, error) {
	provider, modelID, err := ResolveModelWithProviderHint(modelName, providerHint)
	if err != nil {
		return nil, err
	}

	apiKey := providerKeys.APIKey(provider)
	if apiKey == "" &&
		!(ProviderAllowsAmbientCredentials(provider) && providerKeys.HasProvider(provider)) {
		return nil, missingProviderAPIKeyError(provider)
	}
	baseURL := providerKeys.BaseURL(provider)

	var providerClient fantasy.Provider
	switch provider {
	case fantasyanthropic.Name:
		options := []fantasyanthropic.Option{
			fantasyanthropic.WithAPIKey(apiKey),
			fantasyanthropic.WithUserAgent(userAgent),
		}
		if len(extraHeaders) > 0 {
			options = append(options, fantasyanthropic.WithHeaders(extraHeaders))
		}
		if baseURL != "" {
			options = append(options, fantasyanthropic.WithBaseURL(baseURL))
		}
		if httpClient != nil {
			options = append(options, fantasyanthropic.WithHTTPClient(httpClient))
		}
		providerClient, err = fantasyanthropic.New(options...)
	case fantasyazure.Name:
		if baseURL == "" {
			return nil, xerrors.New("AZURE_OPENAI_BASE_URL is not set")
		}
		azureOpts := []fantasyazure.Option{
			fantasyazure.WithAPIKey(apiKey),
			fantasyazure.WithBaseURL(baseURL),
			fantasyazure.WithUseResponsesAPI(),
			fantasyazure.WithUserAgent(userAgent),
		}
		if len(extraHeaders) > 0 {
			azureOpts = append(azureOpts, fantasyazure.WithHeaders(extraHeaders))
		}
		if httpClient != nil {
			azureOpts = append(azureOpts, fantasyazure.WithHTTPClient(httpClient))
		}
		providerClient, err = fantasyazure.New(azureOpts...)
	case fantasybedrock.Name:
		bedrockOpts := []fantasybedrock.Option{
			fantasybedrock.WithUserAgent(userAgent),
		}
		if apiKey != "" {
			bedrockOpts = append(bedrockOpts, fantasybedrock.WithAPIKey(apiKey))
		}
		if len(extraHeaders) > 0 {
			bedrockOpts = append(bedrockOpts, fantasybedrock.WithHeaders(extraHeaders))
		}
		if baseURL != "" {
			bedrockOpts = append(bedrockOpts, fantasybedrock.WithBaseURL(baseURL))
		}
		if httpClient != nil {
			bedrockOpts = append(bedrockOpts, fantasybedrock.WithHTTPClient(httpClient))
		}
		providerClient, err = fantasybedrock.New(bedrockOpts...)
	case fantasygoogle.Name:
		options := []fantasygoogle.Option{
			fantasygoogle.WithGeminiAPIKey(apiKey),
			fantasygoogle.WithUserAgent(userAgent),
		}
		if len(extraHeaders) > 0 {
			options = append(options, fantasygoogle.WithHeaders(extraHeaders))
		}
		if baseURL != "" {
			options = append(options, fantasygoogle.WithBaseURL(baseURL))
		}
		if httpClient != nil {
			options = append(options, fantasygoogle.WithHTTPClient(httpClient))
		}
		providerClient, err = fantasygoogle.New(options...)
	case fantasyopenai.Name:
		options := []fantasyopenai.Option{
			fantasyopenai.WithAPIKey(apiKey),
			fantasyopenai.WithUseResponsesAPI(),
			fantasyopenai.WithUserAgent(userAgent),
		}
		if len(extraHeaders) > 0 {
			options = append(options, fantasyopenai.WithHeaders(extraHeaders))
		}
		if baseURL != "" {
			options = append(options, fantasyopenai.WithBaseURL(baseURL))
		}
		if httpClient != nil {
			options = append(options, fantasyopenai.WithHTTPClient(httpClient))
		}
		providerClient, err = fantasyopenai.New(options...)
	case fantasyopenaicompat.Name:
		options := []fantasyopenaicompat.Option{
			fantasyopenaicompat.WithAPIKey(apiKey),
			fantasyopenaicompat.WithUserAgent(userAgent),
		}
		if len(extraHeaders) > 0 {
			options = append(options, fantasyopenaicompat.WithHeaders(extraHeaders))
		}
		if baseURL != "" {
			options = append(options, fantasyopenaicompat.WithBaseURL(baseURL))
		}
		if httpClient != nil {
			options = append(options, fantasyopenaicompat.WithHTTPClient(httpClient))
		}
		providerClient, err = fantasyopenaicompat.New(options...)
	case fantasyopenrouter.Name:
		routerOpts := []fantasyopenrouter.Option{
			fantasyopenrouter.WithAPIKey(apiKey),
			fantasyopenrouter.WithUserAgent(userAgent),
		}
		if len(extraHeaders) > 0 {
			routerOpts = append(routerOpts, fantasyopenrouter.WithHeaders(extraHeaders))
		}
		if httpClient != nil {
			routerOpts = append(routerOpts, fantasyopenrouter.WithHTTPClient(httpClient))
		}
		providerClient, err = fantasyopenrouter.New(routerOpts...)
	case fantasyvercel.Name:
		options := []fantasyvercel.Option{
			fantasyvercel.WithAPIKey(apiKey),
			fantasyvercel.WithUserAgent(userAgent),
		}
		if len(extraHeaders) > 0 {
			options = append(options, fantasyvercel.WithHeaders(extraHeaders))
		}
		if baseURL != "" {
			options = append(options, fantasyvercel.WithBaseURL(baseURL))
		}
		if httpClient != nil {
			options = append(options, fantasyvercel.WithHTTPClient(httpClient))
		}
		providerClient, err = fantasyvercel.New(options...)
	default:
		return nil, xerrors.Errorf("unsupported model provider %q", provider)
	}
	if err != nil {
		return nil, providerCreationError(provider, err)
	}

	model, err := providerClient.LanguageModel(context.Background(), modelID)
	if err != nil {
		return nil, xerrors.Errorf("load %s model: %w", provider, err)
	}
	return model, nil
}

func providerCreationError(provider string, err error) error {
	return xerrors.Errorf("create %s provider: %w", provider, err)
}

// Providers that allow ambient credentials, such as Bedrock, bypass
// this helper only after ResolveUserProviderKeys marks them
// available.
func missingProviderAPIKeyError(provider string) error {
	switch provider {
	case fantasyanthropic.Name:
		return xerrors.New("ANTHROPIC_API_KEY is not set")
	case fantasyazure.Name:
		return xerrors.New("AZURE_OPENAI_API_KEY is not set")
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
		result[fantasyopenai.Name] = chatopenai.ProviderOptionsFromChatConfig(
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
		User:            chatutil.NormalizedStringPointer(options.User),
		ReasoningEffort: chatopenai.ReasoningEffortFromChat(options.ReasoningEffort),
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
		User:              chatutil.NormalizedStringPointer(options.User),
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
			DataCollection:    chatutil.NormalizedStringPointer(options.Provider.DataCollection),
			Only:              options.Provider.Only,
			Ignore:            options.Provider.Ignore,
			Quantizations:     options.Provider.Quantizations,
			Sort:              chatutil.NormalizedStringPointer(options.Provider.Sort),
		}
	}
	return result
}

func vercelProviderOptionsFromChatConfig(
	options *codersdk.ChatModelVercelProviderOptions,
) *fantasyvercel.ProviderOptions {
	result := &fantasyvercel.ProviderOptions{
		User:              chatutil.NormalizedStringPointer(options.User),
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
