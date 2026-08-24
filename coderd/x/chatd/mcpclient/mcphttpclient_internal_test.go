package mcpclient

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMCPSignatureVectors(t *testing.T) {
	t.Parallel()

	const signingSecret = "0123456789abcdef0123456789abcdef"
	tests := []struct {
		name      string
		timestamp string
		method    string
		url       string
		body      string
		headers   http.Header
		canonical string
		signature string
	}{
		{
			name:      "post with body",
			timestamp: "1700000000",
			method:    http.MethodPost,
			url:       "https://mcp.example.com/api/mcp",
			body:      `{"jsonrpc":"2.0","id":1,"method":"tools/call"}`,
			headers: http.Header{
				headerCoderOwnerID:     {"owner-1"},
				headerCoderChatID:      {"chat-1"},
				headerCoderSubchatID:   {"subchat-1"},
				headerCoderWorkspaceID: {"workspace-1"},
			},
			canonical: "v1\n1700000000\nPOST\n/api/mcp\n7423b5e8269f7d1be6b13214cb7ac414e4afa95ce9d4bf0590fa0e69f6978976\nowner=owner-1\nchat=chat-1\nsubchat=subchat-1\nworkspace=workspace-1",
			signature: "v1=50d37d544c31b98f7b3e5bdd25bb9742bb5c6f18a52c459c0228df20699bb8a1",
		},
		{
			name:      "get without body",
			timestamp: "1700000001",
			method:    http.MethodGet,
			url:       "https://mcp.example.com/api/mcp",
			headers: http.Header{
				headerCoderOwnerID:     {"owner-2"},
				headerCoderChatID:      {"chat-2"},
				headerCoderSubchatID:   {"subchat-2"},
				headerCoderWorkspaceID: {"workspace-2"},
			},
			canonical: "v1\n1700000001\nGET\n/api/mcp\ne3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855\nowner=owner-2\nchat=chat-2\nsubchat=subchat-2\nworkspace=workspace-2",
			signature: "v1=8dd946883ceb5c1f1aec023b37b38a978e0b386ecbdff337dc30daa426037ff3",
		},
		{
			name:      "absent optional headers",
			timestamp: "1700000002",
			method:    http.MethodPost,
			url:       "https://mcp.example.com/api/mcp",
			body:      `{}`,
			headers: http.Header{
				headerCoderOwnerID: {"owner-3"},
				headerCoderChatID:  {"chat-3"},
			},
			canonical: "v1\n1700000002\nPOST\n/api/mcp\n44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a\nowner=owner-3\nchat=chat-3\nsubchat=\nworkspace=",
			signature: "v1=5c2267a0875601f14ef5b6f6f9ce0cabdf5718af80b7ab950a16f71553332c58",
		},
		{
			name:      "query string",
			timestamp: "1700000003",
			method:    http.MethodGet,
			url:       "https://mcp.example.com/api/mcp?x=1&y=two",
			headers: http.Header{
				headerCoderOwnerID:     {"owner-4"},
				headerCoderChatID:      {"chat-4"},
				headerCoderWorkspaceID: {"workspace-4"},
			},
			canonical: "v1\n1700000003\nGET\n/api/mcp?x=1&y=two\ne3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855\nowner=owner-4\nchat=chat-4\nsubchat=\nworkspace=workspace-4",
			signature: "v1=8a59223017cfaebd56bc1606ee74104f141a381323605c70144b975edbe42a65",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var body io.Reader
			if tt.body != "" {
				body = strings.NewReader(tt.body)
			}
			req, err := http.NewRequestWithContext(t.Context(), tt.method, tt.url, body)
			require.NoError(t, err)
			req.Header = tt.headers.Clone()

			canonical := mcpSignatureCanonical(req, tt.timestamp, []byte(tt.body))
			require.Equal(t, tt.canonical, canonical)
			require.Equal(t, tt.signature, signMCPRequest(signingSecret, canonical))
		})
	}
}

func TestBufferRequestBodyRestoresBody(t *testing.T) {
	t.Parallel()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://mcp.example.com", nil)
	require.NoError(t, err)
	req.Body = io.NopCloser(strings.NewReader("request body"))
	req.GetBody = nil
	clone := req.Clone(req.Context())

	body, err := bufferRequestBody(req, clone)
	require.NoError(t, err)
	require.Equal(t, []byte("request body"), body)
	require.NotNil(t, req.GetBody)
	require.NotNil(t, clone.GetBody)

	cloneBody, err := io.ReadAll(clone.Body)
	require.NoError(t, err)
	require.Equal(t, []byte("request body"), cloneBody)

	retryBody, err := req.GetBody()
	require.NoError(t, err)
	defer retryBody.Close()
	retryBytes, err := io.ReadAll(retryBody)
	require.NoError(t, err)
	require.Equal(t, []byte("request body"), retryBytes)
}

func TestBufferRequestBodyRefreshesCloneBody(t *testing.T) {
	t.Parallel()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://mcp.example.com", strings.NewReader("request body"))
	require.NoError(t, err)
	require.NotNil(t, req.GetBody)

	for range 2 {
		clone := req.Clone(req.Context())
		body, err := bufferRequestBody(req, clone)
		require.NoError(t, err)
		require.Equal(t, []byte("request body"), body)

		cloneBody, err := io.ReadAll(clone.Body)
		require.NoError(t, err)
		require.Equal(t, []byte("request body"), cloneBody)
	}
}
