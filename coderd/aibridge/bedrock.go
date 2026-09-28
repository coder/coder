package aibridge

import (
	aibridgeconfig "github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
)

// BedrockConfig maps stored provider settings onto the runtime Bedrock
// configuration. It is shared by the gateway, which serves requests with it,
// and by the provider write path, which resolves application inference profile
// ARNs with it.
//
// It returns nil when the settings are absent or when the Bedrock fields are
// not actually configured. The provider's BaseURL is the generic upstream
// endpoint and is always non-empty, so it cannot serve as a Bedrock detection
// signal; gate on the settings alone via
// [codersdk.AIProviderBedrockSettings.IsConfigured].
func BedrockConfig(baseURL string, bedrock *codersdk.AIProviderBedrockSettings) *aibridgeconfig.AWSBedrock {
	if bedrock == nil {
		return nil
	}
	settings := *bedrock
	if !settings.IsConfigured() {
		return nil
	}
	return &aibridgeconfig.AWSBedrock{
		BaseURL:                baseURL,
		Region:                 settings.Region,
		AccessKey:              ptr.NilToEmpty(settings.AccessKey),
		AccessKeySecret:        ptr.NilToEmpty(settings.AccessKeySecret),
		Model:                  settings.Model,
		SmallFastModel:         settings.SmallFastModel,
		RoleARN:                settings.RoleARN,
		ExternalID:             settings.ExternalID,
		Protocol:               aibridgeconfig.BedrockProtocol(settings.ResolvedProtocol()),
		ResolvedModel:          settings.ResolvedModel,
		ResolvedSmallFastModel: settings.ResolvedSmallFastModel,
	}
}
