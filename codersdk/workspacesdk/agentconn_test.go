//nolint:testpackage // This test exercises the internal query builder directly because agent requests need a live tailnet connection.
package workspacesdk

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	neturl "net/url"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

func TestAgentAPIPath(t *testing.T) {
	t.Parallel()

	t.Run("encodes reserved query characters", func(t *testing.T) {
		t.Parallel()

		path := "/tmp/a&b ?#%c.md"
		got := agentAPIPath("/api/v0/resolve-path", neturl.Values{
			"path": []string{path},
		})

		parsed, err := neturl.Parse(got)
		require.NoError(t, err)
		require.Equal(t, "/api/v0/resolve-path", parsed.Path)
		require.Equal(t, path, parsed.Query().Get("path"))
	})

	t.Run("preserves all query values", func(t *testing.T) {
		t.Parallel()

		got := agentAPIPath("/api/v0/read-file-lines", neturl.Values{
			"path":               []string{"/tmp/plan v1#.md"},
			"offset":             []string{"10"},
			"limit":              []string{"20"},
			"max_file_size":      []string{"30"},
			"max_line_bytes":     []string{"40"},
			"max_response_lines": []string{"50"},
			"max_response_bytes": []string{"60"},
		})

		parsed, err := neturl.Parse(got)
		require.NoError(t, err)
		require.Equal(t, "/api/v0/read-file-lines", parsed.Path)
		require.Equal(t, "/tmp/plan v1#.md", parsed.Query().Get("path"))
		require.Equal(t, "10", parsed.Query().Get("offset"))
		require.Equal(t, "20", parsed.Query().Get("limit"))
		require.Equal(t, "30", parsed.Query().Get("max_file_size"))
		require.Equal(t, "40", parsed.Query().Get("max_line_bytes"))
		require.Equal(t, "50", parsed.Query().Get("max_response_lines"))
		require.Equal(t, "60", parsed.Query().Get("max_response_bytes"))
	})
}

type recordedAgentConnRequest struct {
	Method   string
	Path     string
	RawQuery string
	Body     []byte
}

