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

// allowedBedrockFields is the exhaustive allowlist of settings JSON tag names
// that AWSBedrock.ValidationErrors may report as Field values. These MUST match
// the JSON tags on codersdk.AIProviderBedrockSettings (region, model,
// small_fast_model, base_url). This is a literal allowlist rather than a
// reflective read of codersdk tags to keep aibridge/config a leaf package with
// no codersdk dependency; update both sides together if a tag changes.
var allowedBedrockFields = map[string]bool{
	"region":           true,
	"model":            true,
	"small_fast_model": true,
	"base_url":         true,
}

// TestAWSBedrockValidationErrors asserts the field names returned by
// ValidationErrors() for each failing case, plus a drift guard that every
// reported Field is one of the allowed settings JSON tags. Valid configs must
// return nil.
func TestAWSBedrockValidationErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		cfg        config.AWSBedrock
		wantFields []string // expected Field values, in order
	}{
		{
			name: "invoke model missing model",
			cfg: config.AWSBedrock{
				Region:         "us-east-1",
				SmallFastModel: "anthropic.claude-haiku",
			},
			wantFields: []string{"model"},
		},
		{
			name: "invoke model missing small fast model",
			cfg: config.AWSBedrock{
				Region: "us-east-1",
				Model:  "anthropic.claude-sonnet",
			},
			wantFields: []string{"small_fast_model"},
		},
		{
			name: "invoke model missing region and base url",
			cfg: config.AWSBedrock{
				Model:          "anthropic.claude-sonnet",
				SmallFastModel: "anthropic.claude-haiku",
			},
			wantFields: []string{"region"},
		},
		{
			name: "invoke model missing everything",
			cfg:  config.AWSBedrock{},
			// region (or base_url) is checked first, then model, then small_fast_model.
			wantFields: []string{"region", "model", "small_fast_model"},
		},
		{
			name: "mantle missing region",
			cfg: config.AWSBedrock{
				BaseURL:  "https://bedrock-mantle.us-east-1.api.aws",
				Protocol: config.BedrockProtocolMantle,
			},
			wantFields: []string{"region"},
		},
		{
			name: "mantle missing base url",
			cfg: config.AWSBedrock{
				Region:   "us-east-1",
				Protocol: config.BedrockProtocolMantle,
			},
			wantFields: []string{"base_url"},
		},
		{
			name: "invoke model valid",
			cfg: config.AWSBedrock{
				Region:         "us-east-1",
				Model:          "anthropic.claude-sonnet",
				SmallFastModel: "anthropic.claude-haiku",
			},
			wantFields: nil,
		},
		{
			name: "invoke model valid with base url instead of region",
			cfg: config.AWSBedrock{
				BaseURL:        "https://bedrock-runtime.example.com",
				Model:          "anthropic.claude-sonnet",
				SmallFastModel: "anthropic.claude-haiku",
			},
			wantFields: nil,
		},
		{
			name: "mantle valid",
			cfg: config.AWSBedrock{
				Region:   "us-east-1",
				BaseURL:  "https://bedrock-mantle.us-east-1.api.aws/anthropic",
				Protocol: config.BedrockProtocolMantle,
			},
			wantFields: nil,
		},
		{
			// Unknown protocol: ValidationErrors returns nil (the switch falls
			// through); the unknown-protocol hard error is Validate()'s job.
			name:       "unknown protocol yields no field errors",
			cfg:        config.AWSBedrock{Protocol: config.BedrockProtocol("unknown")},
			wantFields: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			errs := tt.cfg.ValidationErrors()
			if tt.wantFields == nil {
				require.Nil(t, errs)
				return
			}
			require.Len(t, errs, len(tt.wantFields), "unexpected number of field errors")
			gotFields := make([]string, 0, len(errs))
			for _, e := range errs {
				// Drift guard: every Field must be a known settings JSON tag.
				require.Truef(t, allowedBedrockFields[e.Field],
					"ValidationErrors returned disallowed field %q; it must be one of the codersdk.AIProviderBedrockSettings JSON tags (region, model, small_fast_model, base_url)", e.Field)
				gotFields = append(gotFields, e.Field)
			}
			require.Equal(t, tt.wantFields, gotFields, "field names out of order or unexpected")
		})
	}
}
