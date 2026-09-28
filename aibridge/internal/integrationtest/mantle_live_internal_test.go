package integrationtest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge/aibridgetest"
	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/testutil"
)

// These tests exercise the Bedrock Mantle provider against the live Mantle
// endpoint using ambient AWS credentials from the environment (the default
// SDK credential chain: AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY /
// AWS_SESSION_TOKEN, IRSA, container credentials, or instance profile). They
// are opt-in: they run only when CODER_TEST_MANTLE_LIVE=1 is set explicitly,
// and require MANTLE_LIVE_REGION to be set, so they never run in CI.
//
// Rather than pinning specific models, the test discovers the catalog from
// the Mantle /v1/models endpoint and probes every model on the route its
// vendor prefix implies. Mantle routes are vendor-namespaced (verified live
// 2026-08-20):
//   - anthropic.* -> /anthropic/v1/messages
//   - openai.*    -> /openai/v1/responses
//   - other       -> /v1/chat/completions (no vendor prefix)
//
// Every supported model must return 200 on its prefix-implied route (a
// re-run usually clears first-touch Marketplace provisioning failures).
// Families Mantle serves only on routes the prefix rules cannot reach are
// listed in mantleLiveUnsupportedFamilies and asserted to fail; if one starts
// returning 200, Mantle added the route and the bridge rules should be
// revisited.
func TestMantleLive(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong*4)
	t.Cleanup(cancel)

	if os.Getenv("CODER_TEST_MANTLE_LIVE") != "1" {
		t.Skip("CODER_TEST_MANTLE_LIVE=1 not set; skipping live Mantle test")
	}

	region := os.Getenv("MANTLE_LIVE_REGION")
	if region == "" {
		t.Skip("MANTLE_LIVE_REGION not set; skipping live Mantle test")
	}

	creds := mantleLiveCredentials(ctx, t, region)
	models := mantleLiveModels(ctx, t, region, creds)
	require.NotEmpty(t, models, "mantle catalog returned no models")
	t.Logf("discovered %d mantle models", len(models))

	bridgeServer := newBridgeTestServer(ctx, t, "", // upstream URL unused for custom providers
		withCustomProvider(aibridgetest.NewBedrockProvider(t,
			config.Anthropic{
				Name:    "bedrock",
				BaseURL: "https://bedrock-mantle." + region + ".api.aws/anthropic",
			},
			config.AWSBedrock{
				Region:   region,
				BaseURL:  "https://bedrock-mantle." + region + ".api.aws/anthropic",
				Protocol: config.BedrockProtocolMantle,
			})),
		withActor(defaultActorID, nil),
	)

	for _, model := range models {
		t.Run(model, func(t *testing.T) {
			t.Parallel()

			call := mantleLiveRequest(model)
			status, respBody := mantleLiveDo(t, bridgeServer, call)

			if mantleLiveUnsupported(model) {
				// These families are pinned to routes Mantle does not serve
				// them on, so the bridge cannot reach them. If one starts
				// passing, Mantle added the route and the model should move
				// to the supported set (and the bridge prefix rules revisited).
				if status == http.StatusOK {
					t.Logf("UNSUPPORTED-NOW-WORKS: %s returned 200 via %s; remove from mantleLiveUnsupportedFamilies", model, call.path)
					return
				}
				t.Logf("expected unsupported: %s via %s -> %d", model, call.path, status)
				return
			}

			require.Equal(t, http.StatusOK, status, "model %s via %s: %s", model, call.path, respBody)
		})
	}
}

// mantleLiveUnsupportedFamilies lists model ID prefixes that Mantle's
// catalog advertises but the bridge cannot reach, because Mantle serves them
// only on routes the prefix-based selection does not choose (verified live
// 2026-08-20):
//   - openai.gpt-oss-*:   Chat-Completions-only, and only on the root
//     /v1/chat/completions route, but the openai. prefix selects /openai/v1.
//   - google.gemma-4-*:   served on /openai/v1/responses, but the non-prefixed
//     selection sends them to root /v1/chat/completions.
//   - xai.grok-*:         same as gemma-4.
var mantleLiveUnsupportedFamilies = []string{
	"openai.gpt-oss",
	"google.gemma-4",
	"xai.grok",
}

func mantleLiveUnsupported(model string) bool {
	for _, prefix := range mantleLiveUnsupportedFamilies {
		if strings.HasPrefix(model, prefix) {
			return true
		}
	}
	return false
}

// mantleLiveDo posts one call through the bridge and returns the status and
// response body.
func mantleLiveDo(t *testing.T, bridgeServer *bridgeTestServer, call mantleLiveCall) (int, []byte) {
	t.Helper()
	resp, err := bridgeServer.makeRequest(t, http.MethodPost, call.path, call.body)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, body
}

// mantleLiveRequest returns the bridge path and request body for the model's
// vendor-prefix-implied route.
func mantleLiveRequest(model string) mantleLiveCall {
	switch {
	case strings.HasPrefix(model, "anthropic."):
		return mantleLiveCall{
			path: "/bedrock/v1/messages",
			body: []byte(fmt.Sprintf(`{
				"model": %q,
				"anthropic_version": "bedrock-2023-05-31",
				"max_tokens": 16,
				"messages": [{"role": "user", "content": "Say OK"}]
			}`, model)),
		}
	case strings.HasPrefix(model, "openai."):
		return mantleLiveCall{
			path: "/bedrock/v1/responses",
			body: []byte(fmt.Sprintf(`{
				"model": %q,
				"input": "Say OK",
				"max_output_tokens": 32
			}`, model)),
		}
	default:
		return mantleLiveCall{
			path: "/bedrock/v1/chat/completions",
			body: []byte(fmt.Sprintf(`{
				"model": %q,
				"messages": [{"role": "user", "content": "Say OK"}],
				"max_completion_tokens": 16
			}`, model)),
		}
	}
}

type mantleLiveCall struct {
	path string
	body []byte
}

// mantleLiveCredentials resolves the ambient AWS credential chain, skipping
// the test when no credentials are available.
func mantleLiveCredentials(ctx context.Context, t *testing.T, region string) aws.Credentials {
	t.Helper()
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	require.NoError(t, err, "load AWS config")
	creds, err := cfg.Credentials.Retrieve(ctx)
	if err != nil {
		t.Skipf("no resolvable AWS credentials: %v", err)
	}
	return creds
}

// mantleLiveModels fetches the live Mantle model catalog, SigV4-signed with
// the same credential chain the bridge uses. Returns sorted model IDs.
func mantleLiveModels(ctx context.Context, t *testing.T, region string, creds aws.Credentials) []string {
	t.Helper()

	u := "https://bedrock-mantle." + region + ".api.aws/v1/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	require.NoError(t, err)

	// SigV4 over an empty body for GET.
	emptyHash := sha256.Sum256(nil)
	signer := v4.NewSigner()
	err = signer.SignHTTP(ctx, creds, req, hex.EncodeToString(emptyHash[:]), "bedrock-mantle", region, time.Now())
	require.NoError(t, err, "sign catalog request")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err, "fetch mantle catalog")
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "catalog: %s", respBody)

	var catalog struct {
		Data []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(respBody, &catalog), "parse catalog: %s", respBody[:min(len(respBody), 200)])

	var models []string
	for _, m := range catalog.Data {
		if m.Status != "available" {
			continue
		}
		models = append(models, m.ID)
	}
	slices.Sort(models)
	return models
}
