package chatd

import "github.com/coder/coder/v2/coderd/chatd/chatprovider"

// ProviderAPIKeys contains API keys for provider calls.
type ProviderAPIKeys = chatprovider.ProviderAPIKeys

// ConfiguredProvider is an enabled provider loaded from database config.
type ConfiguredProvider = chatprovider.ConfiguredProvider

type configuredModel = chatprovider.ConfiguredModel
type modelCatalog = chatprovider.ModelCatalog

// SupportedProviders returns all chat providers supported by Fantasy.
func SupportedProviders() []string {
	return chatprovider.SupportedProviders()
}

// IsEnvPresetProvider reports whether provider supports env presets.
func IsEnvPresetProvider(provider string) bool {
	return chatprovider.IsEnvPresetProvider(provider)
}

// ProviderDisplayName returns a default display name for a provider.
func ProviderDisplayName(provider string) string {
	return chatprovider.ProviderDisplayName(provider)
}

// NormalizeProvider canonicalizes a provider name.
func NormalizeProvider(provider string) string {
	return chatprovider.NormalizeProvider(provider)
}

// MergeProviderAPIKeys overlays configured provider keys over fallback keys.
func MergeProviderAPIKeys(fallback ProviderAPIKeys, providers []ConfiguredProvider) ProviderAPIKeys {
	return chatprovider.MergeProviderAPIKeys(fallback, providers)
}

func newModelCatalog(keys ProviderAPIKeys) *modelCatalog {
	return chatprovider.NewModelCatalog(keys)
}

func normalizeProvider(provider string) string {
	return chatprovider.NormalizeProvider(provider)
}

func resolveModelWithProviderHint(modelName, providerHint string) (string, string, error) {
	return chatprovider.ResolveModelWithProviderHint(modelName, providerHint)
}
