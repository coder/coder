package agentfiles_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agentfiles"
	"github.com/coder/coder/v2/agent/usershell"
	"github.com/coder/coder/v2/coderd/x/chatfiles"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
)

const uploadChatFileTestChatID = "00000000-0000-0000-0000-000000000001"

// pathOnlyOsFs hides the afero.OsFs type so uploads take the portable
// path-based fallback while still exercising the real filesystem.
type pathOnlyOsFs struct{ afero.OsFs }

var uploadChatFileFilesystems = []struct {
	name string
	fs   afero.Fs
}{
	{name: "Secure", fs: afero.NewOsFs()},
	{name: "PathFallback", fs: pathOnlyOsFs{}},
}

type readerFunc func(p []byte) (int, error)

func (f readerFunc) Read(p []byte) (int, error) { return f(p) }

// newUploadChatFileRouter wires the upload handler against the
// supplied filesystem and home directory so each test is isolated.
func newUploadChatFileRouter(t *testing.T, fs afero.Fs, home string) http.Handler {
	t.Helper()
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	api := agentfiles.NewAPI(logger, fs, nil, agentfiles.WithEnvInfo(fakeBundleEnvInfo{home: home}))
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

func uploadChatFile(t *testing.T, h http.Handler, name string, body io.Reader) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, uploadChatFileURL(uploadChatFileTestChatID, name), body)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func decodeUploadChatFileResponse(t *testing.T, rr *httptest.ResponseRecorder) workspacesdk.AgentUploadChatFileResponse {
	t.Helper()
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	var resp workspacesdk.AgentUploadChatFileResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	return resp
}

func TestHandleUploadChatFile_HappyPath(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	fs := afero.NewOsFs()
	r := newUploadChatFileRouter(t, fs, home)

	resp := decodeUploadChatFileResponse(t, uploadChatFile(t, r, "archive.zip", strings.NewReader("zip bytes")))

	want := filepath.Join(home, ".coder", "chats", uploadChatFileTestChatID, "files", "archive.zip")
	require.Equal(t, want, resp.Path)
	require.Equal(t, "archive.zip", resp.Name)
	require.Equal(t, int64(len("zip bytes")), resp.Size)

	contents, err := afero.ReadFile(fs, resp.Path)
	require.NoError(t, err)
	require.Equal(t, "zip bytes", string(contents))
}

func TestHandleUploadChatFile_SanitizesName(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	r := newUploadChatFileRouter(t, afero.NewOsFs(), home)

	// Path components and unsafe whitespace must be stripped before
	// the file lands on disk.
	resp := decodeUploadChatFileResponse(t, uploadChatFile(t, r, "../etc/secret file.zip", strings.NewReader("payload")))

	require.Equal(t, "secret_file.zip", resp.Name)
	require.Equal(t, filepath.Join(home, ".coder", "chats", uploadChatFileTestChatID, "files", "secret_file.zip"), resp.Path)
}

func TestHandleUploadChatFile_CollisionSuffix(t *testing.T) {
	t.Parallel()

	for _, tc := range uploadChatFileFilesystems {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			home := t.TempDir()
			r := newUploadChatFileRouter(t, tc.fs, home)

			first := decodeUploadChatFileResponse(t, uploadChatFile(t, r, "archive.zip", strings.NewReader("first")))
			second := decodeUploadChatFileResponse(t, uploadChatFile(t, r, "archive.zip", strings.NewReader("second")))
			third := decodeUploadChatFileResponse(t, uploadChatFile(t, r, "archive.zip", strings.NewReader("third")))

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

			entries, err := os.ReadDir(chatfiles.WorkspaceUploadDir(home, uploadChatFileTestChatID))
			require.NoError(t, err)
			require.Len(t, entries, 3, "temp files must not outlive a successful upload")
		})
	}
}

func TestHandleUploadChatFile_CollisionExhausted(t *testing.T) {
	t.Parallel()

	for _, tc := range uploadChatFileFilesystems {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			home := t.TempDir()
			r := newUploadChatFileRouter(t, tc.fs, home)
			dir := chatfiles.WorkspaceUploadDir(home, uploadChatFileTestChatID)
			require.NoError(t, os.MkdirAll(dir, 0o755))
			for i := 1; i <= 1000; i++ {
				name := chatfiles.AddCollisionSuffix("archive.zip", i)
				require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(name), 0o600))
			}

			rr := uploadChatFile(t, r, "archive.zip", strings.NewReader("payload"))

			require.Equal(t, http.StatusConflict, rr.Code, rr.Body.String())
			require.Contains(t, rr.Body.String(), "too many existing files")
			entries, err := os.ReadDir(dir)
			require.NoError(t, err)
			require.Len(t, entries, 1000)
		})
	}
}

