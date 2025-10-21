package aibridge

import (
	"sync/atomic"

	"github.com/coder/aibridge"
)

var (
	upstreamLoggingEnabled atomic.Bool
	providerConfigs        []*aibridge.ProviderConfig
)

// SetProviderConfigs stores the provider configs so they can be updated at runtime.
func SetProviderConfigs(configs []*aibridge.ProviderConfig) {
	providerConfigs = configs
}

// SetUpstreamLoggingEnabled sets whether upstream request/response logging is enabled
// and updates all registered provider configs.
func SetUpstreamLoggingEnabled(enabled bool) {
	upstreamLoggingEnabled.Store(enabled)
	// Update all provider configs.
	for _, cfg := range providerConfigs {
		if cfg != nil {
			cfg.SetEnableUpstreamLogging(enabled)
		}
	}
}
