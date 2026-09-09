package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

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

// TestNewAnthropic_InferenceProfileResolution drives the Bedrock
// GetInferenceProfile path against a mock endpoint.
// https://docs.aws.amazon.com/bedrock/latest/APIReference/API_GetInferenceProfile.html
// NOTE: no t.Parallel() because the subtests use t.Setenv.
func TestNewAnthropic_InferenceProfileResolution(t *testing.T) {
	const profileARN = "arn:aws:bedrock:us-east-1:123456789012:application-inference-profile/46u2vhiyo6z5"

	bedrockCfg := func(model, smallFastModel string) *config.AWSBedrock {
		return &config.AWSBedrock{
			Region:          "us-east-1",
			AccessKey:       "test-key",
			AccessKeySecret: "test-secret",
			Model:           model,
			SmallFastModel:  smallFastModel,
		}
	}

	// mockBedrock serves the Bedrock control-plane API and records the paths it
	// receives. Callers point the SDK at the returned URL.
	mockBedrock := func(t *testing.T, handler http.HandlerFunc) (url string, paths *[]string) {
		t.Helper()

		var got []string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got = append(got, r.URL.Path)
			handler(w, r)
		}))
		t.Cleanup(srv.Close)
		return srv.URL, &got
	}

	t.Run("resolved profile drives the model id", func(t *testing.T) {
		url, paths := mockBedrock(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"models":[{"modelArn":"arn:aws:bedrock:us-east-1::foundation-model/anthropic.claude-opus-4-8"}]}`))
		})
		t.Setenv("AWS_ENDPOINT_URL_BEDROCK", url)

		p, err := NewAnthropic(context.Background(), config.Anthropic{}, bedrockCfg(profileARN, "anthropic.claude-haiku-4-5"))
		require.NoError(t, err)
		require.Equal(t, "anthropic.claude-opus-4-8", p.bedrock.ResolvedModel())
		// The profile stays the configured identifier so AWS attributes spend to it.
		require.Equal(t, profileARN, p.bedrock.ConfiguredModel())
		require.Equal(t, "anthropic.claude-haiku-4-5", p.bedrock.ResolvedSmallFastModel())
		require.Len(t, *paths, 1, "only the profile ARN is resolved")
		require.Contains(t, (*paths)[0], profileARN)
	})

	t.Run("failed resolution fails construction", func(t *testing.T) {
		url, _ := mockBedrock(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Amzn-Errortype", "AccessDeniedException")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"not authorized to perform bedrock:GetInferenceProfile"}`))
		})
		t.Setenv("AWS_ENDPOINT_URL_BEDROCK", url)

		_, err := NewAnthropic(context.Background(), config.Anthropic{}, bedrockCfg(profileARN, "anthropic.claude-haiku-4-5"))
		require.ErrorContains(t, err, "resolve bedrock models")
		require.ErrorContains(t, err, "GetInferenceProfile")
	})

	t.Run("profile without a model fails construction", func(t *testing.T) {
		url, _ := mockBedrock(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"models":[]}`))
		})
		t.Setenv("AWS_ENDPOINT_URL_BEDROCK", url)

		_, err := NewAnthropic(context.Background(), config.Anthropic{}, bedrockCfg(profileARN, "anthropic.claude-haiku-4-5"))
		require.ErrorContains(t, err, "references no model")
	})

	t.Run("small fast profile resolves independently", func(t *testing.T) {
		const smallFastProfileARN = "arn:aws:bedrock:us-east-1:123456789012:application-inference-profile/8x1qk20fzp3r"

		url, paths := mockBedrock(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"models":[{"modelArn":"arn:aws:bedrock:us-east-1::foundation-model/anthropic.claude-haiku-4-5"}]}`))
		})
		t.Setenv("AWS_ENDPOINT_URL_BEDROCK", url)

		p, err := NewAnthropic(context.Background(), config.Anthropic{}, bedrockCfg("eu.anthropic.claude-opus-4-8", smallFastProfileARN))
		require.NoError(t, err)
		require.Equal(t, "eu.anthropic.claude-opus-4-8", p.bedrock.ResolvedModel())
		require.Equal(t, "anthropic.claude-haiku-4-5", p.bedrock.ResolvedSmallFastModel())
		require.Equal(t, smallFastProfileARN, p.bedrock.ConfiguredSmallFastModel())
		require.Len(t, *paths, 1, "only the small fast profile ARN is resolved")
		require.Contains(t, (*paths)[0], smallFastProfileARN)
	})

	t.Run("plain model id needs no resolution", func(t *testing.T) {
		url, paths := mockBedrock(t, func(http.ResponseWriter, *http.Request) {
			t.Error("Bedrock called for a plain model id")
		})
		t.Setenv("AWS_ENDPOINT_URL_BEDROCK", url)

		p, err := NewAnthropic(context.Background(), config.Anthropic{}, bedrockCfg("eu.anthropic.claude-opus-4-8", "anthropic.claude-haiku-4-5"))
		require.NoError(t, err)
		require.Equal(t, "eu.anthropic.claude-opus-4-8", p.bedrock.ResolvedModel())
		require.Equal(t, "eu.anthropic.claude-opus-4-8", p.bedrock.ConfiguredModel())
		require.Empty(t, *paths)
	})
}
