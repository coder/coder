package awssig_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge/intercept/awssig"
)

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

			creds := credentials.NewStaticCredentialsProvider("AKIAIOSFODNN7EXAMPLE", "secret", "")
			//nolint:bodyclose // the caller below closes the response returned by the middleware.
			mw := awssig.SignMiddleware(creds, "us-east-1", service)

			req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, upstream.URL, strings.NewReader(`{"hello":"world"}`))
			require.NoError(t, err)
			req.Header.Set("User-Agent", "aibridge")

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

	creds := credentials.NewStaticCredentialsProvider("AKIAIOSFODNN7EXAMPLE", "secret", "")
	//nolint:bodyclose // the caller below closes the response returned by the middleware.
	mw := awssig.SignMiddleware(creds, "us-east-1", "aws-external-anthropic")

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
