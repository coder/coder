package integrationtest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4signer "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

	"github.com/coder/coder/v2/aibridge/aibridgetest"
	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/fixtures"
	"github.com/coder/coder/v2/aibridge/intercept"
	"github.com/coder/coder/v2/aibridge/intercept/awssig"
	"github.com/coder/coder/v2/aibridge/internal/testutil"
	"github.com/coder/coder/v2/aibridge/provider"
)

const claudePlatformWorkspaceID = "wrkspc_config"

// claudePlatformCfg returns a Claude Platform for AWS config pointing at the
// given URL. The base URL override keeps both bridged and passthrough routes
// on the mock upstream while the signing scope stays region-based.
func claudePlatformCfg(url string) *config.AWSClaudePlatform {
	return &config.AWSClaudePlatform{
		Region:      "us-west-2",
		WorkspaceID: claudePlatformWorkspaceID,
		BaseURL:     url,
	}
}

func setFakeAWSChain(t *testing.T) {
	t.Helper()
	configDir := t.TempDir()
	t.Setenv("AWS_ACCESS_KEY_ID", "test-access-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test-secret-key")
	t.Setenv("AWS_SESSION_TOKEN", "test-session-token")
	t.Setenv("AWS_PROFILE", "")
	t.Setenv("AWS_CONFIG_FILE", filepath.Join(configDir, "config"))
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(configDir, "credentials"))
	t.Setenv("AWS_WEB_IDENTITY_TOKEN_FILE", "")
	t.Setenv("AWS_ROLE_ARN", "")
	t.Setenv("AWS_ROLE_SESSION_NAME", "")
	t.Setenv("AWS_CONTAINER_CREDENTIALS_RELATIVE_URI", "")
	t.Setenv("AWS_CONTAINER_CREDENTIALS_FULL_URI", "")
	t.Setenv("AWS_CONTAINER_AUTHORIZATION_TOKEN", "")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
}

func TestClaudePlatformLive(t *testing.T) {
	t.Parallel()
	// Run with CODER_TEST_CLAUDE_PLATFORM_LIVE=1 and set
	// CLAUDE_PLATFORM_LIVE_REGION, CLAUDE_PLATFORM_LIVE_WORKSPACE_ID, and
	// CLAUDE_PLATFORM_LIVE_MODEL to use real ambient AWS credentials.
	if os.Getenv("CODER_TEST_CLAUDE_PLATFORM_LIVE") != "1" {
		t.Skip("CODER_TEST_CLAUDE_PLATFORM_LIVE=1 not set; skipping live Claude Platform test")
	}

	region := os.Getenv("CLAUDE_PLATFORM_LIVE_REGION")
	workspaceID := os.Getenv("CLAUDE_PLATFORM_LIVE_WORKSPACE_ID")
	model := os.Getenv("CLAUDE_PLATFORM_LIVE_MODEL")
	if region == "" || workspaceID == "" || model == "" {
		t.Fatalf("CLAUDE_PLATFORM_LIVE_REGION, CLAUDE_PLATFORM_LIVE_WORKSPACE_ID, and CLAUDE_PLATFORM_LIVE_MODEL are required when live testing is enabled")
	}

	ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong*4)
	t.Cleanup(cancel)
	p, err := provider.NewAnthropic(ctx, config.Anthropic{}, nil, &config.AWSClaudePlatform{
		Region:      region,
		WorkspaceID: workspaceID,
	})
	require.NoError(t, err)
	bridgeServer := newBridgeTestServer(ctx, t, "", withCustomProvider(p), withActor(defaultActorID, nil))

	body := []byte(fmt.Sprintf(`{"model":%q,"max_tokens":16,"messages":[{"role":"user","content":"Say OK"}]}`, model))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, bridgeServer.URL+pathAnthropicMessages, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := bridgeServer.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "Claude Platform response: %s", responseBody)
	require.Equal(t, "message", gjson.GetBytes(responseBody, "type").String())
	require.NotEmpty(t, gjson.GetBytes(responseBody, "id").String())
	content := gjson.GetBytes(responseBody, "content")
	require.True(t, content.IsArray())
	require.NotEmpty(t, content.Array())
	require.NotEmpty(t, bridgeServer.Recorder.RecordedInterceptions())
	bridgeServer.Recorder.VerifyAllInterceptionsEnded(t)
}

