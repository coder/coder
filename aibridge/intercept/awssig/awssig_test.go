package awssig_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge/intercept/awssig"
)

var testCreds = aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
	return aws.Credentials{AccessKeyID: "test-key", SecretAccessKey: "test-secret"}, nil
})

// TestSignMiddlewareService verifies that the signing service is taken from the
// caller rather than hardcoded, so a non-Bedrock AWS upstream is signed under
// its own service name. The credential scope in the Authorization header is the
// observable proof of which service was signed for.
func TestSignMiddlewareService(t *testing.T) {
	t.Parallel()

	for _, service := range []string{"bedrock-mantle", "aws-external-anthropic"} {
		t.Run(service, func(t *testing.T) {
			t.Parallel()

			var gotAuth, gotUserAgent string
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotAuth = r.Header.Get("Authorization")
				gotUserAgent = r.Header.Get("User-Agent")
				w.WriteHeader(http.StatusOK)
			}))
			defer upstream.Close()

			//nolint:bodyclose // the caller below closes the response returned by the middleware.
			mw := awssig.SignMiddleware(testCreds, "us-east-1", service)

			req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, upstream.URL, strings.NewReader(`{"hello":"world"}`))
			require.NoError(t, err)
			req.Header.Set("User-Agent", "aibridge")
			if service == "aws-external-anthropic" {
				req.Header.Set("session_id", "session")
			}

			resp, err := mw(req, func(r *http.Request) (*http.Response, error) {
				return http.DefaultClient.Do(r)
			})
			require.NoError(t, err)
			defer resp.Body.Close()
			_, _ = io.Copy(io.Discard, resp.Body)

			require.True(t, strings.HasPrefix(gotAuth, "AWS4-HMAC-SHA256"), "missing SigV4 auth: %q", gotAuth)
			require.Contains(t, gotAuth, "/us-east-1/"+service+"/aws4_request",
				"signature must be scoped to the caller's service")

			// Signing must not add Bedrock's PRM attribution marker: that is
			// Bedrock-specific and applied by the Bedrock option builders.
			require.NotContains(t, gotUserAgent, awssig.PRMUserAgent)
			if service == "aws-external-anthropic" {
				require.Contains(t, gotAuth, "session_id")
			}
		})
	}
}

// TestSignMiddlewareRestoresBody verifies the request body survives the hashing
// read, so the upstream receives the payload that was signed.
func TestSignMiddlewareRestoresBody(t *testing.T) {
	t.Parallel()

	const payload = `{"hello":"world"}`

	var gotBody string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		gotBody = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	//nolint:bodyclose // the caller below closes the response returned by the middleware.
	mw := awssig.SignMiddleware(testCreds, "us-east-1", "aws-external-anthropic")

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, upstream.URL, strings.NewReader(payload))
	require.NoError(t, err)

	resp, err := mw(req, func(r *http.Request) (*http.Response, error) {
		return http.DefaultClient.Do(r)
	})
	require.NoError(t, err)
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	require.Equal(t, payload, gotBody)
}

// TestAppendPRMUserAgent verifies the Bedrock attribution marker is appended to
// an existing user-agent and never sets one on a request that has none.
func TestAppendPRMUserAgent(t *testing.T) {
	t.Parallel()

	t.Run("appends to existing", func(t *testing.T) {
		t.Parallel()

		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://example.com", nil)
		require.NoError(t, err)
		req.Header.Set("User-Agent", "aibridge")

		awssig.AppendPRMUserAgent(req)
		require.Equal(t, "aibridge "+awssig.PRMUserAgent, req.Header.Get("User-Agent"))
	})

	t.Run("no-op without user agent", func(t *testing.T) {
		t.Parallel()

		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://example.com", nil)
		require.NoError(t, err)

		awssig.AppendPRMUserAgent(req)
		require.Empty(t, req.Header.Get("User-Agent"))
	})
}

