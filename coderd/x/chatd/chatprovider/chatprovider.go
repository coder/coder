package chatprovider

import (
	"context"
	"fmt"
	"net/http"
	"slices"
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
	"github.com/openai/openai-go/v3/option"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/x/chatd/chatopenai"
	"github.com/coder/coder/v2/coderd/x/chatd/chatutil"
	"github.com/coder/coder/v2/coderd/x/chatfiles"
	"github.com/coder/coder/v2/codersdk"
)

var providerDisplayNameByName = map[string]string{
	fantasyanthropic.Name:    "Anthropic",
	fantasyazure.Name:        "Azure OpenAI",
	fantasybedrock.Name:      "AWS Bedrock",
	fantasygoogle.Name:       "Google",
	fantasyopenai.Name:       "OpenAI",
	fantasyopenaicompat.Name: "OpenAI Compatible",
	fantasyopenrouter.Name:   "OpenRouter",
	fantasyvercel.Name:       "Vercel AI Gateway",
	// Copilot is unsupported but still needs a display name for the
	// unsupported list and AI Settings.
	string(codersdk.AIProviderTypeCopilot): "GitHub Copilot",
}

// ProviderDisplayName returns a default display name for a provider.
func ProviderDisplayName(provider string) string {
	normalized := NormalizeProvider(provider)
	if normalized == "" {
		// Fall back for providers the harness cannot normalize, like copilot.
		normalized = strings.ToLower(strings.TrimSpace(provider))
	}
	if displayName, ok := providerDisplayNameByName[normalized]; ok {
		return displayName
	}
	return normalized
}

// AgentsSupportsProvider reports whether the Agents harness can use the
// provider type.
func AgentsSupportsProvider(provider string) bool {
	providerType := codersdk.AIProviderType(strings.ToLower(strings.TrimSpace(provider)))
	if codersdk.IsAgentsUnsupportedProviderType(providerType) {
		return false
	}
	return NormalizeProvider(provider) != ""
}

// UnsupportedProviders returns the configured providers the Agents harness
// cannot use, deduplicated by provider type.
func UnsupportedProviders(configured []ConfiguredProvider) []codersdk.ChatUnsupportedProvider {
	seen := make(map[string]struct{}, len(configured))
	unsupported := make([]codersdk.ChatUnsupportedProvider, 0)
	for _, provider := range configured {
		if AgentsSupportsProvider(provider.Provider) {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(provider.Provider))
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		unsupported = append(unsupported, codersdk.ChatUnsupportedProvider{
			Provider:    key,
			DisplayName: ProviderDisplayName(provider.Provider),
		})
	}
	slices.SortFunc(unsupported, func(a, b codersdk.ChatUnsupportedProvider) int {
		return strings.Compare(a.Provider, b.Provider)
	})
	return unsupported
}

// InlineImageCapBytes returns the per-image byte cap for inline
// image parts, or (0, false) when no documented cap applies.
// Bedrock shares Anthropic's cap because its Anthropic models use the Anthropic client.
func InlineImageCapBytes(provider string) (int, bool) {
	switch NormalizeProvider(provider) {
	case fantasyanthropic.Name, fantasybedrock.Name:
		return codersdk.AnthropicInlineImageCapBytes, true
	default:
		return 0, false
	}
}

// ToolResultMediaOmission reports media unsupported by the transport or upstream
// provider. configuredProvider supplies vendor limits hidden by a shared
// OpenAI-compatible transport.
func ToolResultMediaOmission(transportProvider, configuredProvider, mediaType string, size int) (string, bool) {
	provider := NormalizeProvider(transportProvider)
	if provider == fantasyopenaicompat.Name {
		provider = NormalizeProvider(configuredProvider)
	}
	baseType := chatfiles.BaseMediaType(mediaType)
	isImage := strings.HasPrefix(baseType, "image/")
	accepted := true
	switch provider {
	case fantasyanthropic.Name, fantasybedrock.Name:
		accepted = slices.Contains([]string{"image/jpeg", "image/png", "image/gif", "image/webp"}, baseType)
	case fantasyopenai.Name, fantasyazure.Name:
		accepted = !isImage || slices.Contains([]string{"image/jpeg", "image/png", "image/gif", "image/webp"}, baseType)
	case fantasygoogle.Name:
		accepted = !isImage || slices.Contains([]string{"image/jpeg", "image/png", "image/webp", "image/heic", "image/heif"}, baseType)
	}
	if !accepted {
		return fmt.Sprintf("[%s content omitted: unsupported tool result media type]", mediaType), true
	}
	if imageCap, hasCap := InlineImageCapBytes(provider); hasCap && size >= imageCap {
		return fmt.Sprintf("[image omitted: %d bytes exceeds the inline image limit of %d bytes]", size, imageCap), true
	}
	return "", false
}

