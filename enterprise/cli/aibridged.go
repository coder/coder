//go:build !slim

package cli

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/xerrors"

	"github.com/coder/aibridge"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/aibridged"
	"github.com/coder/coder/v2/enterprise/coderd"
)

func newAIBridgeDaemon(coderAPI *coderd.API) (*aibridged.Server, error) {
	ctx := context.Background()
	coderAPI.Logger.Debug(ctx, "starting in-memory aibridge daemon")

	logger := coderAPI.Logger.Named("aibridged")

	// Setup supported providers.
	providers := []aibridge.Provider{
		aibridge.NewOpenAIProvider(aibridge.OpenAIConfig{
			BaseURL: coderAPI.DeploymentValues.AI.BridgeConfig.OpenAI.BaseURL.String(),
			Key:     coderAPI.DeploymentValues.AI.BridgeConfig.OpenAI.Key.String(),
		}),
		aibridge.NewAnthropicProvider(aibridge.AnthropicConfig{
			BaseURL: coderAPI.DeploymentValues.AI.BridgeConfig.Anthropic.BaseURL.String(),
			Key:     coderAPI.DeploymentValues.AI.BridgeConfig.Anthropic.Key.String(),
		}, getBedrockConfig(coderAPI.DeploymentValues.AI.BridgeConfig.Bedrock)),
	}

	reg := prometheus.WrapRegistererWithPrefix("coder_aibridged_", coderAPI.PrometheusRegistry)
	metrics := aibridge.NewMetrics(reg)
	tracer := coderAPI.TracerProvider.Tracer(tracing.TracerName)

	// Create pool for reusable stateful [aibridge.RequestBridge] instances (one per user).
	pool, err := aibridged.NewCachedBridgePool(aibridged.DefaultPoolOptions, providers, logger.Named("pool"), metrics, tracer) // TODO: configurable size.
	if err != nil {
		return nil, xerrors.Errorf("create request pool: %w", err)
	}

	// Create daemon.
	srv, err := aibridged.New(ctx, pool, func(dialCtx context.Context) (aibridged.DRPCClient, error) {
		return coderAPI.CreateInMemoryAIBridgeServer(dialCtx)
	}, logger, tracer)
	if err != nil {
		return nil, xerrors.Errorf("start in-memory aibridge daemon: %w", err)
	}
	return srv, nil
}

func getBedrockConfig(cfg codersdk.AIBridgeBedrockConfig) *aibridge.AWSBedrockConfig {
	if cfg.Region.String() == "" && cfg.BaseURL.String() == "" && cfg.AccessKey.String() == "" && cfg.AccessKeySecret.String() == "" {
		return nil
	}

	return &aibridge.AWSBedrockConfig{
		BaseURL:         cfg.BaseURL.String(),
		Region:          cfg.Region.String(),
		AccessKey:       cfg.AccessKey.String(),
		AccessKeySecret: cfg.AccessKeySecret.String(),
		Model:           cfg.Model.String(),
		SmallFastModel:  cfg.SmallFastModel.String(),
	}
}