// TestBearerMiddleware verifies a user Bedrock API key is sent as a bearer token
// with the body and path untouched, and that no SigV4 headers are produced.
func TestBearerMiddleware(t *testing.T) {
	t.Parallel()

	const token = "bedrock-api-key" //nolint:gosec // G101: test-only fake credential.
	body := `{"model":"anthropic.claude-x","max_tokens":1}`

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		"https://bedrock-mantle.us-east-1.api.aws/anthropic/v1/messages", strings.NewReader(body))
	require.NoError(t, err)

	var captured *http.Request
	var capturedBody string
	//nolint:bodyclose // next returns a synthetic no-op response body.
	_, err = awssig.BearerMiddleware(token)(req, func(r *http.Request) (*http.Response, error) {
		captured = r
		b, readErr := io.ReadAll(r.Body)
		require.NoError(t, readErr)
		capturedBody = string(b)
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
	})
	require.NoError(t, err)
	require.NotNil(t, captured)

	require.Equal(t, "Bearer "+token, captured.Header.Get("Authorization"))
	require.Empty(t, captured.Header.Get("X-Amz-Date"))
	require.Equal(t, "/anthropic/v1/messages", captured.URL.Path)
	require.JSONEq(t, body, capturedBody)
}

func TestBaseURLForModel(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		base    string
		model   string
		want    string
		wantErr bool
	}{
		// anthropic.* models route to the /anthropic prefix; the
		// anthropic-go SDK appends /v1/messages itself.
		{name: "anthropic from bare host", base: "https://bedrock-mantle.us-east-1.api.aws", model: "anthropic.claude-sonnet-5", want: "https://bedrock-mantle.us-east-1.api.aws/anthropic"},
		{name: "anthropic keeps stored /anthropic", base: "https://bedrock-mantle.us-east-1.api.aws/anthropic", model: "anthropic.claude-opus-4-8", want: "https://bedrock-mantle.us-east-1.api.aws/anthropic"},

		// openai.* models route to /openai/v1; the openai-go SDK appends
		// /responses or /chat/completions.
		{name: "openai from bare host", base: "https://bedrock-mantle.us-east-1.api.aws", model: "openai.gpt-5.6-luna", want: "https://bedrock-mantle.us-east-1.api.aws/openai/v1"},
		{name: "openai trims stored /anthropic", base: "https://bedrock-mantle.us-east-1.api.aws/anthropic", model: "openai.gpt-5.5", want: "https://bedrock-mantle.us-east-1.api.aws/openai/v1"},
		{name: "openai keeps stored /openai/v1", base: "https://bedrock-mantle.us-east-1.api.aws/openai/v1", model: "openai.gpt-5.4", want: "https://bedrock-mantle.us-east-1.api.aws/openai/v1"},

		// Third-party models use the root /v1 prefix; Mantle serves them on
		// /v1/chat/completions only.
		{name: "third-party from bare host", base: "https://bedrock-mantle.us-east-1.api.aws", model: "mistral.ministral-3-3b-instruct", want: "https://bedrock-mantle.us-east-1.api.aws/v1"},
		{name: "third-party trims stored /anthropic", base: "https://bedrock-mantle.us-east-1.api.aws/anthropic", model: "moonshotai.kimi-k2.5", want: "https://bedrock-mantle.us-east-1.api.aws/v1"},

		// Trailing slashes and deeper stored prefixes are trimmed first.
		{name: "trailing slash", base: "https://bedrock-mantle.us-east-1.api.aws/", model: "openai.gpt-5.5", want: "https://bedrock-mantle.us-east-1.api.aws/openai/v1"},
		{name: "stored /anthropic/v1", base: "https://bedrock-mantle.us-east-1.api.aws/anthropic/v1", model: "anthropic.claude-sonnet-5", want: "https://bedrock-mantle.us-east-1.api.aws/anthropic"},
		{name: "stored /openai", base: "https://bedrock-mantle.us-east-1.api.aws/openai", model: "openai.gpt-5.4", want: "https://bedrock-mantle.us-east-1.api.aws/openai/v1"},

		// Non-vendor path segments survive trimming.
		{name: "proxy prefix preserved", base: "https://proxy.example.com/mantle", model: "openai.gpt-5.4", want: "https://proxy.example.com/mantle/openai/v1"},

		{name: "invalid base URL", base: "://not-a-url", model: "openai.gpt-5.4", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := awssig.BaseURLForModel(tc.base, tc.model)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}
