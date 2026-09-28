package agentfiles_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agentchat"
	"github.com/coder/coder/v2/agent/agentfiles"
	"github.com/coder/coder/v2/agent/agentgit"
	"github.com/coder/coder/v2/agent/agenttoolcall"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// fileToolCallMessageID is above the cutoff newFileToolCallAPI latches,
// so the store treats tool calls in it as never received.
const fileToolCallMessageID = 2

type fileRoute struct {
	name   string
	tool   string
	target string
	body   []byte
}

func fileRoutes(t *testing.T, filePath string) []fileRoute {
	t.Helper()

	editBody, err := json.Marshal(workspacesdk.FileEditRequest{
		Files: []workspacesdk.FileEdits{{
			Path:  filePath,
			Edits: []workspacesdk.FileEdit{{OldText: "one", NewText: "two"}},
		}},
		IncludeDiff: true,
	})
	require.NoError(t, err)
	return []fileRoute{
		{name: "EditFiles", tool: "edit_files", target: "/edit-files", body: editBody},
		{name: "WriteFile", tool: "write_file", target: "/write-file?path=" + url.QueryEscape(filePath), body: []byte("two\n")},
	}
}

// TestFileToolCallRunsOnce verifies that an edit or write with tool call
// headers changes the file once: a repeated request gets the recorded
// response without touching the file, and a cancel afterwards changes
// nothing. Without tool call headers every request applies.
func TestFileToolCallRunsOnce(t *testing.T) {
	t.Parallel()

	filePath := filepath.Join(os.TempDir(), "work", "file.txt")
	for _, route := range fileRoutes(t, filePath) {
		for _, toolCall := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/ToolCall=%t", route.name, toolCall), func(t *testing.T) {
				t.Parallel()

				handler, fs := newFileToolCallAPI(t)
				chatID := uuid.New()
				tc := workspacesdk.ToolCall{MessageID: fileToolCallMessageID, ID: "call", Name: route.tool}
				headers := http.Header{workspacesdk.CoderChatIDHeader: {chatID.String()}}
				if toolCall {
					tc.SetHeaders(headers)
				}
				require.NoError(t, afero.WriteFile(fs, filePath, []byte("one\n"), 0o644))

				first := serveFileRequest(t, handler, route.target, bytes.NewReader(route.body), headers)
				require.Equal(t, http.StatusOK, first.Code, first.Body.String())
				requireFileContent(t, fs, filePath, "two\n")

				// Undo the change so a second run would show on disk.
				require.NoError(t, afero.WriteFile(fs, filePath, []byte("one\n"), 0o644))
				again := serveFileRequest(t, handler, route.target, bytes.NewReader(route.body), headers)
				require.Equal(t, http.StatusOK, again.Code, again.Body.String())
				assert.Equal(t, first.Body.String(), again.Body.String())
				if !toolCall {
					requireFileContent(t, fs, filePath, "two\n")
					return
				}
				requireFileContent(t, fs, filePath, "one\n")

				id := workspacesdk.ToolCallUUID(chatID, tc.MessageID, tc.Name, tc.ID).String()
				canceled := serveFileRequest(t, handler, "/tool-calls/"+id+"/cancel", nil, headers)
				require.Equal(t, http.StatusNoContent, canceled.Code, canceled.Body.String())
				requireFileContent(t, fs, filePath, "one\n")
			})
		}
	}
}

