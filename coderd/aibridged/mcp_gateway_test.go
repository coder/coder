package aibridged_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/aibridged/proto"
)

func TestServeHTTP_MCPGatewayPolicy(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		var request struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		require.NoError(t, json.Unmarshal(body, &request))
		rw.Header().Set("Content-Type", "application/json")
		switch request.Method {
		case "tools/list":
			_, _ = rw.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"tools":[{"name":"read"},{"name":"write"}]}}`))
		case "tools/call":
			_, _ = rw.Write([]byte(`{"jsonrpc":"2.0","id":2,"result":{"content":[{"type":"text","text":"ok"}]}}`))
		default:
			t.Fatalf("unexpected upstream method %q", request.Method)
		}
	}))
	t.Cleanup(upstream.Close)

	srv, client, _ := newTestServer(t)
	initiatorID := uuid.NewString()
	client.EXPECT().IsAuthorized(gomock.Any(), gomock.Any()).AnyTimes().Return(&proto.IsAuthorizedResponse{
		OwnerId:  initiatorID,
		ApiKeyId: "key-id",
		Username: "agent",
	}, nil)
	client.EXPECT().IsBudgetExceeded(gomock.Any(), gomock.Any()).AnyTimes().Return(&proto.IsBudgetExceededResponse{}, nil)
	client.EXPECT().GetMCPGatewayServerConfig(gomock.Any(), &proto.GetMCPGatewayServerConfigRequest{Slug: "github"}).AnyTimes().Return(&proto.GetMCPGatewayServerConfigResponse{
		Found: true,
		Config: &proto.MCPGatewayServerConfig{
			Id:            uuid.NewString(),
			Slug:          "github",
			Url:           upstream.URL,
			Transport:     "streamable_http",
			ToolAllowList: []string{"read"},
		},
	}, nil)
	client.EXPECT().AuthorizeMCPGateway(gomock.Any(), gomock.Any()).AnyTimes().Return(&proto.AuthorizeMCPGatewayResponse{
		Authorized:  true,
		InitiatorId: initiatorID,
		ApiKeyId:    "key-id",
		Username:    "agent",
	}, nil)

	gateway := httptest.NewServer(srv)
	t.Cleanup(gateway.Close)

	response := mcpGatewayRequest(t, gateway.URL, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	var listResponse struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(response, &listResponse))
	require.Equal(t, "read", listResponse.Result.Tools[0].Name)
	require.Len(t, listResponse.Result.Tools, 1)

	response = mcpGatewayRequest(t, gateway.URL, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"read","arguments":{"path":"README.md"}}}`)
	require.Contains(t, string(response), `"text":"ok"`)
	require.Equal(t, int32(2), calls.Load())

	response = mcpGatewayRequest(t, gateway.URL, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"write"}}`)
	require.Contains(t, string(response), `"code":-32603`)
	require.Contains(t, string(response), `write`)
	require.Equal(t, int32(2), calls.Load(), "denied tool call must not reach upstream")
}

func TestServeHTTP_MCPGatewayAuthResponses(t *testing.T) {
	t.Parallel()

	t.Run("unknown token", func(t *testing.T) {
		t.Parallel()
		srv, client, _ := newTestServer(t)
		client.EXPECT().IsAuthorized(gomock.Any(), gomock.Any()).Return(nil, context.Canceled)

		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/mcp/github", bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
		request.Header.Set("Authorization", "Bearer bad-token")
		srv.ServeHTTP(response, request)
		require.Equal(t, http.StatusUnauthorized, response.Code)
	})

	t.Run("unknown server", func(t *testing.T) {
		t.Parallel()
		srv, client, _ := newTestServer(t)
		client.EXPECT().IsAuthorized(gomock.Any(), gomock.Any()).Return(&proto.IsAuthorizedResponse{OwnerId: uuid.NewString()}, nil)
		client.EXPECT().IsBudgetExceeded(gomock.Any(), gomock.Any()).Return(&proto.IsBudgetExceededResponse{}, nil)
		client.EXPECT().GetMCPGatewayServerConfig(gomock.Any(), gomock.Any()).Return(&proto.GetMCPGatewayServerConfigResponse{}, nil)

		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/mcp/missing", bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
		request.Header.Set("Authorization", "Bearer token")
		srv.ServeHTTP(response, request)
		require.Equal(t, http.StatusNotFound, response.Code)
	})
}

func mcpGatewayRequest(t *testing.T, gatewayURL, body string) []byte {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, gatewayURL+"/mcp/github", bytes.NewBufferString(body))
	require.NoError(t, err)
	request.Header.Set("Authorization", "Bearer coder-token")
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusOK, response.StatusCode)
	responseBody, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	return responseBody
}
