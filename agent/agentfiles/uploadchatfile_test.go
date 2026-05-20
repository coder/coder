package agentfiles_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agentfiles"
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
	r.Delete("/delete-chat-files", api.HandleDeleteChatFiles)
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

	var resp workspacesdk.UploadChatFileResponse
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

	var resp workspacesdk.UploadChatFileResponse
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

	uploadOnce := func(t *testing.T, body string) workspacesdk.UploadChatFileResponse {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, uploadChatFileURL(uploadChatFileTestChatID, "archive.zip"),
			strings.NewReader(body))
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		var resp workspacesdk.UploadChatFileResponse
		require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
		return resp
	}

	first := uploadOnce(t, "first")
	second := uploadOnce(t, "second")
	third := uploadOnce(t, "third")

	require.Equal(t, "archive.zip", first.Name)
	require.Equal(t, "archive_2.zip", second.Name)
	require.Equal(t, "archive_3.zip", third.Name)

	for _, r := range []workspacesdk.UploadChatFileResponse{first, second, third} {
		_, err := os.Stat(r.Path)
		require.NoError(t, err)
	}
}

func deleteChatFilesURL(chatID string) string {
	q := url.Values{}
	if chatID != "" {
		q.Set("chat_id", chatID)
	}
	return "/delete-chat-files?" + q.Encode()
}

func TestHandleDeleteChatFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	fs := afero.NewOsFs()
	r := newUploadChatFileRouter(t, fs)

	chatDir := filepath.Join(home, ".coder", "chats", uploadChatFileTestChatID)
	filePath := filepath.Join(chatDir, "files", "archive.zip")
	require.NoError(t, os.MkdirAll(filepath.Dir(filePath), 0o755))
	require.NoError(t, os.WriteFile(filePath, []byte("zip bytes"), 0o600))

	req := httptest.NewRequest(http.MethodDelete, deleteChatFilesURL(uploadChatFileTestChatID), nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	_, err := os.Stat(chatDir)
	require.True(t, os.IsNotExist(err), "expected chat upload directory to be deleted")

	// Repeating cleanup for a missing directory is a successful no-op.
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
}

func TestHandleDeleteChatFiles_BadRequest(t *testing.T) {
	t.Parallel()

	fs := afero.NewOsFs()
	r := newUploadChatFileRouter(t, fs)

	tests := []struct {
		name    string
		chatID  string
		wantSub string
	}{
		{name: "missing_chat_id", chatID: "", wantSub: "chat_id"},
		{name: "chat_id_path_traversal", chatID: "../etc", wantSub: "chat_id"},
		{name: "chat_id_with_slash", chatID: "a/b", wantSub: "chat_id"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodDelete, deleteChatFilesURL(tt.chatID), nil)
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
			require.Contains(t, strings.ToLower(rr.Body.String()), tt.wantSub)
		})
	}
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
