package agentproc

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agentchat"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
)

// TestToolCallProcessChat covers attaching to and killing a tool call's
// process across chats. Through the real agent a repeated start never
// reaches the process manager, since the tool call table answers it.
func TestToolCallProcessChat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		sameChat  bool
		wantStart int
		wantKill  bool
	}{
		{name: "SameChat", sameChat: true, wantStart: http.StatusOK, wantKill: true},
		{name: "OtherChat", wantStart: http.StatusInternalServerError},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			api := NewAPI(slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}), agentexec.DefaultExecer, nil, nil, nil, nil, nil)
			t.Cleanup(func() { _ = api.Close() })
			handler := agentchat.Middleware(api.Routes())
			owner, id := uuid.New(), uuid.New()
			requester := uuid.New()
			if tc.sameChat {
				requester = owner
			}
			start := func(chatID uuid.UUID) *httptest.ResponseRecorder {
				body, err := json.Marshal(workspacesdk.StartProcessRequest{Command: "sleep 300"})
				require.NoError(t, err)
				req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/start", bytes.NewReader(body))
				req.Header.Set(workspacesdk.CoderChatIDHeader, chatID.String())
				req.Header.Set(workspacesdk.CoderToolCallIDHeader, id.String())
				rw := httptest.NewRecorder()
				handler.ServeHTTP(rw, req)
				return rw
			}

			require.Equal(t, http.StatusOK, start(owner).Code)
			rw := start(requester)
			require.Equal(t, tc.wantStart, rw.Code)
			if tc.wantStart == http.StatusOK {
				var resp workspacesdk.StartProcessResponse
				require.NoError(t, json.NewDecoder(rw.Body).Decode(&resp))
				require.Equal(t, id.String(), resp.ID, "a repeated start attaches to the process")
			}

			api.KillToolCall(ctx, requester, id)
			proc, ok := api.manager.get(id.String())
			require.True(t, ok)
			if tc.wantKill {
				testutil.TryReceive(ctx, t, proc.done)
				require.True(t, proc.canceled.Load())
				return
			}
			require.True(t, proc.info().Running)
			require.False(t, proc.canceled.Load())
		})
	}
}
