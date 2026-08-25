package aibridged_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/coder/coder/v2/coderd/aibridged"
	mock "github.com/coder/coder/v2/coderd/aibridged/aibridgedmock"
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
	sponsorID := uuid.NewString()
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
		Authorized:    true,
		InitiatorId:   initiatorID,
		ApiKeyId:      "key-id",
		Username:      "agent",
		SponsorUserId: sponsorID,
	}, nil)
	var interceptionIDs []string
	client.EXPECT().RecordInterception(gomock.Any(), gomock.Any()).Times(2).DoAndReturn(func(_ context.Context, in *proto.RecordInterceptionRequest) (*proto.RecordInterceptionResponse, error) {
		require.Equal(t, initiatorID, in.GetInitiatorId())
		require.Equal(t, sponsorID, in.GetSponsorUserId())
		require.Equal(t, "key-id", in.GetApiKeyId())
		require.Equal(t, "mcp-gateway", in.GetProvider())
		require.Equal(t, "mcp-gateway", in.GetProviderName())
		require.Equal(t, "github", in.GetModel())
		interceptionIDs = append(interceptionIDs, in.GetId())
		return &proto.RecordInterceptionResponse{}, nil
	})
	var toolUsages []*proto.RecordToolUsageRequest
	client.EXPECT().RecordToolUsage(gomock.Any(), gomock.Any()).Times(2).DoAndReturn(func(_ context.Context, in *proto.RecordToolUsageRequest) (*proto.RecordToolUsageResponse, error) {
		toolUsages = append(toolUsages, in)
		return &proto.RecordToolUsageResponse{}, nil
	})
	client.EXPECT().RecordInterceptionEnded(gomock.Any(), gomock.Any()).Times(2).DoAndReturn(func(_ context.Context, in *proto.RecordInterceptionEndedRequest) (*proto.RecordInterceptionEndedResponse, error) {
		require.Contains(t, interceptionIDs, in.GetId())
		return &proto.RecordInterceptionEndedResponse{}, nil
	})

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

	require.Len(t, toolUsages, 2)
	require.Equal(t, interceptionIDs[0], toolUsages[0].GetInterceptionId())
	require.Equal(t, "read", toolUsages[0].GetTool())
	require.JSONEq(t, `{"path":"README.md"}`, toolUsages[0].GetInput())
	require.Equal(t, upstream.URL, toolUsages[0].GetServerUrl())
	require.Equal(t, "2", toolUsages[0].GetToolCallId())
	require.Equal(t, "2", toolUsages[0].GetItemId())
	require.Empty(t, toolUsages[0].GetInvocationError())
	require.Equal(t, "permitted", toolUsages[0].GetDisposition())

	require.Equal(t, interceptionIDs[1], toolUsages[1].GetInterceptionId())
	require.Equal(t, "write", toolUsages[1].GetTool())
	require.JSONEq(t, "null", toolUsages[1].GetInput())
	require.Equal(t, upstream.URL, toolUsages[1].GetServerUrl())
	require.Contains(t, toolUsages[1].GetInvocationError(), `tool "write" denied`)
	require.Equal(t, "blocked", toolUsages[1].GetDisposition())
}