func TestHandleUploadChatFile_RejectsSymlinkedUploadDir(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks requires elevated privileges on Windows")
	}

	for _, tc := range uploadChatFileFilesystems {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			home := t.TempDir()
			r := newUploadChatFileRouter(t, tc.fs, home)
			target := t.TempDir()
			chatParent := filepath.Join(home, ".coder", "chats")
			require.NoError(t, os.MkdirAll(chatParent, 0o755))
			require.NoError(t, os.Symlink(target, filepath.Join(chatParent, uploadChatFileTestChatID)))

			rr := uploadChatFile(t, r, "archive.zip", strings.NewReader("payload"))

			require.Equal(t, http.StatusForbidden, rr.Code, rr.Body.String())
			require.Contains(t, rr.Body.String(), "symlink")
			entries, err := os.ReadDir(target)
			require.NoError(t, err)
			require.Empty(t, entries)
		})
	}
}

func TestHandleUploadChatFile_SkipsSymlinkedTargetName(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks requires elevated privileges on Windows")
	}

	for _, tc := range uploadChatFileFilesystems {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			home := t.TempDir()
			r := newUploadChatFileRouter(t, tc.fs, home)
			outside := filepath.Join(t.TempDir(), "outside.zip")
			require.NoError(t, os.WriteFile(outside, []byte("original"), 0o600))
			dir := chatfiles.WorkspaceUploadDir(home, uploadChatFileTestChatID)
			require.NoError(t, os.MkdirAll(dir, 0o755))
			require.NoError(t, os.Symlink(outside, filepath.Join(dir, "archive.zip")))

			resp := decodeUploadChatFileResponse(t, uploadChatFile(t, r, "archive.zip", strings.NewReader("payload")))

			require.Equal(t, "archive_2.zip", resp.Name)
			contents, err := os.ReadFile(resp.Path)
			require.NoError(t, err)
			require.Equal(t, "payload", string(contents))
			contents, err = os.ReadFile(outside)
			require.NoError(t, err)
			require.Equal(t, "original", string(contents))
		})
	}
}

func TestHandleUploadChatFile_WriteErrorRemovesPartialFile(t *testing.T) {
	t.Parallel()

	for _, tc := range uploadChatFileFilesystems {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			home := t.TempDir()
			r := newUploadChatFileRouter(t, tc.fs, home)

			body := io.MultiReader(strings.NewReader("partial"), iotest.ErrReader(os.ErrClosed))
			rr := uploadChatFile(t, r, "archive.zip", body)

			require.Equal(t, http.StatusInternalServerError, rr.Code, rr.Body.String())
			entries, err := os.ReadDir(chatfiles.WorkspaceUploadDir(home, uploadChatFileTestChatID))
			require.NoError(t, err)
			require.Empty(t, entries)
		})
	}
}

func TestHandleUploadChatFile_NotVisibleBeforeComplete(t *testing.T) {
	t.Parallel()

	for _, tc := range uploadChatFileFilesystems {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			home := t.TempDir()
			r := newUploadChatFileRouter(t, tc.fs, home)
			dir := chatfiles.WorkspaceUploadDir(home, uploadChatFileTestChatID)

			checked := false
			body := io.MultiReader(strings.NewReader("first chunk "), readerFunc(func(p []byte) (int, error) {
				if !checked {
					checked = true
					_, err := os.Lstat(filepath.Join(dir, "archive.zip"))
					require.ErrorIs(t, err, os.ErrNotExist)
					entries, err := os.ReadDir(dir)
					require.NoError(t, err)
					require.Len(t, entries, 1)
					require.True(t, strings.HasPrefix(entries[0].Name(), ".archive.zip.upload-"), entries[0].Name())
				}
				return copy(p, "second chunk"), io.EOF
			}))

			resp := decodeUploadChatFileResponse(t, uploadChatFile(t, r, "archive.zip", body))

			require.True(t, checked)
			contents, err := os.ReadFile(resp.Path)
			require.NoError(t, err)
			require.Equal(t, "first chunk second chunk", string(contents))
		})
	}
}

func TestHandleUploadChatFile_SweepsStaleTempFiles(t *testing.T) {
	t.Parallel()

	for _, tc := range uploadChatFileFilesystems {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			home := t.TempDir()
			r := newUploadChatFileRouter(t, tc.fs, home)
			dir := chatfiles.WorkspaceUploadDir(home, uploadChatFileTestChatID)
			require.NoError(t, os.MkdirAll(dir, 0o755))

			staleTemp := filepath.Join(dir, ".old.zip.upload-0123456789abcdef")
			activeTemp := filepath.Join(dir, ".new.zip.upload-fedcba9876543210")
			oldUpload := filepath.Join(dir, "old.zip")
			old := time.Now().Add(-2 * time.Hour)
			for _, p := range []string{staleTemp, activeTemp, oldUpload} {
				require.NoError(t, os.WriteFile(p, []byte("x"), 0o600))
			}
			require.NoError(t, os.Chtimes(staleTemp, old, old))
			require.NoError(t, os.Chtimes(oldUpload, old, old))

			decodeUploadChatFileResponse(t, uploadChatFile(t, r, "archive.zip", strings.NewReader("payload")))

			_, err := os.Lstat(staleTemp)
			require.ErrorIs(t, err, os.ErrNotExist)
			require.FileExists(t, activeTemp)
			require.FileExists(t, oldUpload)
		})
	}
}

