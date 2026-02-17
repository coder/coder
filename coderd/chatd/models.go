package chatd

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	aianthropic "go.jetify.com/ai/provider/anthropic"
	aiopenai "go.jetify.com/ai/provider/openai"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/codersdk"
)

const (
	defaultOpenAIModelsURL    = "https://api.openai.com/v1/models"
	defaultAnthropicModelsURL = "https://api.anthropic.com/v1/models"
	anthropicAPIVersion       = "2023-06-01"
)

// ProviderAPIKeys contains API keys for provider calls.
type ProviderAPIKeys struct {
	OpenAI    string
	Anthropic string
}

// ConfiguredProvider is an enabled provider loaded from database config.
type ConfiguredProvider struct {
	Provider string
	APIKey   string
}

// ConfiguredModel is an enabled model loaded from database config.
type ConfiguredModel struct {
	Provider    string
	Model       string
	DisplayName string
}

func (k ProviderAPIKeys) apiKey(provider string) string {
	switch normalizeProvider(provider) {
	case aiopenai.ProviderName:
		return strings.TrimSpace(k.OpenAI)
	case aianthropic.ProviderName:
		return strings.TrimSpace(k.Anthropic)
	default:
		return ""
	}
}

// MergeProviderAPIKeys overlays configured provider keys over fallback keys.
func MergeProviderAPIKeys(fallback ProviderAPIKeys, providers []ConfiguredProvider) ProviderAPIKeys {
	merged := ProviderAPIKeys{
		OpenAI:    strings.TrimSpace(fallback.OpenAI),
		Anthropic: strings.TrimSpace(fallback.Anthropic),
	}

	for _, provider := range providers {
		switch normalizeProvider(provider.Provider) {
		case aiopenai.ProviderName:
			if key := strings.TrimSpace(provider.APIKey); key != "" {
				merged.OpenAI = key
			}
		case aianthropic.ProviderName:
			if key := strings.TrimSpace(provider.APIKey); key != "" {
				merged.Anthropic = key
			}
		}
	}

	return merged
}

// ModelCatalogConfig controls model catalog lookups and filtering.
type ModelCatalogConfig struct {
	OpenAIModelsURL    string
	AnthropicModelsURL string
	Allowlist          string
	Denylist           string
}

func (c ModelCatalogConfig) withDefaults() ModelCatalogConfig {
	cfg := ModelCatalogConfig{
		OpenAIModelsURL:    strings.TrimSpace(c.OpenAIModelsURL),
		AnthropicModelsURL: strings.TrimSpace(c.AnthropicModelsURL),
		Allowlist:          strings.TrimSpace(c.Allowlist),
		Denylist:           strings.TrimSpace(c.Denylist),
	}
	if cfg.OpenAIModelsURL == "" {
		cfg.OpenAIModelsURL = defaultOpenAIModelsURL
	}
	if cfg.AnthropicModelsURL == "" {
		cfg.AnthropicModelsURL = defaultAnthropicModelsURL
	}
	return cfg
}

type ModelCatalog struct {
	logger slog.Logger
	client *http.Client
	keys   ProviderAPIKeys
	config ModelCatalogConfig
}

func NewModelCatalog(logger slog.Logger, client *http.Client, keys ProviderAPIKeys, config ModelCatalogConfig) *ModelCatalog {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}

	return &ModelCatalog{
		logger: logger.Named("chat-model-catalog"),
		client: client,
		keys:   keys,
		config: config.withDefaults(),
	}
}

