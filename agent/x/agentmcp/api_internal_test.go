package agentmcp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
)

// TestHandleCallTool_ErrorMapping verifies the call-tool handler maps
// Manager errors to the right HTTP status codes. Tool discovery is no
// longer served over HTTP (the agentcontext manager reads the catalog
// in-process), so only the execution endpoint is exercised here.
func TestHandleCallTool_ErrorMapping(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, nil)
	m := NewManager(ctx, logger, agentexec.DefaultExecer, nil)
	t.Cleanup(func() { _ = m.Close() })

	api := NewAPI(m)

	cases := []struct {
		name     string
		toolName string
		wantCode int
	}{
		{name: "InvalidToolName", toolName: "noseparator", wantCode: http.StatusBadRequest},
		{name: "UnknownServer", toolName: "ghost__echo", wantCode: http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			body, err := json.Marshal(workspacesdk.CallMCPToolRequest{ToolName: tc.toolName})
			require.NoError(t, err)
			req := httptest.NewRequest(http.MethodPost, "/call-tool", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			api.Routes().ServeHTTP(rec, req)

			require.Equal(t, tc.wantCode, rec.Code)
		})
	}
}
