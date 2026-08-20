package integrationtest

import (
	"context"
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge/aibridgetest"
	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/testutil"
)

// These tests exercise the Bedrock Mantle provider against the live Mantle
// endpoint using ambient AWS credentials from the environment
// (AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY / AWS_SESSION_TOKEN, or the
// default credential chain). They are skipped unless MANTLE_LIVE_REGION is
// set, so they never run in CI; they exist to validate SigV4 signing and
// per-vendor route selection against the real service during development.
//
// Mantle routes are vendor-namespaced (verified live 2026-08-20):
//   - anthropic.* -> /anthropic/v1/messages
//   - openai.*    -> /openai/v1/responses and /openai/v1/chat/completions
//   - other       -> /v1/chat/completions (no vendor prefix; /v1/responses
//     and the prefixed routes reject third-party models)
func mantleLiveBedrockConfig(t *testing.T) *config.AWSBedrock {
	t.Helper()

	region := os.Getenv("MANTLE_LIVE_REGION")
	if region == "" {
		t.Skip("MANTLE_LIVE_REGION not set; skipping live Mantle test")
	}
	// Static config keys are deliberately left empty so the SDK default
	// credential chain resolves the full environment credential set,
	// including AWS_SESSION_TOKEN (buildBedrockCredentials does not pass a
	// session token to its static provider). IRSA, container credentials,
	// and instance profiles resolve through the same chain.
	cfg := &config.AWSBedrock{
		Region:   region,
		BaseURL:  "https://bedrock-mantle." + region + ".api.aws/anthropic",
		Protocol: config.BedrockProtocolMantle,
	}
	if os.Getenv("AWS_ACCESS_KEY_ID") == "" &&
		os.Getenv("AWS_CONTAINER_CREDENTIALS_RELATIVE_URI") == "" &&
		os.Getenv("AWS_CONTAINER_CREDENTIALS_FULL_URI") == "" &&
		os.Getenv("AWS_WEB_IDENTITY_TOKEN_FILE") == "" {
		t.Skip("no ambient AWS credentials found; skipping live Mantle test")
	}
	return cfg
}

func TestMantleLive(t *testing.T) {
	t.Parallel()

	newLiveBridge := func(t *testing.T, ctx context.Context) *bridgeTestServer {
		t.Helper()
		bedrockCfg := mantleLiveBedrockConfig(t)
		return newBridgeTestServer(ctx, t, "", // upstream URL unused for custom providers
			withCustomProvider(aibridgetest.NewAnthropicProvider(t,
				config.Anthropic{Name: "bedrock", BaseURL: bedrockCfg.BaseURL},
				bedrockCfg)),
			withActor(defaultActorID, nil),
		)
	}

	// Anthropic model over the Messages API: the existing Mantle path.
	t.Run("anthropic/v1/messages", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
		t.Cleanup(cancel)
		bridgeServer := newLiveBridge(t, ctx)

		body := []byte(`{
			"model": "anthropic.claude-sonnet-5",
			"anthropic_version": "bedrock-2023-05-31",
			"max_tokens": 16,
			"messages": [{"role": "user", "content": "Say OK"}]
		}`)
		resp, err := bridgeServer.makeRequest(t, http.MethodPost, "/bedrock/v1/messages", body)
		require.NoError(t, err)
		defer resp.Body.Close()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", respBody)
	})

	// OpenAI model over the Responses API: exercises SigV4-signed OpenAI
	// interception on the Bedrock provider.
	t.Run("openai/v1/responses", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
		t.Cleanup(cancel)
		bridgeServer := newLiveBridge(t, ctx)

		body := []byte(`{
			"model": "openai.gpt-5.6-luna",
			"input": "Say OK",
			"max_output_tokens": 16
		}`)
		resp, err := bridgeServer.makeRequest(t, http.MethodPost, "/bedrock/v1/responses", body)
		require.NoError(t, err)
		defer resp.Body.Close()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", respBody)
	})

	// Third-party model over Chat Completions: Mantle serves non-OpenAI,
	// non-Anthropic vendors on the root /v1/chat/completions route.
	t.Run("v1/chat/completions third-party", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
		t.Cleanup(cancel)
		bridgeServer := newLiveBridge(t, ctx)

		body := []byte(`{
			"model": "mistral.ministral-3-3b-instruct",
			"messages": [{"role": "user", "content": "Say OK"}],
			"max_completion_tokens": 16
		}`)
		resp, err := bridgeServer.makeRequest(t, http.MethodPost, "/bedrock/v1/chat/completions", body)
		require.NoError(t, err)
		defer resp.Body.Close()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", respBody)
	})
}
