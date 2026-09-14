package agentsdk

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"cloud.google.com/go/compute/metadata"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestAWSInstanceIdentityExchange_AgentName(t *testing.T) {
	t.Parallel()

	capturedBody := runAWSInstanceIdentityExchange(t, WithInstanceIdentityAgentName("test-agent"))
	assertJSONField(t, capturedBody, "agent_name", "test-agent")
}

func TestAWSInstanceIdentityExchange_OmitsAgentName(t *testing.T) {
	t.Parallel()

	capturedBody := runAWSInstanceIdentityExchange(t)
	assertJSONFieldAbsent(t, capturedBody, "agent_name")
}

func TestAzureInstanceIdentityExchange_AgentName(t *testing.T) {
	t.Parallel()

	capturedBody := runAzureInstanceIdentityExchange(t, WithInstanceIdentityAgentName("test-agent"))
	assertJSONField(t, capturedBody, "agent_name", "test-agent")
}

func TestAzureInstanceIdentityExchange_OmitsAgentName(t *testing.T) {
	t.Parallel()

	capturedBody := runAzureInstanceIdentityExchange(t)
	assertJSONFieldAbsent(t, capturedBody, "agent_name")
}

func TestGoogleInstanceIdentityExchange_AgentName(t *testing.T) {
	t.Parallel()

	capturedBody := runGoogleInstanceIdentityExchange(t, WithInstanceIdentityAgentName("test-agent"))
	assertJSONField(t, capturedBody, "agent_name", "test-agent")
}

func TestGoogleInstanceIdentityExchange_OmitsAgentName(t *testing.T) {
	t.Parallel()

	capturedBody := runGoogleInstanceIdentityExchange(t)
	assertJSONFieldAbsent(t, capturedBody, "agent_name")
}

func runAWSInstanceIdentityExchange(t *testing.T, opts ...InstanceIdentityOption) []byte {
	t.Helper()

	var capturedBody []byte
	server := newInstanceIdentityServer(t, "/api/v2/workspaceagents/aws-instance-identity", &capturedBody)
	defer server.Close()

	client := newCodersdkClient(t, server, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.URL.Host == "169.254.169.254" && req.Method == http.MethodPut && req.URL.Path == "/latest/api/token":
			return httpResponse(req, http.StatusOK, "fake-imds-token", nil), nil
		case req.URL.Host == "169.254.169.254" && req.Method == http.MethodGet && req.URL.Path == "/latest/dynamic/instance-identity/signature":
			return httpResponse(req, http.StatusOK, "fakesig", nil), nil
		case req.URL.Host == "169.254.169.254" && req.Method == http.MethodGet && req.URL.Path == "/latest/dynamic/instance-identity/document":
			return httpResponse(req, http.StatusOK, "fakedoc", nil), nil
		default:
			return http.DefaultTransport.RoundTrip(req)
		}
	}))

	provider := requireInstanceIdentityProvider(t, WithAWSInstanceIdentity(opts...)(client))
	resp, err := provider.TokenExchanger.exchange(context.Background())
	require.NoError(t, err)
	require.Equal(t, "test-session-token", resp.SessionToken)

	return capturedBody
}

func runAzureInstanceIdentityExchange(t *testing.T, opts ...InstanceIdentityOption) []byte {
	t.Helper()

	var capturedBody []byte
	server := newInstanceIdentityServer(t, "/api/v2/workspaceagents/azure-instance-identity", &capturedBody)
	defer server.Close()

	client := newCodersdkClient(t, server, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.URL.Host == "169.254.169.254" && req.Method == http.MethodGet && req.URL.Path == "/metadata/attested/document":
			return httpResponse(req, http.StatusOK, `{"signature":"fakesig","encoding":"fakeenc"}`, http.Header{"Content-Type": []string{"application/json"}}), nil
		default:
			return http.DefaultTransport.RoundTrip(req)
		}
	}))

	provider := requireInstanceIdentityProvider(t, WithAzureInstanceIdentity(opts...)(client))
	resp, err := provider.TokenExchanger.exchange(context.Background())
	require.NoError(t, err)
	require.Equal(t, "test-session-token", resp.SessionToken)

	return capturedBody
}

func runGoogleInstanceIdentityExchange(t *testing.T, opts ...InstanceIdentityOption) []byte {
	t.Helper()

	var capturedBody []byte
	server := newInstanceIdentityServer(t, "/api/v2/workspaceagents/google-instance-identity", &capturedBody)
	defer server.Close()

	metadataClient := metadata.NewClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, "169.254.169.254", req.URL.Host)
		require.Equal(t, http.MethodGet, req.Method)
		require.Equal(t, "/computeMetadata/v1/instance/service-accounts/test-service-account/identity", req.URL.Path)
		require.Equal(t, "audience=coder&format=full", req.URL.RawQuery)
		require.Equal(t, "Google", req.Header.Get("Metadata-Flavor"))
		return httpResponse(req, http.StatusOK, "fake-jwt", nil), nil
	})})
	client := newCodersdkClient(t, server, http.DefaultTransport)

	provider := requireInstanceIdentityProvider(t, WithGoogleInstanceIdentity("test-service-account", metadataClient, opts...)(client))
	resp, err := provider.TokenExchanger.exchange(context.Background())
	require.NoError(t, err)
	require.Equal(t, "test-session-token", resp.SessionToken)

	return capturedBody
}

func newInstanceIdentityServer(t *testing.T, path string, capturedBody *[]byte) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		require.Equal(t, http.MethodPost, req.Method)
		require.Equal(t, path, req.URL.Path)

		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		require.NoError(t, req.Body.Close())
		*capturedBody = body

		rw.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(rw).Encode(AuthenticateResponse{SessionToken: "test-session-token"}))
	}))
}

func newCodersdkClient(t *testing.T, server *httptest.Server, transport http.RoundTripper) *codersdk.Client {
	t.Helper()

	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)

	return &codersdk.Client{
		URL: serverURL,
		HTTPClient: &http.Client{
			Transport: transport,
		},
	}
}

func requireInstanceIdentityProvider(t *testing.T, provider RefreshableSessionTokenProvider) *InstanceIdentitySessionTokenProvider {
	t.Helper()

	identityProvider, ok := provider.(*InstanceIdentitySessionTokenProvider)
	require.True(t, ok)
	return identityProvider
}

func httpResponse(req *http.Request, statusCode int, body string, headers http.Header) *http.Response {
	if headers == nil {
		headers = make(http.Header)
	}

	return &http.Response{
		StatusCode: statusCode,
		Header:     headers,
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}
}

func decodeJSONBody(t *testing.T, body []byte) map[string]any {
	t.Helper()

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(body, &decoded))
	return decoded
}

func assertJSONField(t *testing.T, body []byte, key string, want string) {
	t.Helper()

	decoded := decodeJSONBody(t, body)
	require.Equal(t, want, decoded[key])
}

func assertJSONFieldAbsent(t *testing.T, body []byte, key string) {
	t.Helper()

	decoded := decodeJSONBody(t, body)
	_, ok := decoded[key]
	require.False(t, ok)
}