func TestHandleUploadChatFile_LogsFailure(t *testing.T) {
	t.Parallel()

	sink := testutil.NewFakeSink(t)
	api := agentfiles.NewAPI(sink.Logger(), afero.NewOsFs(), nil, agentfiles.WithEnvInfo(fakeBundleEnvInfo{home: t.TempDir()}))

	body := io.MultiReader(strings.NewReader("partial"), iotest.ErrReader(os.ErrClosed))
	req := httptest.NewRequest(http.MethodPost, uploadChatFileURL(uploadChatFileTestChatID, "my archive.zip"), body)
	rr := httptest.NewRecorder()
	api.HandleUploadChatFile(rr, req)
	require.Equal(t, http.StatusInternalServerError, rr.Code, rr.Body.String())

	entries := sink.Entries(func(e slog.SinkEntry) bool { return e.Level == slog.LevelWarn })
	require.Len(t, entries, 1)
	fields := map[string]any{}
	for _, f := range entries[0].Fields {
		fields[f.Name] = f.Value
	}
	require.Equal(t, uploadChatFileTestChatID, fmt.Sprint(fields["chat_id"]))
	require.Equal(t, "my_archive.zip", fields["name"])
	require.EqualValues(t, len("partial"), fields["bytes_received"])
	require.Equal(t, http.StatusInternalServerError, fields["status"])
	require.Contains(t, fields, "duration")
	loggedErr, ok := fields["error"].(error)
	require.True(t, ok)
	require.ErrorIs(t, loggedErr, os.ErrClosed)
}

func TestHandleUploadChatFile_BadRequest(t *testing.T) {
	t.Parallel()

	r := newUploadChatFileRouter(t, afero.NewOsFs(), t.TempDir())

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

func TestHandleUploadChatFile_UsesConfiguredHomeDir(t *testing.T) {
	t.Parallel()

	// A fresh chat ID makes the check against the real process home
	// meaningful: nothing else could have created this directory.
	chatID := uuid.NewString()
	processHome, err := usershell.SystemEnvInfo{}.HomeDir()
	require.NoError(t, err)
	agentHome := t.TempDir()
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	api := agentfiles.NewAPI(logger, afero.NewOsFs(), nil, agentfiles.WithEnvInfo(fakeBundleEnvInfo{home: agentHome}))

	req := httptest.NewRequest(http.MethodPost, uploadChatFileURL(chatID, "archive.zip"), strings.NewReader("zip bytes"))
	rr := httptest.NewRecorder()
	api.HandleUploadChatFile(rr, req)
	resp := decodeUploadChatFileResponse(t, rr)

	require.Equal(t, filepath.Join(chatfiles.WorkspaceUploadDir(agentHome, chatID), "archive.zip"), resp.Path)
	contents, err := os.ReadFile(resp.Path)
	require.NoError(t, err)
	require.Equal(t, "zip bytes", string(contents))
	_, err = os.Stat(chatfiles.WorkspaceChatDir(processHome, chatID))
	require.ErrorIs(t, err, os.ErrNotExist)
}

// deadlineRecorder exposes the per-request deadline hooks that
// http.ResponseController looks for, so the test can observe how the
// upload handler pushes the agent server's fixed timeouts forward.
type deadlineRecorder struct {
	*httptest.ResponseRecorder
	readDeadlines  []time.Time
	writeDeadlines []time.Time
}

func (d *deadlineRecorder) SetReadDeadline(deadline time.Time) error {
	d.readDeadlines = append(d.readDeadlines, deadline)
	return nil
}

func (d *deadlineRecorder) SetWriteDeadline(deadline time.Time) error {
	d.writeDeadlines = append(d.writeDeadlines, deadline)
	return nil
}

func TestHandleUploadChatFile_ExtendsDeadlinesWhileStreaming(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	api := agentfiles.NewAPI(logger, afero.NewOsFs(), nil, agentfiles.WithEnvInfo(fakeBundleEnvInfo{home: home}))

	const payload = "abc"
	// OneByteReader delivers the body as three separate chunks, so a
	// stalled-but-progressing upload is modeled by three reads.
	req := httptest.NewRequest(http.MethodPost, uploadChatFileURL(uploadChatFileTestChatID, "slow.bin"), iotest.OneByteReader(strings.NewReader(payload)))
	rr := &deadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
	start := time.Now()
	api.HandleUploadChatFile(rr, req)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())

	for _, deadlines := range [][]time.Time{rr.readDeadlines, rr.writeDeadlines} {
		// At least one extension before the copy starts plus one per chunk.
		require.GreaterOrEqual(t, len(deadlines), 1+len(payload))
		for i, deadline := range deadlines {
			require.False(t, deadline.Before(start.Add(10*time.Second)), "deadline %s was not pushed into the future", deadline)
			if i > 0 {
				require.False(t, deadline.Before(deadlines[i-1]), "deadline moved backwards")
			}
		}
	}
}