func (c *ModelCatalog) ListModels(ctx context.Context) codersdk.ChatModelsResponse {
	filter := parseModelFilter(c.config.Allowlist, c.config.Denylist)

	providers := []string{aiopenai.ProviderName, aianthropic.ProviderName}
	response := codersdk.ChatModelsResponse{
		Providers: make([]codersdk.ChatModelProvider, 0, len(providers)),
	}

	for _, provider := range providers {
		result := codersdk.ChatModelProvider{
			Provider: provider,
			Models:   []codersdk.ChatModel{},
		}

		apiKey := c.keys.apiKey(provider)
		if apiKey == "" {
			result.Available = false
			result.UnavailableReason = codersdk.ChatModelProviderUnavailableMissingAPIKey
			response.Providers = append(response.Providers, result)
			continue
		}

		models, err := c.fetchProviderModels(ctx, provider, apiKey)
		if err != nil {
			c.logger.Warn(ctx, "failed to list provider chat models",
				slog.F("provider", provider),
				slog.Error(err),
			)
			result.Available = false
			result.UnavailableReason = codersdk.ChatModelProviderUnavailableFetchFailed
			response.Providers = append(response.Providers, result)
			continue
		}

		result.Available = true
		result.Models = applyModelFilter(provider, models, filter)
		response.Providers = append(response.Providers, result)
	}

	return response
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
		provider, modelID, err := resolveModelWithProviderHint(model.Model, model.Provider)
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
		Providers: make([]codersdk.ChatModelProvider, 0, 2),
	}

	for _, provider := range []string{aiopenai.ProviderName, aianthropic.ProviderName} {
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

func (c *ModelCatalog) fetchProviderModels(ctx context.Context, provider, apiKey string) ([]codersdk.ChatModel, error) {
	switch normalizeProvider(provider) {
	case aiopenai.ProviderName:
		return c.fetchOpenAIModels(ctx, apiKey)
	case aianthropic.ProviderName:
		return c.fetchAnthropicModels(ctx, apiKey)
	default:
		return nil, xerrors.Errorf("unsupported provider %q", provider)
	}
}

func (c *ModelCatalog) fetchOpenAIModels(ctx context.Context, apiKey string) ([]codersdk.ChatModel, error) {
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		c.config.OpenAIModelsURL,
		nil,
	)
	if err != nil {
		return nil, xerrors.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, xerrors.Errorf("request openai models: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, xerrors.Errorf("openai model API returned status %d", resp.StatusCode)
	}

	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, xerrors.Errorf("decode openai response: %w", err)
	}

	models := make([]codersdk.ChatModel, 0, len(payload.Data))
	seen := make(map[string]struct{}, len(payload.Data))
	for _, item := range payload.Data {
		modelID := strings.TrimSpace(item.ID)
		if modelID == "" || !isChatModelForProvider(aiopenai.ProviderName, modelID) {
			continue
		}
		key := strings.ToLower(modelID)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		models = append(models, newChatModel(aiopenai.ProviderName, modelID, ""))
	}

	sortChatModels(models)
	return models, nil
}