// TestFileToolCallAbandon verifies that a request whose body cannot be
// read changes nothing and leaves no record, so the next request for the
// tool call runs, while a failure after the body was read is recorded.
func TestFileToolCallAbandon(t *testing.T) {
	t.Parallel()

	filePath := filepath.Join(os.TempDir(), "work", "file.txt")
	routes := fileRoutes(t, filePath)
	edit, write := routes[0], routes[1]

	tests := []struct {
		name  string
		route fileRoute
		// body is the first request's body.
		body io.Reader
		// wantStatus is the first request's status.
		wantStatus int
	}{
		{
			name:       "EditInvalidJSON",
			route:      edit,
			body:       strings.NewReader(`{"files":`),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "EditBodyCutOff",
			route:      edit,
			body:       io.MultiReader(bytes.NewReader(edit.body[:10]), iotestErrReader{}),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "WriteBodyCutOff",
			route:      write,
			body:       io.MultiReader(strings.NewReader("tw"), iotestErrReader{}),
			wantStatus: http.StatusInternalServerError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			handler, fs := newFileToolCallAPI(t)
			headers := http.Header{workspacesdk.CoderChatIDHeader: {uuid.NewString()}}
			workspacesdk.ToolCall{MessageID: fileToolCallMessageID, ID: "call", Name: tt.route.tool}.SetHeaders(headers)
			require.NoError(t, afero.WriteFile(fs, filePath, []byte("one\n"), 0o644))

			first := serveFileRequest(t, handler, tt.route.target, tt.body, headers)
			require.Equal(t, tt.wantStatus, first.Code, first.Body.String())
			requireFileContent(t, fs, filePath, "one\n")

			retry := serveFileRequest(t, handler, tt.route.target, bytes.NewReader(tt.route.body), headers)
			require.Equal(t, http.StatusOK, retry.Code, retry.Body.String())
			requireFileContent(t, fs, filePath, "two\n")
		})
	}

	t.Run("EditErrorRecorded", func(t *testing.T) {
		t.Parallel()

		handler, fs := newFileToolCallAPI(t)
		headers := http.Header{workspacesdk.CoderChatIDHeader: {uuid.NewString()}}
		workspacesdk.ToolCall{MessageID: fileToolCallMessageID, ID: "call", Name: edit.tool}.SetHeaders(headers)
		require.NoError(t, afero.WriteFile(fs, filePath, []byte("three\n"), 0o644))

		first := serveFileRequest(t, handler, edit.target, bytes.NewReader(edit.body), headers)
		require.Equal(t, http.StatusBadRequest, first.Code, first.Body.String())

		// The edit would apply now, but the tool call already has its
		// answer.
		require.NoError(t, afero.WriteFile(fs, filePath, []byte("one\n"), 0o644))
		again := serveFileRequest(t, handler, edit.target, bytes.NewReader(edit.body), headers)
		require.Equal(t, http.StatusBadRequest, again.Code, again.Body.String())
		assert.Equal(t, first.Body.String(), again.Body.String())
		requireFileContent(t, fs, filePath, "one\n")
	})
}

// iotestErrReader fails every read, like a request body whose connection
// dropped.
type iotestErrReader struct{}

func (iotestErrReader) Read([]byte) (int, error) {
	return 0, xerrors.New("connection reset")
}

// newFileToolCallAPI returns a handler that serves the file routes and
// the cancel route the way the agent mounts them, with a cutoff message
// ID below fileToolCallMessageID.
func newFileToolCallAPI(t *testing.T) (http.Handler, afero.Fs) {
	t.Helper()

	store := agenttoolcall.NewStore(quartz.NewMock(t))
	store.SetLastChatMessageID(fileToolCallMessageID - 1)
	fs := afero.NewMemMapFs()
	api := agentfiles.NewAPI(slogtest.Make(t, nil), fs, agentgit.NewPathStore(), agentfiles.WithToolCallStore(store))

	router := chi.NewRouter()
	router.Post("/tool-calls/{id}/cancel", store.CancelHandler())
	router.Mount("/", api.Routes())
	return agentchat.Middleware(router), fs
}

func serveFileRequest(t *testing.T, handler http.Handler, target string, body io.Reader, headers http.Header) *httptest.ResponseRecorder {
	t.Helper()

	w := httptest.NewRecorder()
	r := httptest.NewRequestWithContext(testutil.Context(t, testutil.WaitShort), http.MethodPost, target, body)
	for k, v := range headers {
		r.Header[k] = v
	}
	handler.ServeHTTP(w, r)
	return w
}

func requireFileContent(t *testing.T, fs afero.Fs, path, want string) {
	t.Helper()

	got, err := afero.ReadFile(fs, path)
	require.NoError(t, err)
	require.Equal(t, want, string(got))
}
