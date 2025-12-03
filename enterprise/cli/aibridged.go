//go:build !slim

package cli

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/coder/aibridge"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/aibridged"
	"github.com/coder/coder/v2/enterprise/coderd"
)

func newAIBridgeDaemon(coderAPI *coderd.API) (*aibridged.Server, error) {
	ctx := context.Background()
	coderAPI.Logger.Debug(ctx, "starting in-memory aibridge daemon")

	logger := coderAPI.Logger.Named("aibridged")

	// Setup supported providers.
	openai := aibridge.NewProviderConfig(coderAPI.DeploymentValues.AI.BridgeConfig.OpenAI.BaseURL.String(), coderAPI.DeploymentValues.AI.BridgeConfig.OpenAI.Key.String(), "")
	anthropic := aibridge.NewProviderConfig(coderAPI.DeploymentValues.AI.BridgeConfig.Anthropic.BaseURL.String(), coderAPI.DeploymentValues.AI.BridgeConfig.Anthropic.Key.String(), "")
	openai.SetEnableUpstreamLogging(true)
	anthropic.SetEnableUpstreamLogging(true)

	providers := []aibridge.Provider{
		aibridge.NewOpenAIProvider(openai),
		aibridge.NewAnthropicProvider(anthropic,
			getBedrockConfig(coderAPI.DeploymentValues.AI.BridgeConfig.Bedrock)),
	}

	reg := prometheus.WrapRegistererWithPrefix("coder_aibridged_", coderAPI.PrometheusRegistry)
	metrics := aibridge.NewMetrics(reg)

	// Create pool for reusable stateful [aibridge.RequestBridge] instances (one per user).
	pool, err := aibridged.NewCachedBridgePool(aibridged.DefaultPoolOptions, providers, metrics, logger.Named("pool")) // TODO: configurable size.
	if err != nil {
		return nil, xerrors.Errorf("create request pool: %w", err)
	}

	// Create daemon.
	srv, err := aibridged.New(ctx, pool, func(dialCtx context.Context) (aibridged.DRPCClient, error) {
		return coderAPI.CreateInMemoryAIBridgeServer(dialCtx)
	}, logger)
	if err != nil {
		return nil, xerrors.Errorf("start in-memory aibridge daemon: %w", err)
	}
	return srv, nil
}

func getBedrockConfig(cfg codersdk.AIBridgeBedrockConfig) *aibridge.AWSBedrockConfig {
	if cfg.Region.String() == "" && cfg.AccessKey.String() == "" && cfg.AccessKeySecret.String() == "" {
		return nil
	}

	return &aibridge.AWSBedrockConfig{
		Region:          cfg.Region.String(),
		AccessKey:       cfg.AccessKey.String(),
		AccessKeySecret: cfg.AccessKeySecret.String(),
		Model:           cfg.Model.String(),
		SmallFastModel:  cfg.SmallFastModel.String(),
	}
}
