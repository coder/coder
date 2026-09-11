package bedrocksig_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge/intercept/bedrocksig"
	"github.com/coder/coder/v2/testutil"
)

var testCreds = aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
	return aws.Credentials{AccessKeyID: "test-key", SecretAccessKey: "test-secret"}, nil
})

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
			got, err := bedrocksig.BaseURLForModel(tc.base, tc.model)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

// TestSignMiddlewareStripsUnsafeHeaders documents a real failure seen when
// bridging clients that send headers containing underscores (e.g. session_id).
// aibridge forwards arbitrary client headers to the upstream and SignMiddleware
// signs whatever is present in req.Header at send time.
//
// We've observed that headers with underscores seem to be stripped somewhere
// along the path between AI Gateway and Bedrock Mantle. When AWS recomputes
// the canonical request from the header value it actually received, we get a
// SigV4 mismatch error.
//
// Workaround: SignMiddleware must not sign or forward headers whose name contains
// an underscore.
func TestSignMiddlewareStripsUnsafeHeaders(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodPost, "https://bedrock-mantle.us-east-1.api.aws/openai/v1/responses", bytes.NewBufferString(`{"model":"openai.gpt-5.6-luna"}`))
	req.Header.Set("Anthropic-Version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("session_id", "01a08608-1a93-760c-98f6-d79765c14f0b")
	req.Header.Set("x_client_request_id", "01a08608-1a93-760c-98f6-d79765c14f0b")
	req.Header.Set("X-Client-Request-Id", "01a08608-1a93-760c-98f6-d79765c14f0b")

	sink := testutil.NewFakeSink(t)
	logger := sink.Logger(slog.LevelDebug)

	mw := bedrocksig.SignMiddleware(logger, testCreds, "us-east-1") //nolint:bodyclose // http.NoBody

	var gotReq *http.Request
	resp, err := mw(req, func(r *http.Request) (*http.Response, error) { //nolint:bodyclose // http.NoBody
		gotReq = r
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, gotReq)

	assert.Empty(t, gotReq.Header.Get("session_id"),
		"underscore-named headers must not be forwarded to Bedrock Mantle")
	assert.Empty(t, gotReq.Header.Get("x_client_request_id"),
		"underscore-named headers must not be forwarded to Bedrock Mantle")

	// Headers without underscores must survive untouched.
	assert.Equal(t, "01a08608-1a93-760c-98f6-d79765c14f0b", gotReq.Header.Get("X-Client-Request-Id"))
	assert.Equal(t, "2023-06-01", gotReq.Header.Get("Anthropic-Version"))

	authHeader := gotReq.Header.Get("Authorization")
	require.NotEmpty(t, authHeader, "request must still be signed")

	// A single warning must be logged naming every stripped header, not one
	// warning per header, so operators get one log line per interception.
	warnings := sink.Entries(func(e slog.SinkEntry) bool { return e.Level == slog.LevelWarn })
	require.Len(t, warnings, 1)
	found := false
	for _, f := range warnings[0].Fields {
		if f.Name == "headers" {
			found = true
			assert.ElementsMatch(t, []string{"Session_id", "X_client_request_id"}, f.Value)
		}
	}
	assert.True(t, found, "expected a 'headers' field naming the stripped headers")
}