func TestServeHTTP_MCPGatewayEscalation(t *testing.T) {
	t.Parallel()

	setup := func(t *testing.T, upstreamURL string) (*aibridged.Server, *mock.MockDRPCClient, string) {
		t.Helper()
		srv, client, _ := newTestServer(t)
		initiatorID := uuid.NewString()
		configID := uuid.NewString()
		client.EXPECT().IsAuthorized(gomock.Any(), gomock.Any()).AnyTimes().Return(&proto.IsAuthorizedResponse{
			OwnerId:  initiatorID,
			ApiKeyId: "key-id",
			Username: "agent",
		}, nil)
		client.EXPECT().IsBudgetExceeded(gomock.Any(), gomock.Any()).AnyTimes().Return(&proto.IsBudgetExceededResponse{}, nil)
		client.EXPECT().GetMCPGatewayServerConfig(gomock.Any(), gomock.Any()).Return(&proto.GetMCPGatewayServerConfigResponse{
			Found: true,
			Config: &proto.MCPGatewayServerConfig{
				Id:           configID,
				Slug:         "github",
				Url:          upstreamURL,
				Transport:    "streamable_http",
				AuthType:     "api_key",
				ApiKeyHeader: "X-Upstream-Key",
				ApiKeyValue:  "secret",
				ToolRules: []*proto.MCPGatewayToolRule{
					{Tool: "delete_repo", Action: "escalate"},
				},
				ToolDefault: "enabled",
			},
		}, nil)
		client.EXPECT().AuthorizeMCPGateway(gomock.Any(), gomock.Any()).Return(&proto.AuthorizeMCPGatewayResponse{
			Authorized:  true,
			InitiatorId: initiatorID,
			ApiKeyId:    "key-id",
		}, nil)
		client.EXPECT().RecordInterception(gomock.Any(), gomock.Any()).Return(&proto.RecordInterceptionResponse{}, nil)
		client.EXPECT().RecordInterceptionEnded(gomock.Any(), gomock.Any()).Return(&proto.RecordInterceptionEndedResponse{}, nil)
		return srv, client, configID
	}

	t.Run("approved after two waits", func(t *testing.T) {
		t.Parallel()

		var upstreamCalls atomic.Int32
		upstream := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			upstreamCalls.Add(1)
			require.Equal(t, "secret", r.Header.Get("X-Upstream-Key"))
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			require.JSONEq(t, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"delete_repo","arguments":{"repo":"example"}}}`, string(body))
			rw.Header().Set("Content-Type", "application/json")
			_, _ = rw.Write([]byte(`{"jsonrpc":"2.0","id":2,"result":{"content":[{"type":"text","text":"deleted"}]}}`))
		}))
		t.Cleanup(upstream.Close)

		srv, client, configID := setup(t, upstream.URL)
		escalationID := uuid.NewString()
		client.EXPECT().CreateMCPGatewayEscalation(gomock.Any(), &proto.CreateMCPGatewayEscalationRequest{
			Key:               "coder-token",
			McpServerConfigId: configID,
			ServerSlug:        "github",
			ServerUrl:         upstream.URL,
			Tool:              "delete_repo",
			Input:             `{"repo":"example"}`,
			WorkspaceName:     "",
		}).Return(&proto.CreateMCPGatewayEscalationResponse{
			Id:        escalationID,
			ExpiresAt: timestamppb.New(time.Now().Add(time.Minute)),
		}, nil)
		var waits atomic.Int32
		client.EXPECT().WaitMCPGatewayEscalation(gomock.Any(), &proto.WaitMCPGatewayEscalationRequest{Id: escalationID}).Times(2).DoAndReturn(
			func(context.Context, *proto.WaitMCPGatewayEscalationRequest) (*proto.WaitMCPGatewayEscalationResponse, error) {
				if waits.Add(1) == 1 {
					return &proto.WaitMCPGatewayEscalationResponse{Status: "pending"}, nil
				}
				return &proto.WaitMCPGatewayEscalationResponse{Status: "approved"}, nil
			},
		)
		var usage *proto.RecordToolUsageRequest
		client.EXPECT().RecordToolUsage(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, in *proto.RecordToolUsageRequest) (*proto.RecordToolUsageResponse, error) {
			usage = in
			return &proto.RecordToolUsageResponse{}, nil
		})

		gateway := httptest.NewServer(srv)
		t.Cleanup(gateway.Close)
		request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, gateway.URL+"/mcp/github", bytes.NewBufferString(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"delete_repo","arguments":{"repo":"example"}}}`))
		require.NoError(t, err)
		request.Header.Set("Authorization", "Bearer coder-token")
		request.Header.Set("Content-Type", "application/json")
		response, err := http.DefaultClient.Do(request)
		require.NoError(t, err)
		defer response.Body.Close()
		require.Equal(t, http.StatusOK, response.StatusCode)
		require.Contains(t, response.Header.Get("Content-Type"), "text/event-stream")
		responseBody, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		events := string(responseBody)
		require.Contains(t, events, ": escalation pending "+escalationID)
		require.Contains(t, events, ": keepalive")
		require.Contains(t, events, "event: message\ndata: ")
		require.Contains(t, events, `"text":"deleted"`)
		require.Equal(t, int32(1), upstreamCalls.Load())
		require.NotNil(t, usage)
		require.Equal(t, "escalated_approved", usage.GetDisposition())
		require.Equal(t, escalationID, usage.GetEscalationId())
		require.Empty(t, usage.GetInvocationError())
	})

	t.Run("denied", func(t *testing.T) {
		t.Parallel()

		var upstreamCalls atomic.Int32
		upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			upstreamCalls.Add(1)
		}))
		t.Cleanup(upstream.Close)

		srv, client, _ := setup(t, upstream.URL)
		escalationID := uuid.NewString()
		client.EXPECT().CreateMCPGatewayEscalation(gomock.Any(), gomock.Any()).Return(&proto.CreateMCPGatewayEscalationResponse{
			Id:        escalationID,
			ExpiresAt: timestamppb.New(time.Now().Add(time.Minute)),
		}, nil)
		client.EXPECT().WaitMCPGatewayEscalation(gomock.Any(), &proto.WaitMCPGatewayEscalationRequest{Id: escalationID}).Return(&proto.WaitMCPGatewayEscalationResponse{Status: "denied"}, nil)
		var usage *proto.RecordToolUsageRequest
		client.EXPECT().RecordToolUsage(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, in *proto.RecordToolUsageRequest) (*proto.RecordToolUsageResponse, error) {
			usage = in
			return &proto.RecordToolUsageResponse{}, nil
		})

		gateway := httptest.NewServer(srv)
		t.Cleanup(gateway.Close)
		response := mcpGatewayRequest(t, gateway.URL, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"delete_repo","arguments":{}}}`)
		events := string(response)
		require.Contains(t, events, `"error"`)
		require.Contains(t, events, `"escalation_id":"`+escalationID+`"`)
		require.Contains(t, events, `"status":"denied"`)
		require.Zero(t, upstreamCalls.Load())
		require.NotNil(t, usage)
		require.Equal(t, "escalated_denied", usage.GetDisposition())
		require.Equal(t, escalationID, usage.GetEscalationId())
	})

	t.Run("batch denied locally", func(t *testing.T) {
		t.Parallel()

		var upstreamCalls atomic.Int32
		upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			upstreamCalls.Add(1)
		}))
		t.Cleanup(upstream.Close)

		srv, client, _ := setup(t, upstream.URL)
		var usage *proto.RecordToolUsageRequest
		client.EXPECT().RecordToolUsage(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, in *proto.RecordToolUsageRequest) (*proto.RecordToolUsageResponse, error) {
			usage = in
			return &proto.RecordToolUsageResponse{}, nil
		})

		gateway := httptest.NewServer(srv)
		t.Cleanup(gateway.Close)
		response := mcpGatewayRequest(t, gateway.URL, `[{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"delete_repo","arguments":{}}}]`)
		require.Contains(t, string(response), "escalated tools cannot be called in batches")
		require.Zero(t, upstreamCalls.Load())
		require.NotNil(t, usage)
		require.Equal(t, "blocked", usage.GetDisposition())
		require.Empty(t, usage.GetEscalationId())
	})
}

// TestServeHTTP_MCPGatewaySSEFiltering exercises a strict Streamable HTTP
// upstream, modeled on GitHub's MCP server, which rejects POSTs whose Accept
// header does not list both JSON and SSE and answers with an SSE stream.
func TestServeHTTP_MCPGatewaySSEFiltering(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		accept := r.Header.Get("Accept")
		if !strings.Contains(accept, "application/json") || !strings.Contains(accept, "text/event-stream") {
			http.Error(rw, "Accept must contain both 'application/json' and 'text/event-stream'", http.StatusNotAcceptable)
			return
		}
		rw.Header().Set("Content-Type", "text/event-stream")
		rw.Header().Set("Mcp-Session-Id", "session-1")
		rw.WriteHeader(http.StatusOK)
		_, _ = rw.Write([]byte(": stream comment\n\n"))
		_, _ = rw.Write([]byte("event: message\ndata: {\"jsonrpc\":\"2.0\",\"method\":\"notifications/message\",\"params\":{\"level\":\"info\",\"data\":\"working\"}}\n\n"))
		_, _ = rw.Write([]byte("event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"tools\":[{\"name\":\"read\"},{\"name\":\"write\"}]}}\n\n"))
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

	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, gateway.URL+"/mcp/github", bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	require.NoError(t, err)
	request.Header.Set("Authorization", "Bearer coder-token")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.Contains(t, response.Header.Get("Content-Type"), "text/event-stream")
	require.Equal(t, "session-1", response.Header.Get("Mcp-Session-Id"))

	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	events := string(body)
	require.Contains(t, events, ": stream comment", "comment events must pass through")
	require.Contains(t, events, `"notifications/message"`, "notification events must pass through")
	require.Contains(t, events, `{"name":"read"}`)
	require.NotContains(t, events, `"write"`, "denied tool must be filtered from tools/list")
	require.Contains(t, events, "event: message\ndata: ", "SSE event framing must be preserved")
}

func TestServeHTTP_MCPGatewayRecording(t *testing.T) {
	t.Parallel()

	t.Run("batch", func(t *testing.T) {
		t.Parallel()

		upstream := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			var requests []mcpGatewayTestEnvelope
			require.NoError(t, json.Unmarshal(body, &requests))
			require.Len(t, requests, 2)
			rw.Header().Set("Content-Type", "application/json")
			_, _ = rw.Write([]byte(`[{"jsonrpc":"2.0","id":"call-a","result":{}},{"jsonrpc":"2.0","id":22,"result":{}}]`))
		}))
		t.Cleanup(upstream.Close)

		srv, client, _ := newTestServer(t)
		initiatorID := uuid.NewString()
		setupMCPGatewayRecordingTest(t, client, upstream.URL, initiatorID)
		var interceptionID string
		client.EXPECT().RecordInterception(gomock.Any(), gomock.Any()).Times(1).DoAndReturn(func(_ context.Context, in *proto.RecordInterceptionRequest) (*proto.RecordInterceptionResponse, error) {
			interceptionID = in.GetId()
			return &proto.RecordInterceptionResponse{}, nil
		})
		var usages []*proto.RecordToolUsageRequest
		client.EXPECT().RecordToolUsage(gomock.Any(), gomock.Any()).Times(2).DoAndReturn(func(_ context.Context, in *proto.RecordToolUsageRequest) (*proto.RecordToolUsageResponse, error) {
			usages = append(usages, in)
			return &proto.RecordToolUsageResponse{}, nil
		})
		client.EXPECT().RecordInterceptionEnded(gomock.Any(), gomock.Any()).Times(1).DoAndReturn(func(_ context.Context, in *proto.RecordInterceptionEndedRequest) (*proto.RecordInterceptionEndedResponse, error) {
			require.Equal(t, interceptionID, in.GetId())
			return &proto.RecordInterceptionEndedResponse{}, nil
		})

		gateway := httptest.NewServer(srv)
		t.Cleanup(gateway.Close)
		response := mcpGatewayRequest(t, gateway.URL, `[
			{"jsonrpc":"2.0","id":"call-a","method":"tools/call","params":{"name":"read","arguments":{"path":"a"}}},
			{"jsonrpc":"2.0","id":22,"method":"tools/call","params":{"name":"search","arguments":{"query":"b"}}}
		]`)
		require.Contains(t, string(response), `"id":"call-a"`)
		require.Len(t, usages, 2)
		require.Equal(t, interceptionID, usages[0].GetInterceptionId())
		require.Equal(t, interceptionID, usages[1].GetInterceptionId())
		require.Equal(t, []string{"read", "search"}, []string{usages[0].GetTool(), usages[1].GetTool()})
		require.Equal(t, "call-a", usages[0].GetToolCallId())
		require.Equal(t, "22", usages[1].GetToolCallId())
		require.Equal(t, "permitted", usages[0].GetDisposition())
		require.Equal(t, "permitted", usages[1].GetDisposition())
	})

	t.Run("recording failure", func(t *testing.T) {
		t.Parallel()

		var upstreamCalls atomic.Int32
		upstream := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
			upstreamCalls.Add(1)
			rw.Header().Set("Content-Type", "application/json")
			_, _ = rw.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"ok":true}}`))
		}))
		t.Cleanup(upstream.Close)

		srv, client, _ := newTestServer(t)
		setupMCPGatewayRecordingTest(t, client, upstream.URL, uuid.NewString())
		client.EXPECT().RecordInterception(gomock.Any(), gomock.Any()).Return(nil, context.Canceled)

		gateway := httptest.NewServer(srv)
		t.Cleanup(gateway.Close)
		response := mcpGatewayRequest(t, gateway.URL, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"read"}}`)
		require.Contains(t, string(response), `"ok":true`)
		require.Equal(t, int32(1), upstreamCalls.Load())
	})
}

type mcpGatewayTestEnvelope struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
}

func setupMCPGatewayRecordingTest(t *testing.T, client *mock.MockDRPCClient, upstreamURL, initiatorID string) {
	t.Helper()
	client.EXPECT().IsAuthorized(gomock.Any(), gomock.Any()).AnyTimes().Return(&proto.IsAuthorizedResponse{
		OwnerId:  initiatorID,
		ApiKeyId: "key-id",
		Username: "agent",
	}, nil)
	client.EXPECT().IsBudgetExceeded(gomock.Any(), gomock.Any()).AnyTimes().Return(&proto.IsBudgetExceededResponse{}, nil)
	client.EXPECT().GetMCPGatewayServerConfig(gomock.Any(), gomock.Any()).Return(&proto.GetMCPGatewayServerConfigResponse{
		Found: true,
		Config: &proto.MCPGatewayServerConfig{
			Id:        uuid.NewString(),
			Slug:      "github",
			Url:       upstreamURL,
			Transport: "streamable_http",
		},
	}, nil)
	client.EXPECT().AuthorizeMCPGateway(gomock.Any(), gomock.Any()).Return(&proto.AuthorizeMCPGatewayResponse{
		Authorized:  true,
		InitiatorId: initiatorID,
		ApiKeyId:    "key-id",
		Username:    "agent",
	}, nil)
}

func TestServeHTTP_MCPGatewayCredentials(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		config        *proto.MCPGatewayServerConfig
		expected      http.Header
		requestCount  int
		credentialRPC *proto.GetMCPUpstreamCredentialResponse
	}{
		{
			name: "none",
			config: &proto.MCPGatewayServerConfig{
				AuthType: "none",
			},
			expected: http.Header{},
		},
		{
			name: "api key",
			config: &proto.MCPGatewayServerConfig{
				AuthType:     "api_key",
				ApiKeyHeader: "X-Upstream-Key",
				ApiKeyValue:  "api-secret",
			},
			expected: http.Header{"X-Upstream-Key": []string{"api-secret"}},
		},
		{
			name: "custom headers",
			config: &proto.MCPGatewayServerConfig{
				AuthType:      "custom_headers",
				CustomHeaders: `{"X-Custom-One":"one","X-Custom-Two":"two"}`,
			},
			expected: http.Header{
				"X-Custom-One": []string{"one"},
				"X-Custom-Two": []string{"two"},
			},
		},
		{
			name: "external auth caches credential",
			config: &proto.MCPGatewayServerConfig{
				AuthType:               "external_auth",
				ExternalAuthProviderId: "github",
			},
			expected:     http.Header{"Authorization": []string{"Bearer sponsor-token"}},
			requestCount: 2,
			credentialRPC: &proto.GetMCPUpstreamCredentialResponse{
				AccessToken: "sponsor-token",
				ProviderId:  "github",
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				for key, values := range testCase.expected {
					require.Equal(t, values, r.Header.Values(key))
				}
				if testCase.expected.Get("Authorization") == "" {
					require.Empty(t, r.Header.Get("Authorization"))
				}
				require.Empty(t, r.Header.Get("X-Coder-AI-Governance-Token"))
				require.Empty(t, r.Header.Get("X-Api-Key"))
				require.Empty(t, r.Header.Get("Cookie"))
				rw.Header().Set("Content-Type", "application/json")
				_, _ = rw.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}`))
			}))
			t.Cleanup(upstream.Close)

			srv, client, _ := newTestServer(t)
			initiatorID := uuid.NewString()
			configID := uuid.NewString()
			testCase.config.Id = configID
			testCase.config.Slug = "github"
			testCase.config.Url = upstream.URL
			testCase.config.Transport = "streamable_http"
			client.EXPECT().IsAuthorized(gomock.Any(), gomock.Any()).AnyTimes().Return(&proto.IsAuthorizedResponse{
				OwnerId:  initiatorID,
				ApiKeyId: "key-id",
				Username: "agent",
			}, nil)
			client.EXPECT().IsBudgetExceeded(gomock.Any(), gomock.Any()).AnyTimes().Return(&proto.IsBudgetExceededResponse{}, nil)
			client.EXPECT().GetMCPGatewayServerConfig(gomock.Any(), gomock.Any()).AnyTimes().Return(&proto.GetMCPGatewayServerConfigResponse{
				Found:  true,
				Config: testCase.config,
			}, nil)
			client.EXPECT().AuthorizeMCPGateway(gomock.Any(), gomock.Any()).AnyTimes().Return(&proto.AuthorizeMCPGatewayResponse{
				Authorized:  true,
				InitiatorId: initiatorID,
				ApiKeyId:    "key-id",
				Username:    "agent",
			}, nil)
			if testCase.credentialRPC != nil {
				client.EXPECT().GetMCPUpstreamCredential(gomock.Any(), &proto.GetMCPUpstreamCredentialRequest{
					Key:                    "coder-token",
					ExternalAuthProviderId: "github",
				}).Times(1).Return(testCase.credentialRPC, nil)
			}

			gateway := httptest.NewServer(srv)
			t.Cleanup(gateway.Close)

			requestCount := testCase.requestCount
			if requestCount == 0 {
				requestCount = 1
			}
			for range requestCount {
				response := mcpGatewayRequest(t, gateway.URL, `{"jsonrpc":"2.0","id":1,"method":"ping"}`)
				require.Contains(t, string(response), `"result":{}`)
			}
			require.EqualValues(t, requestCount, calls.Load())
		})
	}
}

