package config_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge/config"
)

func TestAWSBedrockValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		cfg      config.AWSBedrock
		errorMsg string
	}{
		{
			name: "invoke model valid",
			cfg: config.AWSBedrock{
				Region:         "us-east-1",
				Model:          "anthropic.claude-sonnet",
				SmallFastModel: "anthropic.claude-haiku",
			},
		},
		{
			name: "invoke model valid with base url instead of region",
			cfg: config.AWSBedrock{
				BaseURL:        "https://bedrock-runtime.example.com",
				Model:          "anthropic.claude-sonnet",
				SmallFastModel: "anthropic.claude-haiku",
			},
		},
		{
			name: "invoke model missing region and base url",
			cfg: config.AWSBedrock{
				Model:          "anthropic.claude-sonnet",
				SmallFastModel: "anthropic.claude-haiku",
			},
			errorMsg: "region or base url required",
		},
		{
			name: "invoke model missing model",
			cfg: config.AWSBedrock{
				Region:         "us-east-1",
				SmallFastModel: "anthropic.claude-haiku",
			},
			errorMsg: "model required",
		},
		{
			name: "invoke model missing small fast model",
			cfg: config.AWSBedrock{
				Region: "us-east-1",
				Model:  "anthropic.claude-sonnet",
			},
			errorMsg: "small fast model required",
		},
		{
			name: "unknown protocol rejected",
			cfg: config.AWSBedrock{
				Protocol: config.BedrockProtocol("unknown"),
			},
			errorMsg: "unknown bedrock protocol",
		},
		{
			name: "mantle valid official api prefix",
			cfg: config.AWSBedrock{
				Region:   "us-east-1",
				BaseURL:  "https://bedrock-mantle.us-east-1.api.aws/anthropic",
				Protocol: config.BedrockProtocolMantle,
			},
		},
		{
			name: "mantle valid proxy api prefix",
			cfg: config.AWSBedrock{
				Region:   "us-east-1",
				BaseURL:  "https://proxy.internal/proxy",
				Protocol: config.BedrockProtocolMantle,
			},
		},
		{
			name: "mantle missing region",
			cfg: config.AWSBedrock{
				BaseURL:  "https://bedrock-mantle.us-east-1.api.aws",
				Protocol: config.BedrockProtocolMantle,
			},
			errorMsg: "region required",
		},
		{
			name: "mantle missing base url",
			cfg: config.AWSBedrock{
				Region:   "us-east-1",
				Protocol: config.BedrockProtocolMantle,
			},
			errorMsg: "base_url required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.cfg.Validate()
			if tt.errorMsg != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.errorMsg)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestAWSClaudePlatformValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		cfg      config.AWSClaudePlatform
		errorMsg string
	}{
		{
			name: "iam valid with default credential chain",
			cfg: config.AWSClaudePlatform{
				AuthMode:    config.ClaudePlatformAuthModeIAM,
				Region:      "us-east-1",
				WorkspaceID: "wrkspc_123",
			},
		},
		{
			name: "iam valid with static credentials and role",
			cfg: config.AWSClaudePlatform{
				AuthMode:        config.ClaudePlatformAuthModeIAM,
				Region:          "us-east-1",
				WorkspaceID:     "wrkspc_123",
				AccessKey:       "AKIA",
				AccessKeySecret: "secret",
				RoleARN:         "arn:aws:iam::123456789012:role/claude",
				ExternalID:      "coder-external-id",
			},
		},
		{
			name: "api key valid",
			cfg: config.AWSClaudePlatform{
				AuthMode:    config.ClaudePlatformAuthModeAPIKey,
				Region:      "us-east-1",
				WorkspaceID: "wrkspc_123",
			},
		},
		{
			name: "missing region",
			cfg: config.AWSClaudePlatform{
				AuthMode:    config.ClaudePlatformAuthModeIAM,
				WorkspaceID: "wrkspc_123",
			},
			errorMsg: "region required",
		},
		{
			name: "missing workspace id",
			cfg: config.AWSClaudePlatform{
				AuthMode: config.ClaudePlatformAuthModeIAM,
				Region:   "us-east-1",
			},
			errorMsg: "workspace id required",
		},
		{
			name: "missing auth mode",
			cfg: config.AWSClaudePlatform{
				Region:      "us-east-1",
				WorkspaceID: "wrkspc_123",
			},
			errorMsg: "auth mode required",
		},
		{
			name: "unknown auth mode",
			cfg: config.AWSClaudePlatform{
				AuthMode:    config.ClaudePlatformAuthMode("sigv2"),
				Region:      "us-east-1",
				WorkspaceID: "wrkspc_123",
			},
			errorMsg: "unknown claude platform auth mode",
		},
		{
			name: "iam access key without secret",
			cfg: config.AWSClaudePlatform{
				AuthMode:    config.ClaudePlatformAuthModeIAM,
				Region:      "us-east-1",
				WorkspaceID: "wrkspc_123",
				AccessKey:   "AKIA",
			},
			errorMsg: "must be provided together",
		},
		{
			name: "api key mode rejects aws credentials",
			cfg: config.AWSClaudePlatform{
				AuthMode:        config.ClaudePlatformAuthModeAPIKey,
				Region:          "us-east-1",
				WorkspaceID:     "wrkspc_123",
				AccessKey:       "AKIA",
				AccessKeySecret: "secret",
			},
			errorMsg: "access key credentials are not valid with api_key auth mode",
		},
		{
			name: "api key mode rejects role arn",
			cfg: config.AWSClaudePlatform{
				AuthMode:    config.ClaudePlatformAuthModeAPIKey,
				Region:      "us-east-1",
				WorkspaceID: "wrkspc_123",
				RoleARN:     "arn:aws:iam::123456789012:role/claude",
			},
			errorMsg: "role arn is not valid with api_key auth mode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.cfg.Validate()
			if tt.errorMsg != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.errorMsg)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestAWSClaudePlatformResolvedBaseURL(t *testing.T) {
	t.Parallel()

	t.Run("regional default", func(t *testing.T) {
		t.Parallel()

		cfg := config.AWSClaudePlatform{Region: "us-west-2"}
		require.Equal(t, "https://aws-external-anthropic.us-west-2.api.aws", cfg.ResolvedBaseURL())
	})

	t.Run("explicit override wins", func(t *testing.T) {
		t.Parallel()

		cfg := config.AWSClaudePlatform{Region: "us-west-2", BaseURL: "https://proxy.internal/anthropic"}
		require.Equal(t, "https://proxy.internal/anthropic", cfg.ResolvedBaseURL())
	})
}