// AcceptsFilePartMediaType reports whether m's provider accepts mediaType as a
// file content part rather than silently dropping it. Callers replace rejected
// parts with text, so a false negative costs fidelity while a false positive
// loses the attachment entirely. Unknown providers therefore return false.
func (m Model) AcceptsFilePartMediaType(mediaType string) bool {
	baseType := chatfiles.BaseMediaType(mediaType)
	// No provider accepts SVG as a native part; it is inlined as text.
	if baseType == string(codersdk.ChatAttachmentMediaTypeImageSVG) {
		return false
	}
	isImage := strings.HasPrefix(baseType, "image/")
	isText := strings.HasPrefix(baseType, "text/")
	// Audio types are included for matrix completeness but are not
	// currently reachable: no audio type is in the storable attachment
	// allowlist (codersdk.AllChatAttachmentMediaTypes).
	isAudio := baseType == "audio/wav" || baseType == "audio/mpeg" || baseType == "audio/mp3"
	isPDF := baseType == "application/pdf"

	switch NormalizeProvider(m.Provider()) {
	case fantasyanthropic.Name:
		return isImage || isText || isPDF
	case fantasyopenai.Name:
		// Chat Completions accepts text and audio as native file parts.
		if m.transport.UsesResponses() {
			return isImage || isPDF
		}
		return isImage || isText || isAudio || isPDF
	case fantasyopenaicompat.Name:
		return isImage || isText || isAudio || isPDF
	default:
		return false
	}
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
	// SupportsAmbientCredentials reports that the provider configuration supports
	// ambient credentials. It does not verify that credentials are available.
	SupportsAmbientCredentials bool
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

// mergedFromFallback keeps both maps non-nil so callers can add providers and
// mirrors legacy OpenAI/Anthropic keys into ByProvider for provider-name lookups.
func mergedFromFallback(fallback ProviderAPIKeys) ProviderAPIKeys {
	merged := ProviderAPIKeys{
		OpenAI:            strings.TrimSpace(fallback.OpenAI),
		Anthropic:         strings.TrimSpace(fallback.Anthropic),
		ByProvider:        map[string]string{},
		BaseURLByProvider: map[string]string{},
	}
	for provider, apiKey := range fallback.ByProvider {
		if normalized := NormalizeProvider(provider); normalized != "" {
			if key := strings.TrimSpace(apiKey); key != "" {
				merged.ByProvider[normalized] = key
			}
		}
	}
	for provider, baseURL := range fallback.BaseURLByProvider {
		if normalized := NormalizeProvider(provider); normalized != "" {
			if url := strings.TrimSpace(baseURL); url != "" {
				merged.BaseURLByProvider[normalized] = url
			}
		}
	}
	if merged.OpenAI != "" {
		merged.ByProvider[fantasyopenai.Name] = merged.OpenAI
	}
	if merged.Anthropic != "" {
		merged.ByProvider[fantasyanthropic.Name] = merged.Anthropic
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
	merged := mergedFromFallback(fallback)

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
		case (normalizedProvider == fantasybedrock.Name || provider.SupportsAmbientCredentials) && provider.CentralAPIKeyEnabled:
			// These provider configurations support ambient credentials without an
			// explicit key, but only when the credential policy allows central
			// credentials to satisfy the request.
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
	if trimmedKey != "" || availability.Available {
		keys.ByProvider[normalizedProvider] = trimmedKey
	}
}

// PruneDisabledProviderKeys removes keys and base URLs for disabled providers,
// including the legacy OpenAI and Anthropic key fields.
func PruneDisabledProviderKeys(keys *ProviderAPIKeys, enabledProviders map[string]struct{}) {
	for provider := range keys.ByProvider {
		if _, ok := enabledProviders[provider]; ok {
			continue
		}
		delete(keys.ByProvider, provider)
	}
	for provider := range keys.BaseURLByProvider {
		if _, ok := enabledProviders[provider]; ok {
			continue
		}
		delete(keys.BaseURLByProvider, provider)
	}
	if _, ok := enabledProviders[NormalizeProvider("openai")]; !ok {
		keys.OpenAI = ""
	}
	if _, ok := enabledProviders[NormalizeProvider("anthropic")]; !ok {
		keys.Anthropic = ""
	}
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

	// A valid provider hint is authoritative, so preserve the model ID
	// instead of interpreting its namespace as a different provider.
	if provider := NormalizeProvider(providerHint); provider != "" {
		return provider, modelName, nil
	}

	if provider, modelID, ok := parseCanonicalModelRef(modelName); ok {
		return provider, modelID, nil
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
	if isChatModelForProvider(fantasygoogle.Name, normalized) {
		return fantasygoogle.Name, modelName, nil
	}

	return "", "", xerrors.Errorf("unknown model %q", modelName)
}

func parseCanonicalModelRef(modelRef string) (provider string, model string, ok bool) {
	modelRef = strings.TrimSpace(modelRef)
	if modelRef == "" {
		return "", "", false
	}

	for _, separator := range []string{":", "/"} {
		before, after, found := strings.Cut(modelRef, separator)
		if !found {
			continue
		}

		provider := NormalizeProvider(before)
		modelID := strings.TrimSpace(after)
		if provider != "" && modelID != "" {
			return provider, modelID, true
		}
	}

	return "", "", false
}

// IsAnthropicFamilyModelID reports whether a model ID names a Claude
// model regardless of the transport it is served through: OpenRouter and
// Vercel namespaces ("anthropic/claude-haiku-4.5"), Bedrock IDs
// ("anthropic.claude-...", "us.anthropic.claude-..."), and bare Claude
// IDs on generic OpenAI-compatible gateways.
func IsAnthropicFamilyModelID(modelID string) bool {
	normalized := strings.ToLower(strings.TrimSpace(modelID))
	return strings.HasPrefix(normalized, "anthropic/") ||
		strings.HasPrefix(normalized, "anthropic.") ||
		strings.Contains(normalized, ".anthropic.") ||
		strings.Contains(normalized, "claude")
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

// AnthropicThinkingDisplayFromChat normalizes chat-config thinking display
// values for Anthropic and returns the canonical provider display value.
func AnthropicThinkingDisplayFromChat(value *string) *fantasyanthropic.ThinkingDisplay {
	if value == nil {
		return nil
	}

	normalized := strings.ToLower(strings.TrimSpace(*value))
	if normalized == "" {
		return nil
	}

	display := chatutil.NormalizedEnumValue(
		normalized,
		string(fantasyanthropic.ThinkingDisplaySummarized),
		string(fantasyanthropic.ThinkingDisplayOmitted),
	)
	if display == nil {
		return nil
	}
	valueCopy := fantasyanthropic.ThinkingDisplay(*display)
	return &valueCopy
}

// GoogleThinkingLevelFromChat normalizes chat-config thinking level values
// for Google and returns the canonical provider level value.
func GoogleThinkingLevelFromChat(value *string) *fantasygoogle.ThinkingLevel {
	if value == nil {
		return nil
	}

	normalized := strings.ToLower(strings.TrimSpace(*value))
	if normalized == "" {
		return nil
	}

	return chatutil.NormalizedEnumValue(
		normalized,
		fantasygoogle.ThinkingLevelMinimal,
		fantasygoogle.ThinkingLevelLow,
		fantasygoogle.ThinkingLevelMedium,
		fantasygoogle.ThinkingLevelHigh,
	)
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

// AnthropicBetaContext1M is the beta token for Anthropic's 1M context window.
const AnthropicBetaContext1M = "context-1m-2025-08-07"

// HeaderAnthropicBeta names Anthropic's beta feature header.
const HeaderAnthropicBeta = "Anthropic-Beta"

// BetaHeadersFromCallConfig returns beta feature headers for Anthropic calls.
func BetaHeadersFromCallConfig(providerName string, config *codersdk.ChatModelCallConfig) map[string]string {
	if config == nil || config.ProviderOptions == nil || config.ProviderOptions.Anthropic == nil {
		return nil
	}
	enabled := config.ProviderOptions.Anthropic.Context1MEnabled
	if enabled == nil || !*enabled {
		return nil
	}
	if NormalizeProvider(providerName) != fantasyanthropic.Name {
		return nil
	}
	return map[string]string{HeaderAnthropicBeta: AnthropicBetaContext1M}
}

// openAIResponsesAPIOverride returns the configured OpenAI Responses API
// override, or nil when the model config leaves the choice to the provider
// SDK's known-model list. It stays unexported so the decision is reachable
// only from client construction.
func openAIResponsesAPIOverride(config *codersdk.ChatModelOpenAIConfig) *bool {
	if config == nil {
		return nil
	}
	return config.UseResponsesAPI
}

// effectiveOpenAIConfig fills the Responses API and reasoning-model overrides
// a configured reasoning mode implies when the config leaves them unset,
// because the SDK's known-model list cannot vouch for an alias it has never seen.
func effectiveOpenAIConfig(callConfig *codersdk.ChatModelCallConfig) *codersdk.ChatModelOpenAIConfig {
	if callConfig == nil {
		return nil
	}
	if chatopenai.ReasoningMode(callConfig) == nil {
		return callConfig.OpenAIConfig
	}
	var config codersdk.ChatModelOpenAIConfig
	if callConfig.OpenAIConfig != nil {
		config = *callConfig.OpenAIConfig
	}
	if config.UseResponsesAPI == nil {
		config.UseResponsesAPI = ptr.Ref(true)
	}
	if config.ReasoningModel == nil {
		config.ReasoningModel = ptr.Ref(true)
	}
	return &config
}

// ModelFromConfig resolves a provider/model pair and constructs a fantasy
// language model client using the provided provider credentials. The
// userAgent is sent as the User-Agent header on every outgoing LLM
// API request. extraHeaders, when non-nil, are sent as additional
// HTTP headers on every request. httpClient, when non-nil, is used for
// all provider HTTP requests. callConfig carries the model's client-scoped
// OpenAI settings applied here: the transport override and reasoning mode.
func ModelFromConfig(
	providerHint string,
	modelName string,
	providerKeys ProviderAPIKeys,
	userAgent string,
	extraHeaders map[string]string,
	httpClient *http.Client,
	callConfig *codersdk.ChatModelCallConfig,
) (Model, error) {
	provider, modelID, err := ResolveModelWithProviderHint(modelName, providerHint)
	if err != nil {
		return Model{}, err
	}

	if err := chatopenai.ValidateReasoningMode(provider, callConfig); err != nil {
		return Model{}, err
	}
	openAIConfig := effectiveOpenAIConfig(callConfig)

	apiKey := providerKeys.APIKey(provider)
	if apiKey == "" {
		return Model{}, missingProviderAPIKeyError(provider)
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
	case fantasyopenai.Name:
		// Resolved once here so a later mutation of the config cannot move
		// the client off the transport NewModel records.
		useResponses := chatopenai.UsesResponsesAPI(modelID, openAIResponsesAPIOverride(openAIConfig))
		options := []fantasyopenai.Option{
			fantasyopenai.WithAPIKey(apiKey),
			fantasyopenai.WithUseResponsesAPI(),
			fantasyopenai.WithResponsesAPIFunc(func(string) bool { return useResponses }),
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
		if openAIConfig != nil && openAIConfig.ReasoningModel != nil {
			reasoningModel := *openAIConfig.ReasoningModel
			options = append(options, fantasyopenai.WithReasoningModelFunc(func(string) bool { return reasoningModel }))
		}
		// The pinned SDK reinterprets pre-serialization overlay keys as JSON paths.
		// Escape the dot so its merge adds mode without replacing reasoning.
		if mode := chatopenai.ReasoningMode(callConfig); mode != nil {
			options = append(options, fantasyopenai.WithSDKOptions(option.WithJSONSet(`reasoning\.mode`, *mode)))
		}
		providerClient, err = fantasyopenai.New(options...)
	case fantasyopenaicompat.Name:
		httpClient = withOpenAICompatRequestPatches(httpClient, baseURL, modelID)
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
	default:
		return Model{}, xerrors.Errorf("unsupported model provider %q", provider)
	}
	if err != nil {
		return Model{}, xerrors.Errorf("create %s provider: %w", provider, err)
	}

	model, err := providerClient.LanguageModel(context.Background(), modelID)
	if err != nil {
		return Model{}, xerrors.Errorf("load %s model: %w", provider, err)
	}
	return NewModel(model, openAIConfig), nil
}

func missingProviderAPIKeyError(provider string) error {
	switch provider {
	case fantasyanthropic.Name:
		return xerrors.New("ANTHROPIC_API_KEY is not set")
	case fantasyopenai.Name:
		return xerrors.New("OPENAI_API_KEY is not set")
	case fantasyopenaicompat.Name:
		return xerrors.New("OPENAI_COMPAT_API_KEY is not set")
	default:
		return xerrors.Errorf("API key for provider %q is not set", provider)
	}
}

// ProviderOptionsForCall builds the provider options for one inference call.
// Config conversion and reasoning effort both create OpenAI option structs, so
// owning them together is what keeps their type aligned with the model's
// transport. requestedEffort is the caller's per-turn choice, which the
// config's bounds clamp.
func ProviderOptionsForCall(
	model Model,
	config codersdk.ChatModelCallConfig,
	requestedEffort *string,
) fantasy.ProviderOptions {
	options := providerOptionsFromChatModelConfig(model, config.ProviderOptions)
	effort := ResolveReasoningEffort(requestedEffort, config.ReasoningEffort)
	options = applyReasoningEffort(model, options, effort)
	return ensureOpenAIResponsesIncludes(model, options)
}

// ensureOpenAIResponsesIncludes requests encrypted reasoning on every
// Responses call, so reasoning can be replayed without provider-side storage.
// An options block created here leaves Store unset, keeping the provider's
// default.
func ensureOpenAIResponsesIncludes(
	model Model,
	options fantasy.ProviderOptions,
) fantasy.ProviderOptions {
	if !model.Valid() || !model.transport.UsesResponses() {
		return options
	}
	if options == nil {
		options = fantasy.ProviderOptions{}
	}
	responsesOptions := ensureProviderOptions[fantasyopenai.ResponsesProviderOptions](options, fantasyopenai.Name)
	responsesOptions.Include = chatopenai.EnsureResponseIncludes(responsesOptions.Include)
	return options
}

func providerOptionsFromChatModelConfig(
	model Model,
	options *codersdk.ChatModelProviderOptions,
) fantasy.ProviderOptions {
	if options == nil {
		return nil
	}

	result := fantasy.ProviderOptions{}

	if options.OpenAI != nil {
		result[fantasyopenai.Name] = chatopenai.ProviderOptionsFromChatConfig(
			model.transport,
			options.OpenAI,
		)
	}
	if options.Anthropic != nil {
		result[fantasyanthropic.Name] = anthropicProviderOptionsFromChatConfig(
			options.Anthropic,
		)
	}
	if options.OpenAICompat != nil {
		result[fantasyopenaicompat.Name] = openAICompatProviderOptionsFromChatConfig(
			options.OpenAICompat,
		)
	}

	// Google models route through the OpenAI-compatible client, so a pinned
	// thinking configuration travels as the compat request's extra_body for
	// the transport patch to honor it.
	if options.Google != nil && options.Google.ThinkingConfig != nil &&
		model.Valid() && NormalizeProvider(model.Provider()) == fantasyopenaicompat.Name {
		if extraBody := googleCompatExtraBodyFromThinkingConfig(model.ModelID(), options.Google.ThinkingConfig); extraBody != nil {
			compatOptions := ensureProviderOptions[fantasyopenaicompat.ProviderOptions](result, fantasyopenaicompat.Name)
			compatOptions.ExtraBody = extraBody
		}
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
		ThinkingDisplay:        AnthropicThinkingDisplayFromChat(options.ThinkingDisplay),
		DisableParallelToolUse: options.DisableParallelToolUse,
	}
	if options.Thinking != nil && options.Thinking.BudgetTokens != nil {
		result.Thinking = &fantasyanthropic.ThinkingProviderOption{
			BudgetTokens: *options.Thinking.BudgetTokens,
		}
	}
	return result
}

func openAICompatProviderOptionsFromChatConfig(
	options *codersdk.ChatModelOpenAICompatProviderOptions,
) *fantasyopenaicompat.ProviderOptions {
	return &fantasyopenaicompat.ProviderOptions{
		User: chatutil.NormalizedStringPointer(options.User),
	}
}