func TestServeHTTP_MCPGatewayCredentialReauth(t *testing.T) {
	t.Parallel()

	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		upstreamCalls.Add(1)
	}))
	t.Cleanup(upstream.Close)

	srv, client, _ := newTestServer(t)
	initiatorID := uuid.NewString()
	client.EXPECT().IsAuthorized(gomock.Any(), gomock.Any()).AnyTimes().Return(&proto.IsAuthorizedResponse{OwnerId: initiatorID}, nil)
	client.EXPECT().IsBudgetExceeded(gomock.Any(), gomock.Any()).AnyTimes().Return(&proto.IsBudgetExceededResponse{}, nil)
	client.EXPECT().GetMCPGatewayServerConfig(gomock.Any(), gomock.Any()).Return(&proto.GetMCPGatewayServerConfigResponse{
		Found: true,
		Config: &proto.MCPGatewayServerConfig{
			Id:                     uuid.NewString(),
			Slug:                   "github",
			Url:                    upstream.URL,
			Transport:              "streamable_http",
			AuthType:               "external_auth",
			ExternalAuthProviderId: "github",
		},
	}, nil)
	client.EXPECT().AuthorizeMCPGateway(gomock.Any(), gomock.Any()).Return(&proto.AuthorizeMCPGatewayResponse{
		Authorized:  true,
		InitiatorId: initiatorID,
	}, nil)
	client.EXPECT().GetMCPUpstreamCredential(gomock.Any(), gomock.Any()).Return(&proto.GetMCPUpstreamCredentialResponse{
		ReauthRequired: true,
		ReauthUrl:      "https://coder.example/external-auth/github",
		ProviderId:     "github",
	}, nil)

	gateway := httptest.NewServer(srv)
	t.Cleanup(gateway.Close)
	response := mcpGatewayRequest(t, gateway.URL, `{"jsonrpc":"2.0","id":42,"method":"ping"}`)
	var decoded struct {
		ID    int `json:"id"`
		Error struct {
			Message string         `json:"message"`
			Data    map[string]any `json:"data"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(response, &decoded))
	require.Equal(t, 42, decoded.ID)
	require.Contains(t, decoded.Error.Message, "Authentication is required")
	require.Equal(t, "https://coder.example/external-auth/github", decoded.Error.Data["reauth_url"])
	require.Equal(t, "github", decoded.Error.Data["provider_id"])
	require.Zero(t, upstreamCalls.Load())
}

func TestServeHTTP_MCPGatewayExternalAuthRetry(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		attempt := calls.Add(1)
		if attempt == 1 {
			require.Equal(t, "Bearer expired-token", r.Header.Get("Authorization"))
			rw.WriteHeader(http.StatusUnauthorized)
			return
		}
		require.Equal(t, "Bearer refreshed-token", r.Header.Get("Authorization"))
		rw.Header().Set("Content-Type", "application/json")
		_, _ = rw.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"ok":true}}`))
	}))
	t.Cleanup(upstream.Close)

	srv, client, _ := newTestServer(t)
	initiatorID := uuid.NewString()
	client.EXPECT().IsAuthorized(gomock.Any(), gomock.Any()).AnyTimes().Return(&proto.IsAuthorizedResponse{OwnerId: initiatorID}, nil)
	client.EXPECT().IsBudgetExceeded(gomock.Any(), gomock.Any()).AnyTimes().Return(&proto.IsBudgetExceededResponse{}, nil)
	client.EXPECT().GetMCPGatewayServerConfig(gomock.Any(), gomock.Any()).Return(&proto.GetMCPGatewayServerConfigResponse{
		Found: true,
		Config: &proto.MCPGatewayServerConfig{
			Id:                     uuid.NewString(),
			Slug:                   "github",
			Url:                    upstream.URL,
			Transport:              "streamable_http",
			AuthType:               "external_auth",
			ExternalAuthProviderId: "github",
		},
	}, nil)
	client.EXPECT().AuthorizeMCPGateway(gomock.Any(), gomock.Any()).Return(&proto.AuthorizeMCPGatewayResponse{
		Authorized:  true,
		InitiatorId: initiatorID,
	}, nil)
	gomock.InOrder(
		client.EXPECT().GetMCPUpstreamCredential(gomock.Any(), gomock.Any()).Return(&proto.GetMCPUpstreamCredentialResponse{
			AccessToken: "expired-token",
			ProviderId:  "github",
		}, nil),
		client.EXPECT().GetMCPUpstreamCredential(gomock.Any(), gomock.Any()).Return(&proto.GetMCPUpstreamCredentialResponse{
			AccessToken: "refreshed-token",
			ProviderId:  "github",
		}, nil),
	)

	gateway := httptest.NewServer(srv)
	t.Cleanup(gateway.Close)
	response := mcpGatewayRequest(t, gateway.URL, `{"jsonrpc":"2.0","id":1,"method":"ping"}`)
	require.Contains(t, string(response), `"ok":true`)
	require.Equal(t, int32(2), calls.Load())
}

