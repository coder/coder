package agentfiles_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/go-chi/chi/v5"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agentfiles"
	"github.com/coder/coder/v2/coderd/x/chatfiles"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

const uploadChatFileTestChatID = "00000000-0000-0000-0000-000000000001"

// newUploadChatFileRouter wires the upload handler against the
// supplied filesystem so each test gets its own router and fs.
func newUploadChatFileRouter(t *testing.T, fs afero.Fs) http.Handler {
	t.Helper()
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	api := agentfiles.NewAPI(logger, fs, nil)
	r := chi.NewRouter()
	r.Post("/upload-chat-file", api.HandleUploadChatFile)
	return r
}

func uploadChatFileURL(chatID, name string) string {
	q := url.Values{}
	if chatID != "" {
		q.Set("chat_id", chatID)
	}
	if name != "" {
		q.Set("name", name)
	}
	return "/upload-chat-file?" + q.Encode()
}

func TestHandleUploadChatFile_HappyPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// macOS resolves $HOME through getpwuid by default; we already
	// override it but USERPROFILE is read on Windows.
	t.Setenv("USERPROFILE", home)

	fs := afero.NewOsFs()
	r := newUploadChatFileRouter(t, fs)

	body := strings.NewReader("zip bytes")
	req := httptest.NewRequest(http.MethodPost, uploadChatFileURL(uploadChatFileTestChatID, "archive.zip"), body)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())

	var resp workspacesdk.AgentUploadChatFileResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))

	want := filepath.Join(home, ".coder", "chats", uploadChatFileTestChatID, "files", "archive.zip")
	require.Equal(t, want, resp.Path)
	require.Equal(t, "archive.zip", resp.Name)
	require.Equal(t, int64(len("zip bytes")), resp.Size)

	contents, err := afero.ReadFile(fs, resp.Path)
	require.NoError(t, err)
	require.Equal(t, "zip bytes", string(contents))
}

func TestHandleUploadChatFile_SanitizesName(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	fs := afero.NewOsFs()
	r := newUploadChatFileRouter(t, fs)

	body := strings.NewReader("payload")
	// Path components and unsafe whitespace must be stripped before
	// the file lands on disk.
	req := httptest.NewRequest(http.MethodPost, uploadChatFileURL(uploadChatFileTestChatID, "../etc/secret file.zip"), body)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())

	var resp workspacesdk.AgentUploadChatFileResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))

	require.Equal(t, "secret_file.zip", resp.Name)
	require.Equal(t, filepath.Join(home, ".coder", "chats", uploadChatFileTestChatID, "files", "secret_file.zip"), resp.Path)
}

func TestHandleUploadChatFile_CollisionSuffix(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	fs := afero.NewOsFs()
	r := newUploadChatFileRouter(t, fs)

	uploadOnce := func(t *testing.T, body string) workspacesdk.AgentUploadChatFileResponse {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, uploadChatFileURL(uploadChatFileTestChatID, "archive.zip"),
			strings.NewReader(body))
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		var resp workspacesdk.AgentUploadChatFileResponse
		require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
		return resp
	}

	first := uploadOnce(t, "first")
	second := uploadOnce(t, "second")
	third := uploadOnce(t, "third")

	require.Equal(t, "archive.zip", first.Name)
	require.Equal(t, "archive_2.zip", second.Name)
	require.Equal(t, "archive_3.zip", third.Name)

	for _, tt := range []struct {
		resp workspacesdk.AgentUploadChatFileResponse
		body string
	}{
		{resp: first, body: "first"},
		{resp: second, body: "second"},
		{resp: third, body: "third"},
	} {
		contents, err := os.ReadFile(tt.resp.Path)
		require.NoError(t, err)
		require.Equal(t, tt.body, string(contents))
	}
}

func TestHandleUploadChatFile_CollisionExhausted(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	fs := afero.NewOsFs()
	r := newUploadChatFileRouter(t, fs)
	dir := chatfiles.WorkspaceUploadDir(home, uploadChatFileTestChatID)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	for i := 1; i <= 1000; i++ {
		name := chatfiles.AddCollisionSuffix("archive.zip", i)
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(name), 0o600))
	}

	req := httptest.NewRequest(http.MethodPost, uploadChatFileURL(uploadChatFileTestChatID, "archive.zip"), strings.NewReader("payload"))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	require.Equal(t, http.StatusConflict, rr.Code, rr.Body.String())
	require.Contains(t, rr.Body.String(), "too many existing files")
}

func TestHandleUploadChatFile_RejectsSymlinkedUploadDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks requires elevated privileges on Windows")
	}

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	fs := afero.NewOsFs()
	r := newUploadChatFileRouter(t, fs)
	target := t.TempDir()
	chatParent := filepath.Join(home, ".coder", "chats")
	require.NoError(t, os.MkdirAll(chatParent, 0o755))
	require.NoError(t, os.Symlink(target, filepath.Join(chatParent, uploadChatFileTestChatID)))

	req := httptest.NewRequest(http.MethodPost,
		uploadChatFileURL(uploadChatFileTestChatID, "archive.zip"),
		strings.NewReader("payload"),
	)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	require.Equal(t, http.StatusForbidden, rr.Code, rr.Body.String())
	require.Contains(t, rr.Body.String(), "symlink")
	_, err := os.Stat(filepath.Join(target, "files", "archive.zip"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestHandleUploadChatFile_WriteErrorRemovesPartialFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	fs := afero.NewOsFs()
	r := newUploadChatFileRouter(t, fs)

	req := httptest.NewRequest(http.MethodPost,
		uploadChatFileURL(uploadChatFileTestChatID, "archive.zip"),
		iotest.ErrReader(os.ErrClosed),
	)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code, rr.Body.String())

	dir := chatfiles.WorkspaceUploadDir(home, uploadChatFileTestChatID)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestHandleUploadChatFile_BadRequest(t *testing.T) {
	t.Parallel()

	fs := afero.NewOsFs()
	r := newUploadChatFileRouter(t, fs)

	tests := []struct {
		name     string
		chatID   string
		filename string
		wantSub  string
	}{
		{name: "missing_chat_id", chatID: "", filename: "foo.zip", wantSub: "chat_id"},
		{name: "missing_name", chatID: uploadChatFileTestChatID, filename: "", wantSub: "name"},
		{name: "name_only_dots", chatID: uploadChatFileTestChatID, filename: "....", wantSub: "required"},
		{name: "name_only_whitespace", chatID: uploadChatFileTestChatID, filename: "   ", wantSub: "required"},
		{name: "chat_id_path_traversal", chatID: "../etc", filename: "foo.zip", wantSub: "chat_id"},
		{name: "chat_id_with_slash", chatID: "a/b", filename: "foo.zip", wantSub: "chat_id"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodPost, uploadChatFileURL(tt.chatID, tt.filename), strings.NewReader("x"))
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
			require.Contains(t, strings.ToLower(rr.Body.String()), tt.wantSub)
		})
	}
}
