package provider

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/aibridge/config"
)

func TestIsApplicationInferenceProfileARN(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		model string
		want  bool
	}{
		{
			name:  "application inference profile arn",
			model: "arn:aws:bedrock:eu-west-2:123456789012:application-inference-profile/46u2vhiyo6z5",
			want:  true,
		},
		{
			name:  "plain model id",
			model: "anthropic.claude-opus-4-8",
			want:  false,
		},
		{
			name:  "regional model id",
			model: "eu.anthropic.claude-opus-4-8",
			want:  false,
		},
		{
			// System-defined inference profiles carry the model ID, so they need
			// no resolution and must not require the extra AWS permission.
			name:  "system defined inference profile arn",
			model: "arn:aws:bedrock:us-east-1:123456789012:inference-profile/us.anthropic.claude-opus-4-8",
			want:  false,
		},
		{
			name:  "foundation model arn",
			model: "arn:aws:bedrock:us-east-1::foundation-model/anthropic.claude-opus-4-8",
			want:  false,
		},
		{
			name:  "non bedrock arn",
			model: "arn:aws:iam::123456789012:role/BedrockRole",
			want:  false,
		},
		{
			name:  "malformed arn",
			model: "arn:aws:bedrock:broken",
			want:  false,
		},
		{
			name:  "resource without type separator",
			model: "arn:aws:bedrock:us-east-1:123456789012:application-inference-profile",
			want:  false,
		},
		{
			name:  "empty",
			model: "",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.want, isApplicationInferenceProfileARN(tt.model))
		})
	}
}

