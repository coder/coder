package aibridge

import (
	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
)

// BedrockConfigFromSettings converts codersdk bedrock settings + the parent
// provider base URL into the aibridge runtime config.AWSBedrock. Returns the
// zero value and ok=false when the settings are absent or not configured.
//
// The provider's BaseURL is the generic upstream endpoint and is always
// non-empty, so it cannot serve as a Bedrock detection signal; gating is on
// the settings alone via [codersdk.AIProviderBedrockSettings.IsConfigured].
func BedrockConfigFromSettings(baseURL string, bedrock *codersdk.AIProviderBedrockSettings) (config.AWSBedrock, bool) {
	if bedrock == nil {
		return config.AWSBedrock{}, false
	}
	s := *bedrock
	if !s.IsConfigured() {
		return config.AWSBedrock{}, false
	}
	return config.AWSBedrock{
		BaseURL:         baseURL,
		Region:          s.Region,
		AccessKey:       ptr.NilToEmpty(s.AccessKey),
		AccessKeySecret: ptr.NilToEmpty(s.AccessKeySecret),
		Model:           s.Model,
		SmallFastModel:  s.SmallFastModel,
		RoleARN:         s.RoleARN,
		ExternalID:      s.ExternalID,
		Protocol:        config.BedrockProtocol(s.ResolvedProtocol()),
	}, true
}
