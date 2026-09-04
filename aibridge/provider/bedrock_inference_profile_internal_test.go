package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/intercept"
	"github.com/coder/coder/v2/testutil"
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

// TestInferenceProfileResolutionOnFirstRequest drives the Bedrock
// GetInferenceProfile path against a mock endpoint.
// https://docs.aws.amazon.com/bedrock/latest/APIReference/API_GetInferenceProfile.html
// NOTE: no t.Parallel() because the subtests use t.Setenv.
func TestInferenceProfileResolutionOnFirstRequest(t *testing.T) {
	const (
		profileARN          = "arn:aws:bedrock:us-east-1:123456789012:application-inference-profile/46u2vhiyo6z5"
		smallFastProfileARN = "arn:aws:bedrock:us-east-1:123456789012:application-inference-profile/8x1qk20fzp3r"
	)

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

		var (
			mu  sync.Mutex
			got []string
		)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			got = append(got, r.URL.Path)
			mu.Unlock()
			handler(w, r)
		}))
		t.Cleanup(srv.Close)
		return srv.URL, &got
	}

	// interceptFor runs a request through the provider the way the bridge does.
	interceptFor := func(t *testing.T, p *Anthropic, model string) (intercept.Interceptor, error) {
		t.Helper()

		body := fmt.Sprintf(`{"model":%q,"max_tokens":10000}`, model)
		req := httptest.NewRequest(http.MethodPost, p.RoutePrefix()+routeMessages, strings.NewReader(body))
		return p.CreateInterceptor(httptest.NewRecorder(), req, testTracer)
	}

	respondWithModel := func(modelARN string) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"models":[{"modelArn":%q}]}`, modelARN)
		}
	}

	t.Run("construction makes no aws call", func(t *testing.T) {
		url, paths := mockBedrock(t, func(http.ResponseWriter, *http.Request) {
			t.Error("Bedrock called during construction")
		})
		t.Setenv("AWS_ENDPOINT_URL_BEDROCK", url)

		_, err := NewAnthropic(context.Background(), config.Anthropic{}, bedrockCfg(profileARN, "anthropic.claude-haiku-4-5"), NewInferenceProfileCache())
		require.NoError(t, err)
		require.Empty(t, *paths)
	})

	t.Run("resolved profile drives the model id", func(t *testing.T) {
		url, paths := mockBedrock(t, respondWithModel("arn:aws:bedrock:us-east-1::foundation-model/anthropic.claude-opus-4-8"))
		t.Setenv("AWS_ENDPOINT_URL_BEDROCK", url)

		p, err := NewAnthropic(context.Background(), config.Anthropic{}, bedrockCfg(profileARN, "anthropic.claude-haiku-4-5"), NewInferenceProfileCache())
		require.NoError(t, err)

		interceptor, err := interceptFor(t, p, "claude-opus-4-8")
		require.NoError(t, err)
		require.Equal(t, "anthropic.claude-opus-4-8", interceptor.Model())
		require.Len(t, *paths, 1, "only the profile ARN is resolved")
		require.Contains(t, (*paths)[0], profileARN)
	})

	t.Run("resolution is cached across requests", func(t *testing.T) {
		url, paths := mockBedrock(t, respondWithModel("arn:aws:bedrock:us-east-1::foundation-model/anthropic.claude-opus-4-8"))
		t.Setenv("AWS_ENDPOINT_URL_BEDROCK", url)

		p, err := NewAnthropic(context.Background(), config.Anthropic{}, bedrockCfg(profileARN, "anthropic.claude-haiku-4-5"), NewInferenceProfileCache())
		require.NoError(t, err)

		for range 3 {
			interceptor, err := interceptFor(t, p, "claude-opus-4-8")
			require.NoError(t, err)
			require.Equal(t, "anthropic.claude-opus-4-8", interceptor.Model())
		}
		require.Len(t, *paths, 1, "the profile is resolved once")
	})

	t.Run("failed resolution fails the request and is retried", func(t *testing.T) {
		var attempts atomic.Int64
		url, _ := mockBedrock(t, func(w http.ResponseWriter, _ *http.Request) {
			if attempts.Add(1) == 1 {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("X-Amzn-Errortype", "AccessDeniedException")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"message":"not authorized to perform bedrock:GetInferenceProfile"}`))
				return
			}
			respondWithModel("arn:aws:bedrock:us-east-1::foundation-model/anthropic.claude-opus-4-8")(w, nil)
		})
		t.Setenv("AWS_ENDPOINT_URL_BEDROCK", url)

		p, err := NewAnthropic(context.Background(), config.Anthropic{}, bedrockCfg(profileARN, "anthropic.claude-haiku-4-5"), NewInferenceProfileCache())
		require.NoError(t, err)

		_, err = interceptFor(t, p, "claude-opus-4-8")
		require.ErrorContains(t, err, "resolve model")
		require.ErrorContains(t, err, "GetInferenceProfile")

		// The provider stays usable: the failure was not cached.
		interceptor, err := interceptFor(t, p, "claude-opus-4-8")
		require.NoError(t, err)
		require.Equal(t, "anthropic.claude-opus-4-8", interceptor.Model())
	})

	t.Run("profile without a model fails the request", func(t *testing.T) {
		url, _ := mockBedrock(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"models":[]}`))
		})
		t.Setenv("AWS_ENDPOINT_URL_BEDROCK", url)

		p, err := NewAnthropic(context.Background(), config.Anthropic{}, bedrockCfg(profileARN, "anthropic.claude-haiku-4-5"), NewInferenceProfileCache())
		require.NoError(t, err)

		_, err = interceptFor(t, p, "claude-opus-4-8")
		require.ErrorContains(t, err, "references no model")
	})

	t.Run("small fast profile resolves independently", func(t *testing.T) {
		url, paths := mockBedrock(t, respondWithModel("arn:aws:bedrock:us-east-1::foundation-model/anthropic.claude-haiku-4-5"))
		t.Setenv("AWS_ENDPOINT_URL_BEDROCK", url)

		p, err := NewAnthropic(context.Background(), config.Anthropic{}, bedrockCfg("eu.anthropic.claude-opus-4-8", smallFastProfileARN), NewInferenceProfileCache())
		require.NoError(t, err)

		interceptor, err := interceptFor(t, p, "claude-haiku-4-5")
		require.NoError(t, err)
		require.Equal(t, "anthropic.claude-haiku-4-5", interceptor.Model())
		require.Len(t, *paths, 1, "only the small fast profile ARN is resolved")
		require.Contains(t, (*paths)[0], smallFastProfileARN)
	})

	t.Run("plain model id needs no resolution", func(t *testing.T) {
		url, paths := mockBedrock(t, func(http.ResponseWriter, *http.Request) {
			t.Error("Bedrock called for a plain model id")
		})
		t.Setenv("AWS_ENDPOINT_URL_BEDROCK", url)

		p, err := NewAnthropic(context.Background(), config.Anthropic{}, bedrockCfg("eu.anthropic.claude-opus-4-8", "anthropic.claude-haiku-4-5"), NewInferenceProfileCache())
		require.NoError(t, err)

		interceptor, err := interceptFor(t, p, "claude-opus-4-8")
		require.NoError(t, err)
		require.Equal(t, "eu.anthropic.claude-opus-4-8", interceptor.Model())
		require.Empty(t, *paths)
	})
}

// TestInferenceProfileCacheSharesConcurrentResolutions verifies that a burst of
// requests for an unresolved profile issues a single AWS lookup.
func TestInferenceProfileCacheSharesConcurrentResolutions(t *testing.T) {
	const profileARN = "arn:aws:bedrock:us-east-1:123456789012:application-inference-profile/46u2vhiyo6z5"

	var calls atomic.Int64
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		<-release
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"models":[{"modelArn":"arn:aws:bedrock:us-east-1::foundation-model/anthropic.claude-opus-4-8"}]}`))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("AWS_ENDPOINT_URL_BEDROCK", srv.URL)

	awsCfg, err := buildBedrockCredentials(context.Background(), config.AWSBedrock{
		Region:          "us-east-1",
		AccessKey:       "test-key",
		AccessKeySecret: "test-secret",
	})
	require.NoError(t, err)

	cache := NewInferenceProfileCache()
	const callers = 8
	results := make(chan error, callers)
	for range callers {
		go func() {
			model, err := cache.Resolve(context.Background(), awsCfg, profileARN)
			if err == nil && model != "anthropic.claude-opus-4-8" {
				err = xerrors.Errorf("unexpected model %q", model)
			}
			results <- err
		}()
	}

	// Hold the handler until every caller has joined the in-flight resolution.
	require.Eventually(t, func() bool { return calls.Load() == 1 }, testutil.WaitShort, testutil.IntervalFast)
	close(release)

	for range callers {
		require.NoError(t, <-results)
	}
	require.Equal(t, int64(1), calls.Load())
}