// Unlike Bedrock it speaks the native Messages wire format, so only routing and
// authentication differ from a direct Anthropic provider.
//
//nolint:paralleltest,tparallel // The parent scopes AWS environment variables for parallel subtests.
func TestClaudePlatformIntegration(t *testing.T) {
	setFakeAWSChain(t)

	t.Run("invalid config", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
		t.Cleanup(cancel)

		cpCfg := claudePlatformCfg("http://unused")
		cpCfg.Region = ""

		_, err := provider.NewAnthropic(ctx, config.Anthropic{}, nil, cpCfg)
		require.ErrorContains(t, err, "region required")
	})

	// Recompute the signature from the wire request, including the final body
	// and workspace header, rather than merely checking for an auth prefix.
	verifySignature := func(t *testing.T, r *http.Request, body []byte) {
		t.Helper()
		auth := r.Header.Get(intercept.AuthHeaderAuthorization)
		require.True(t, strings.HasPrefix(auth, "AWS4-HMAC-SHA256"), "missing SigV4 auth: %q", auth)
		require.Contains(t, auth, "/us-west-2/aws-external-anthropic/aws4_request")
		signedHeaders := strings.Split(extractSigV4Field(auth, "SignedHeaders="), ";")
		require.Contains(t, signedHeaders, strings.ToLower(intercept.HeaderAnthropicWorkspaceID))
		require.Equal(t, claudePlatformWorkspaceID, r.Header.Get(intercept.HeaderAnthropicWorkspaceID))
		require.Empty(t, r.Header.Get(intercept.AuthHeaderXAPIKey))
		require.NotContains(t, r.Header.Get("User-Agent"), awssig.PRMUserAgent)

		verifyReq := r.Clone(r.Context())
		verifyReq.Header.Del(intercept.AuthHeaderAuthorization)
		for h := range verifyReq.Header {
			if !slices.Contains(signedHeaders, strings.ToLower(h)) {
				verifyReq.Header.Del(h)
			}
		}
		verifyReq.ContentLength = int64(len(body))
		signingTime, err := time.Parse("20060102T150405Z", r.Header.Get("X-Amz-Date"))
		require.NoError(t, err)
		hash := sha256.Sum256(body)
		err = v4signer.NewSigner().SignHTTP(r.Context(), aws.Credentials{
			AccessKeyID: "test-access-key", SecretAccessKey: "test-secret-key", SessionToken: "test-session-token",
		}, verifyReq, hex.EncodeToString(hash[:]), config.ClaudePlatformSigningService, "us-west-2", signingTime)
		require.NoError(t, err)
		require.Equal(t, extractSigV4Field(auth, "Signature="),
			extractSigV4Field(verifyReq.Header.Get(intercept.AuthHeaderAuthorization), "Signature="))
	}

	t.Run("iam/v1/messages", func(t *testing.T) {
		t.Parallel()
		for _, streaming := range []bool{true, false} {
			for _, retry := range []bool{false, true} {
				t.Run(fmt.Sprintf("streaming=%v/retry=%v", streaming, retry), func(t *testing.T) {
					t.Parallel()
					ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
					t.Cleanup(cancel)
					fix := fixtures.Parse(t, fixtures.AntSingleBuiltinTool)
					response := testutil.NewFixtureResponse(fix)
					response.OnRequest = func(r *http.Request, body []byte) { verifySignature(t, r, body) }
					responses := []testutil.UpstreamResponse{response}
					if retry {
						failure := testutil.NewErrorResponse(http.StatusInternalServerError, "")
						// Exercise SDK retries, not key-pool failover, with minimal backoff.
						failure.Blocking = bytes.ReplaceAll(failure.Blocking, []byte("x-should-retry: false"), []byte("x-should-retry: true\r\nretry-after-ms: 1"))
						failure.Streaming = failure.Blocking
						failure.OnRequest = response.OnRequest
						responses = append([]testutil.UpstreamResponse{failure}, responses...)
					}
					upstream := testutil.NewMockUpstream(ctx, t, responses...)
					cpCfg := claudePlatformCfg(upstream.URL)
					bridgeServer := newBridgeTestServer(ctx, t, upstream.URL,
						withCustomProvider(aibridgetest.NewClaudePlatformProvider(t, config.Anthropic{}, cpCfg)))
					reqBody, err := sjson.SetBytes(fix.Request(), "stream", streaming)
					require.NoError(t, err)
					resp, err := bridgeServer.makeRequest(t, http.MethodPost, pathAnthropicMessages, reqBody, http.Header{
						intercept.HeaderAnthropicWorkspaceID: {"wrkspc_from_client"},
					})
					require.NoError(t, err)
					defer resp.Body.Close()
					require.Equal(t, http.StatusOK, resp.StatusCode)
					_, err = io.ReadAll(resp.Body)
					require.NoError(t, err)
					received := upstream.ReceivedRequests()
					require.Len(t, received, len(responses))
					wantModel := gjson.GetBytes(reqBody, "model").String()
					for _, request := range received {
						require.Equal(t, "/v1/messages", request.Path)
						require.Equal(t, http.MethodPost, request.Method)
						require.JSONEq(t, string(reqBody), string(request.Body))
					}
					interceptions := bridgeServer.Recorder.RecordedInterceptions()
					require.Len(t, interceptions, 1)
					require.Equal(t, wantModel, interceptions[0].Model)
					bridgeServer.Recorder.VerifyAllInterceptionsEnded(t)
				})
			}
		}
	})

	for _, tc := range []struct {
		name       string
		byokHeader string
		key        string
	}{
		{name: "api_key/v1/messages", key: apiKey},
		{name: "api_key BYOK without pool", byokHeader: intercept.AuthHeaderXAPIKey, key: "user-key"},
		{name: "byok api key wins over ambient IAM", byokHeader: intercept.AuthHeaderXAPIKey, key: "user-key"},
		{name: "byok bearer wins over ambient IAM", byokHeader: intercept.AuthHeaderAuthorization, key: "Bearer user-token"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			for _, streaming := range []bool{true, false} {
				t.Run(fmt.Sprintf("streaming=%v", streaming), func(t *testing.T) {
					t.Parallel()
					ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
					t.Cleanup(cancel)
					fix := fixtures.Parse(t, fixtures.AntSingleBuiltinTool)
					upstream := testutil.NewMockUpstream(ctx, t, testutil.NewFixtureResponse(fix))
					cfg := config.Anthropic{}
					headers := http.Header{intercept.HeaderAnthropicWorkspaceID: {"wrkspc_from_client"}}
					if tc.byokHeader == "" {
						cfg = anthropicCfg(upstream.URL, tc.key)
					} else {
						headers.Set(tc.byokHeader, tc.key)
					}
					bridgeServer := newBridgeTestServer(ctx, t, upstream.URL,
						withCustomProvider(aibridgetest.NewClaudePlatformProvider(t, cfg, claudePlatformCfg(upstream.URL))))
					reqBody, err := sjson.SetBytes(fix.Request(), "stream", streaming)
					require.NoError(t, err)
					resp, err := bridgeServer.makeRequest(t, http.MethodPost, pathAnthropicMessages, reqBody, headers)
					require.NoError(t, err)
					defer resp.Body.Close()
					require.Equal(t, http.StatusOK, resp.StatusCode)
					_, err = io.ReadAll(resp.Body)
					require.NoError(t, err)
					received := upstream.ReceivedRequests()
					require.Len(t, received, 1)
					require.Equal(t, "/v1/messages", received[0].Path)
					require.JSONEq(t, string(reqBody), string(received[0].Body))
					if tc.byokHeader == intercept.AuthHeaderAuthorization {
						require.Equal(t, tc.key, received[0].Header.Get(intercept.AuthHeaderAuthorization))
						require.Empty(t, received[0].Header.Get(intercept.AuthHeaderXAPIKey))
					} else {
						require.Equal(t, tc.key, received[0].Header.Get(intercept.AuthHeaderXAPIKey))
						require.Empty(t, received[0].Header.Get(intercept.AuthHeaderAuthorization), "API keys must not be combined with IAM signing")
					}
					require.Equal(t, claudePlatformWorkspaceID, received[0].Header.Get(intercept.HeaderAnthropicWorkspaceID))
					require.NotContains(t, received[0].Header.Get("User-Agent"), awssig.PRMUserAgent)
					bridgeServer.Recorder.VerifyAllInterceptionsEnded(t)
				})
			}
		})
	}

	t.Run("iam BYOK upstream 401 does not retry with ambient identity", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
		t.Cleanup(cancel)
		fix := fixtures.Parse(t, fixtures.AntSingleBuiltinTool)
		upstream := testutil.NewMockUpstream(ctx, t,
			testutil.NewErrorResponse(http.StatusUnauthorized, ""),
		)
		upstream.AllowOverflow = true
		bridgeServer := newBridgeTestServer(ctx, t, upstream.URL,
			withCustomProvider(aibridgetest.NewClaudePlatformProvider(t, config.Anthropic{}, claudePlatformCfg(upstream.URL))))
		reqBody, err := sjson.SetBytes(fix.Request(), "stream", false)
		require.NoError(t, err)
		resp, err := bridgeServer.makeRequest(t, http.MethodPost, pathAnthropicMessages, reqBody, http.Header{
			intercept.HeaderAnthropicWorkspaceID: {"wrkspc_from_client"},
			intercept.AuthHeaderXAPIKey:          {"rejected-user-key"},
		})
		require.NoError(t, err)
		_, err = io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		received := upstream.ReceivedRequests()
		require.Len(t, received, 1, "BYOK 401 must not retry with ambient IAM credentials")
		require.Equal(t, "rejected-user-key", received[0].Header.Get(intercept.AuthHeaderXAPIKey))
		require.Empty(t, received[0].Header.Get(intercept.AuthHeaderAuthorization), "BYOK must not be replaced by IAM signing")
		bridgeServer.Recorder.VerifyAllInterceptionsEnded(t)
	})
	t.Run("iam passthrough", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
		t.Cleanup(cancel)
		upstream := testutil.NewMockUpstream(ctx, t, testutil.UpstreamResponse{
			Blocking:  []byte(`{"data":[]}`),
			OnRequest: func(r *http.Request, body []byte) { verifySignature(t, r, body) },
		})
		bridgeServer := newBridgeTestServer(ctx, t, upstream.URL,
			withCustomProvider(aibridgetest.NewClaudePlatformProvider(t, config.Anthropic{}, claudePlatformCfg(upstream.URL))))
		resp, err := bridgeServer.makeRequest(t, http.MethodGet, "/anthropic/v1/models", nil, http.Header{
			intercept.HeaderAnthropicWorkspaceID: {"wrkspc_from_client"},
		})
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		_, err = io.ReadAll(resp.Body)
		require.NoError(t, err)
		received := upstream.ReceivedRequests()
		require.Len(t, received, 1)
		require.Equal(t, "/v1/models", received[0].Path)
	})
}