func (c *ModelCatalog) fetchAnthropicModels(ctx context.Context, apiKey string) ([]codersdk.ChatModel, error) {
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		c.config.AnthropicModelsURL,
		nil,
	)
	if err != nil {
		return nil, xerrors.Errorf("create request: %w", err)
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", anthropicAPIVersion)
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, xerrors.Errorf("request anthropic models: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, xerrors.Errorf("anthropic model API returned status %d", resp.StatusCode)
	}

	var payload struct {
		Data []struct {
			ID          string `json:"id"`
			DisplayName string `json:"display_name"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, xerrors.Errorf("decode anthropic response: %w", err)
	}

	models := make([]codersdk.ChatModel, 0, len(payload.Data))
	seen := make(map[string]struct{}, len(payload.Data))
	for _, item := range payload.Data {
		modelID := strings.TrimSpace(item.ID)
		if modelID == "" || !isChatModelForProvider(aianthropic.ProviderName, modelID) {
			continue
		}
		key := strings.ToLower(modelID)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		models = append(models, newChatModel(aianthropic.ProviderName, modelID, item.DisplayName))
	}

	sortChatModels(models)
	return models, nil
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
	return normalizeProvider(provider) + ":" + strings.TrimSpace(modelID)
}

func orderProviders(providerSet map[string]struct{}) []string {
	if len(providerSet) == 0 {
		return nil
	}

	ordered := make([]string, 0, len(providerSet))
	for _, provider := range []string{aiopenai.ProviderName, aianthropic.ProviderName} {
		if _, ok := providerSet[provider]; ok {
			ordered = append(ordered, provider)
		}
	}

	extras := make([]string, 0, len(providerSet))
	for provider := range providerSet {
		if provider == aiopenai.ProviderName || provider == aianthropic.ProviderName {
			continue
		}
		extras = append(extras, provider)
	}
	sort.Strings(extras)

	return append(ordered, extras...)
}

func normalizeProvider(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case aiopenai.ProviderName:
		return aiopenai.ProviderName
	case aianthropic.ProviderName:
		return aianthropic.ProviderName
	default:
		return ""
	}
}

func resolveModel(modelName string) (string, string, error) {
	return resolveModelWithProviderHint(modelName, "")
}

func resolveModelWithProviderHint(modelName, providerHint string) (string, string, error) {
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
		return aianthropic.ProviderName, "claude-opus-4-6", nil
	case "gpt-5.2":
		return aiopenai.ProviderName, "gpt-5.2", nil
	}

	if isChatModelForProvider(aianthropic.ProviderName, normalized) {
		return aianthropic.ProviderName, modelName, nil
	}
	if isChatModelForProvider(aiopenai.ProviderName, normalized) {
		return aiopenai.ProviderName, modelName, nil
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
	case aiopenai.ProviderName:
		return strings.HasPrefix(normalizedModel, "gpt-") ||
			strings.HasPrefix(normalizedModel, "chatgpt-") ||
			isOpenAIReasoningModel(normalizedModel)
	case aianthropic.ProviderName:
		return strings.HasPrefix(normalizedModel, "claude-")
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

type modelFilter struct {
	allow modelFilterSet
	deny  modelFilterSet
}

type modelFilterSet struct {
	any        map[string]struct{}
	byProvider map[string]map[string]struct{}
	enabled    bool
}

func parseModelFilter(allowRaw, denyRaw string) modelFilter {
	return modelFilter{
		allow: parseModelFilterSet(allowRaw),
		deny:  parseModelFilterSet(denyRaw),
	}
}

func parseModelFilterSet(raw string) modelFilterSet {
	set := modelFilterSet{
		any:        map[string]struct{}{},
		byProvider: map[string]map[string]struct{}{},
	}

	for _, token := range strings.Split(raw, ",") {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}

		if provider, modelID, ok := parseCanonicalModelRef(token); ok {
			if set.byProvider[provider] == nil {
				set.byProvider[provider] = map[string]struct{}{}
			}
			set.byProvider[provider][strings.ToLower(modelID)] = struct{}{}
			set.enabled = true
			continue
		}

		set.any[strings.ToLower(token)] = struct{}{}
		set.enabled = true
	}

	return set
}

func (s modelFilterSet) contains(provider, modelRef string) bool {
	if !s.enabled {
		return false
	}

	normalizedModel := strings.ToLower(strings.TrimSpace(modelRef))
	if normalizedModel == "" {
		return false
	}
	if _, ok := s.any[normalizedModel]; ok {
		return true
	}

	provider = normalizeProvider(provider)
	providerModels := s.byProvider[provider]
	if providerModels == nil {
		return false
	}
	_, ok := providerModels[normalizedModel]
	return ok
}

func (s modelFilterSet) matchesModel(provider string, model codersdk.ChatModel) bool {
	return s.contains(provider, model.Model) || s.contains(provider, model.ID)
}

func applyModelFilter(provider string, models []codersdk.ChatModel, filter modelFilter) []codersdk.ChatModel {
	if len(models) == 0 {
		return models
	}

	filtered := make([]codersdk.ChatModel, 0, len(models))
	for _, model := range models {
		if filter.allow.enabled && !filter.allow.matchesModel(provider, model) {
			continue
		}
		if filter.deny.matchesModel(provider, model) {
			continue
		}
		filtered = append(filtered, model)
	}
	return filtered
}
