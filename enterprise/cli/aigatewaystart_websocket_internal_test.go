//go:build !slim

package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/coderd/aibridged"
	mock "github.com/coder/coder/v2/coderd/aibridged/aibridgedmock"
	"github.com/coder/coder/v2/coderd/aibridged/proto"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
	"github.com/coder/websocket"
)

// TestStandaloneGatewayResponsesWebSocket requires that the standalone
// gateway's HTTP stack serves Responses WebSocket mode for a user with the
// experiment on, and refuses the upgrade for a user without it.
func TestStandaloneGatewayResponsesWebSocket(t *testing.T) {
	t.Parallel()

	for _, enabled := range []bool{true, false} {
		name := "ExperimentOff"
		if enabled {
			name = "ExperimentOn"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)

			upstreams := make(chan *websocket.Conn, 1)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := websocket.Accept(w, r, nil)
				if err != nil {
					t.Errorf("upstream accept: %v", err)
					return
				}
				upstreams <- conn
			}))
			t.Cleanup(upstream.Close)

			client := mock.NewMockDRPCClient(gomock.NewController(t))
			client.EXPECT().DRPCConn().AnyTimes().Return(&connectedDRPCConn{closed: make(chan struct{})})
			client.EXPECT().IsAuthorized(gomock.Any(), gomock.Any()).AnyTimes().Return(&proto.IsAuthorizedResponse{
				OwnerId:                   uuid.NewString(),
				ResponsesWebsocketEnabled: enabled,
			}, nil)
			client.EXPECT().IsBudgetExceeded(gomock.Any(), gomock.Any()).AnyTimes().Return(&proto.IsBudgetExceededResponse{}, nil)
			client.EXPECT().GetMCPServerConfigs(gomock.Any(), gomock.Any()).AnyTimes().Return(&proto.GetMCPServerConfigsResponse{}, nil)
			client.EXPECT().RecordInterception(gomock.Any(), gomock.Any()).AnyTimes().Return(&proto.RecordInterceptionResponse{}, nil)
			client.EXPECT().RecordInterceptionEnded(gomock.Any(), gomock.Any()).AnyTimes().Return(&proto.RecordInterceptionEndedResponse{}, nil)
			client.EXPECT().RecordPromptUsage(gomock.Any(), gomock.Any()).AnyTimes().Return(&proto.RecordPromptUsageResponse{}, nil)

			gateway, _ := newTestStandaloneGateway(t, func(p *standaloneGatewayParams) {
				p.dialer = func(context.Context) (aibridged.DRPCClient, error) { return client, nil }
			})
			pool, err := keypool.New("openai", []string{"sk-standalone"}, quartz.NewReal(), nil)
			require.NoError(t, err)
			require.NoError(t, gateway.daemon.ReplaceProviders(ctx, []aibridge.Provider{
				aibridge.NewOpenAIProvider(aibridge.OpenAIConfig{BaseURL: upstream.URL, KeyPool: pool}),
			}))
			front := httptest.NewServer(gateway.httpServer.Handler)
			t.Cleanup(front.Close)

			//nolint:bodyclose // Dial owns the response body.
			conn, resp, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(front.URL, "http")+"/api/v2/ai-gateway/openai/v1/responses", &websocket.DialOptions{
				HTTPHeader: http.Header{"Authorization": {"Bearer coder-token"}},
			})
			require.NotNil(t, resp)
			if !enabled {
				require.Error(t, err)
				assert.Equal(t, http.StatusNotImplemented, resp.StatusCode)
				assert.Empty(t, upstreams)
				return
			}
			require.NoError(t, err)
			defer conn.CloseNow()
			up := testutil.TryReceive(ctx, t, upstreams)
			defer up.CloseNow()

			const create = `{"type":"response.create","model":"gpt-5","input":"hi"}`
			require.NoError(t, conn.Write(ctx, websocket.MessageText, []byte(create)))
			_, msg, err := up.Read(ctx)
			require.NoError(t, err)
			assert.Equal(t, create, string(msg))

			// Let both sides finish closing before the gateway shuts down.
			go func() { _, _, _ = up.Read(ctx) }()
			require.NoError(t, conn.Close(websocket.StatusNormalClosure, ""))
		})
	}
}