func TestServeHTTP_MCPGatewayUnsupportedCredentials(t *testing.T) {
	t.Parallel()

	for _, authType := range []string{"oauth2", "user_oidc"} {
		t.Run(authType, func(t *testing.T) {
			t.Parallel()

			var upstreamCalls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				upstreamCalls.Add(1)
			}))
			t.Cleanup(upstream.Close)

			srv, client, _ := newTestServer(t)
			initiatorID := uuid.NewString()
			client.EXPECT().IsAuthorized(gomock.Any(), gomock.Any()).AnyTimes().Return(&proto.IsAuthorizedResponse{OwnerId: initiatorID}, nil)
			client.EXPECT().IsBudgetExceeded(gomock.Any(), gomock.Any()).AnyTimes().Return(&proto.IsBudgetExceededResponse{}, nil)
			client.EXPECT().GetMCPGatewayServerConfig(gomock.Any(), gomock.Any()).Return(&proto.GetMCPGatewayServerConfigResponse{
				Found: true,
				Config: &proto.MCPGatewayServerConfig{
					Id:        uuid.NewString(),
					Slug:      "github",
					Url:       upstream.URL,
					Transport: "streamable_http",
					AuthType:  authType,
				},
			}, nil)
			client.EXPECT().AuthorizeMCPGateway(gomock.Any(), gomock.Any()).Return(&proto.AuthorizeMCPGatewayResponse{
				Authorized:  true,
				InitiatorId: initiatorID,
			}, nil)

			gateway := httptest.NewServer(srv)
			t.Cleanup(gateway.Close)
			response := mcpGatewayRequest(t, gateway.URL, `{"jsonrpc":"2.0","id":1,"method":"ping"}`)
			require.Contains(t, string(response), "does not support upstream auth type")
			require.Contains(t, string(response), authType)
			require.Zero(t, upstreamCalls.Load())
		})
	}
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
	request.Header.Set("X-Coder-AI-Governance-Token", "coder-token")
	request.Header.Set("X-Api-Key", "coder-token")
	request.Header.Set("Cookie", "coder_session_token=coder-token")
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusOK, response.StatusCode)
	responseBody, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	return responseBody
}