func TestModelIDFromARN(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		modelARN string
		want     string
		errorMsg string
	}{
		{
			name:     "foundation model",
			modelARN: "arn:aws:bedrock:us-east-1::foundation-model/anthropic.claude-opus-4-8",
			want:     "anthropic.claude-opus-4-8",
		},
		{
			name:     "system defined inference profile",
			modelARN: "arn:aws:bedrock:us-east-1:123456789012:inference-profile/us.anthropic.claude-opus-4-8",
			want:     "us.anthropic.claude-opus-4-8",
		},
		{
			name:     "not an arn",
			modelARN: "anthropic.claude-opus-4-8",
			errorMsg: "parse model arn",
		},
		{
			name:     "no model identifier",
			modelARN: "arn:aws:bedrock:us-east-1::foundation-model",
			errorMsg: "has no model identifier",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := modelIDFromARN(tt.modelARN)
			if tt.errorMsg != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.errorMsg)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestResolveBedrockModels(t *testing.T) {
	t.Parallel()

	const (
		profileARN          = "arn:aws:bedrock:eu-west-2:123456789012:application-inference-profile/46u2vhiyo6z5"
		smallFastProfileARN = "arn:aws:bedrock:eu-west-2:123456789012:application-inference-profile/8x1qk20fzp3r"
	)

	t.Run("plain model ids are not resolved", func(t *testing.T) {
		t.Parallel()

		cfg := config.AWSBedrock{
			Model:          "eu.anthropic.claude-opus-4-8",
			SmallFastModel: "anthropic.claude-haiku-4-5",
		}
		resolve := func(context.Context, config.AWSBedrock, aws.CredentialsProvider, string) (string, error) {
			t.Error("resolver called for a plain model id")
			return "", nil
		}

		model, smallFastModel, err := resolveBedrockModels(context.Background(), cfg, nil, resolve)
		require.NoError(t, err)
		require.Equal(t, cfg.Model, model)
		require.Equal(t, cfg.SmallFastModel, smallFastModel)
	})

	t.Run("profile arns are resolved", func(t *testing.T) {
		t.Parallel()

		cfg := config.AWSBedrock{
			Model:          profileARN,
			SmallFastModel: smallFastProfileARN,
		}
		resolved := map[string]string{
			profileARN:          "anthropic.claude-opus-4-8",
			smallFastProfileARN: "anthropic.claude-haiku-4-5",
		}
		resolve := func(_ context.Context, _ config.AWSBedrock, _ aws.CredentialsProvider, arn string) (string, error) {
			return resolved[arn], nil
		}

		model, smallFastModel, err := resolveBedrockModels(context.Background(), cfg, nil, resolve)
		require.NoError(t, err)
		require.Equal(t, "anthropic.claude-opus-4-8", model)
		require.Equal(t, "anthropic.claude-haiku-4-5", smallFastModel)
	})

	t.Run("small fast model resolves independently", func(t *testing.T) {
		t.Parallel()

		cfg := config.AWSBedrock{
			Model:          "eu.anthropic.claude-opus-4-8",
			SmallFastModel: smallFastProfileARN,
		}
		var calls []string
		resolve := func(_ context.Context, _ config.AWSBedrock, _ aws.CredentialsProvider, arn string) (string, error) {
			calls = append(calls, arn)
			return "anthropic.claude-haiku-4-5", nil
		}

		model, smallFastModel, err := resolveBedrockModels(context.Background(), cfg, nil, resolve)
		require.NoError(t, err)
		require.Equal(t, []string{smallFastProfileARN}, calls)
		require.Equal(t, cfg.Model, model)
		require.Equal(t, "anthropic.claude-haiku-4-5", smallFastModel)
	})

	t.Run("resolution failure is returned", func(t *testing.T) {
		t.Parallel()

		cfg := config.AWSBedrock{
			Model:          profileARN,
			SmallFastModel: "anthropic.claude-haiku-4-5",
		}
		resolve := func(context.Context, config.AWSBedrock, aws.CredentialsProvider, string) (string, error) {
			return "", xerrors.New("AccessDeniedException")
		}

		_, _, err := resolveBedrockModels(context.Background(), cfg, nil, resolve)
		require.Error(t, err)
		require.Contains(t, err.Error(), "resolve model")
		require.Contains(t, err.Error(), "AccessDeniedException")
	})
}

func TestNewAnthropic_InferenceProfileResolution(t *testing.T) {
	t.Parallel()

	const profileARN = "arn:aws:bedrock:eu-west-2:123456789012:application-inference-profile/46u2vhiyo6z5"

	bedrockCfg := func(model string) *config.AWSBedrock {
		return &config.AWSBedrock{
			Region:          "eu-west-2",
			AccessKey:       "test-key",
			AccessKeySecret: "test-secret",
			Model:           model,
			SmallFastModel:  "anthropic.claude-haiku-4-5",
		}
	}

	t.Run("resolved profile drives the model id", func(t *testing.T) {
		t.Parallel()

		resolve := func(_ context.Context, _ config.AWSBedrock, _ aws.CredentialsProvider, arn string) (string, error) {
			require.Equal(t, profileARN, arn)
			return "anthropic.claude-opus-4-8", nil
		}

		p, err := NewAnthropic(context.Background(), config.Anthropic{}, bedrockCfg(profileARN), withInferenceProfileResolver(resolve))
		require.NoError(t, err)
		require.Equal(t, "anthropic.claude-opus-4-8", p.bedrock.ResolvedModel())
		// The profile stays the configured identifier so AWS attributes spend to it.
		require.Equal(t, profileARN, p.bedrock.ConfiguredModel())
		require.Equal(t, "anthropic.claude-haiku-4-5", p.bedrock.ResolvedSmallFastModel())
	})

	t.Run("failed resolution fails construction", func(t *testing.T) {
		t.Parallel()

		resolve := func(context.Context, config.AWSBedrock, aws.CredentialsProvider, string) (string, error) {
			return "", xerrors.New("AccessDeniedException")
		}

		_, err := NewAnthropic(context.Background(), config.Anthropic{}, bedrockCfg(profileARN), withInferenceProfileResolver(resolve))
		require.ErrorContains(t, err, "resolve bedrock models")
	})

	t.Run("plain model id needs no resolution", func(t *testing.T) {
		t.Parallel()

		resolve := func(context.Context, config.AWSBedrock, aws.CredentialsProvider, string) (string, error) {
			t.Error("resolver called for a plain model id")
			return "", nil
		}

		p, err := NewAnthropic(context.Background(), config.Anthropic{}, bedrockCfg("eu.anthropic.claude-opus-4-8"), withInferenceProfileResolver(resolve))
		require.NoError(t, err)
		require.Equal(t, "eu.anthropic.claude-opus-4-8", p.bedrock.ResolvedModel())
		require.Equal(t, "eu.anthropic.claude-opus-4-8", p.bedrock.ConfiguredModel())
	})
}