func TestAgentConn_HTTPAPIURL(t *testing.T) {
	t.Parallel()

	var (
		mu       sync.Mutex
		requests []recordedAgentConnRequest
	)

	recordRequest := func(r *http.Request) []byte {
		t.Helper()
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, r.Body.Close())

		mu.Lock()
		requests = append(requests, recordedAgentConnRequest{
			Method:   r.Method,
			Path:     r.URL.Path,
			RawQuery: r.URL.RawQuery,
			Body:     body,
		})
		mu.Unlock()
		return body
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := recordRequest(r)
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/api/v0/processes/start":
			var req StartProcessRequest
			require.NoError(t, json.Unmarshal(body, &req))
			require.Equal(t, "echo hello", req.Command)
			require.NoError(t, json.NewEncoder(w).Encode(StartProcessResponse{
				ID:      "proc-1",
				Started: true,
			}))
		case "/api/v0/processes/list":
			require.NoError(t, json.NewEncoder(w).Encode(ListProcessesResponse{
				Processes: []ProcessInfo{{
					ID:      "proc-1",
					Command: "echo hello",
					Running: true,
				}},
			}))
		case "/api/v0/processes/proc-1/output":
			require.Equal(t, "wait=true", r.URL.RawQuery)
			require.NoError(t, json.NewEncoder(w).Encode(ProcessOutputResponse{
				Output:  "hello\n",
				Running: false,
			}))
		case "/api/v0/processes/proc-1/signal":
			var req SignalProcessRequest
			require.NoError(t, json.Unmarshal(body, &req))
			require.Equal(t, "TERM", req.Signal)
			require.NoError(t, json.NewEncoder(w).Encode(codersdk.Response{Message: "signaled"}))
		case "/api/v0/read-file-lines":
			query := r.URL.Query()
			require.Equal(t, "/tmp/file.txt", query.Get("path"))
			require.Equal(t, "0", query.Get("offset"))
			require.Equal(t, "20", query.Get("limit"))
			require.Equal(t, "1048576", query.Get("max_file_size"))
			require.Equal(t, "1024", query.Get("max_line_bytes"))
			require.Equal(t, "2000", query.Get("max_response_lines"))
			require.Equal(t, "32768", query.Get("max_response_bytes"))
			require.NoError(t, json.NewEncoder(w).Encode(ReadFileLinesResponse{
				Success:    true,
				FileSize:   6,
				TotalLines: 1,
				LinesRead:  1,
				Content:    "1\thello",
			}))
		case "/api/v0/write-file":
			require.Equal(t, "/tmp/file.txt", r.URL.Query().Get("path"))
			require.Equal(t, "hello", string(body))
			require.NoError(t, json.NewEncoder(w).Encode(codersdk.Response{Message: "written"}))
		case "/api/v0/edit-files":
			var req FileEditRequest
			require.NoError(t, json.Unmarshal(body, &req))
			require.Len(t, req.Files, 1)
			require.Equal(t, "/tmp/file.txt", req.Files[0].Path)
			require.NoError(t, json.NewEncoder(w).Encode(codersdk.Response{Message: "edited"}))
		case "/api/v0/desktop/action":
			var req DesktopAction
			require.NoError(t, json.Unmarshal(body, &req))
			require.Equal(t, "click", req.Action)
			require.NoError(t, json.NewEncoder(w).Encode(DesktopActionResponse{
				Output:           "clicked",
				ScreenshotData:   "png-bytes",
				ScreenshotWidth:  1280,
				ScreenshotHeight: 800,
			}))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	conn := NewAgentConn(nil, AgentConnOptions{
		AgentID:    uuid.New(),
		HTTPAPIURL: srv.URL,
		CloseFunc: func() error {
			return ErrSkipClose
		},
	})

	ctx := context.Background()
	require.True(t, conn.AwaitReachable(ctx))

	startResp, err := conn.StartProcess(ctx, StartProcessRequest{Command: "echo hello"})
	require.NoError(t, err)
	require.Equal(t, StartProcessResponse{ID: "proc-1", Started: true}, startResp)

	listResp, err := conn.ListProcesses(ctx)
	require.NoError(t, err)
	require.Len(t, listResp.Processes, 1)
	require.Equal(t, "proc-1", listResp.Processes[0].ID)

	outputResp, err := conn.ProcessOutput(ctx, "proc-1", &ProcessOutputOptions{Wait: true})
	require.NoError(t, err)
	require.Equal(t, "hello\n", outputResp.Output)

	require.NoError(t, conn.SignalProcess(ctx, "proc-1", "TERM"))

	readResp, err := conn.ReadFileLines(ctx, "/tmp/file.txt", 0, 20, DefaultReadFileLinesLimits())
	require.NoError(t, err)
	require.True(t, readResp.Success)
	require.Equal(t, "1\thello", readResp.Content)

	require.NoError(t, conn.WriteFile(ctx, "/tmp/file.txt", strings.NewReader("hello")))

	editResp, err := conn.EditFiles(ctx, FileEditRequest{
		Files: []FileEdits{{
			Path: "/tmp/file.txt",
			Edits: []FileEdit{{
				Search:  "hello",
				Replace: "goodbye",
			}},
		}},
	})
	require.NoError(t, err)
	require.NotNil(t, editResp)

	actionResp, err := conn.ExecuteDesktopAction(ctx, DesktopAction{Action: "click"})
	require.NoError(t, err)
	require.Equal(t, "clicked", actionResp.Output)
	require.Equal(t, "png-bytes", actionResp.ScreenshotData)

	require.NoError(t, conn.Close())

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, requests, 8)
	require.Equal(t, recordedAgentConnRequest{
		Method: http.MethodPost,
		Path:   "/api/v0/processes/start",
		Body:   []byte("{\"command\":\"echo hello\"}\n"),
	}, recordedAgentConnRequest{
		Method: requests[0].Method,
		Path:   requests[0].Path,
		Body:   requests[0].Body,
	})
}
