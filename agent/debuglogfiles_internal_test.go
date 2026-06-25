package agent

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

func TestCollectDebugLogFiles(t *testing.T) {
	t.Parallel()

	t.Run("CollectsExpandedPathsAndGlobs", func(t *testing.T) {
		t.Parallel()

		home := t.TempDir()
		writeTestLogFile(t, home, ".vscode-server/data/logs/server.log", "server log")
		writeTestLogFile(t, home, ".vscode-server/data/logs/2026/06/nested.log", "nested")
		writeTestLogFile(t, home, ".vscode-server/data/logs/2026/06/skip.txt", "skip")
		writeTestLogFile(t, home, ".local/share/code-server/coder-logs/app.log", "code server log")
		writeTestLogFile(t, home, "jetbrains/idea.log", "jetbrains log")
		writeTestLogFile(t, home, "brace/one.log", "one")
		writeTestLogFile(t, home, "brace/two.txt", "two")
		writeTestLogFile(t, home, "brace/skip.json", "skip")

		entries := readDebugLogFilesArchive(t, collectDebugLogFilesForTest(t, home, []string{
			filepath.Join(home, ".vscode-server/data/logs/server.log"),
			"$HOME/.vscode-server/data/logs/**/*.log",
			"$HOME/.local/share/code-server/coder-logs/app.log",
			"~/jetbrains/idea.log",
			"$HOME/brace/*.{log,txt}",
		}))

		require.Equal(t, "server log", string(entries.files["files/.vscode-server/data/logs/server.log"]))
		require.Equal(t, "nested", string(entries.files["files/.vscode-server/data/logs/2026/06/nested.log"]))
		require.Equal(t, "code server log", string(entries.files["files/.local/share/code-server/coder-logs/app.log"]))
		require.Equal(t, "jetbrains log", string(entries.files["files/jetbrains/idea.log"]))
		require.Equal(t, "one", string(entries.files["files/brace/one.log"]))
		require.Equal(t, "two", string(entries.files["files/brace/two.txt"]))
		require.NotContains(t, entries.files, "files/.vscode-server/data/logs/2026/06/skip.txt")
		require.NotContains(t, entries.files, "files/brace/skip.json")
		require.Empty(t, entries.manifest.Errors)
	})

	t.Run("RejectedPathsAreNonFatal", func(t *testing.T) {
		t.Parallel()

		home := t.TempDir()
		outside := t.TempDir()
		writeTestLogFile(t, home, "kept.log", "kept")
		writeTestLogFile(t, outside, "outside.log", "outside")

		entries := readDebugLogFilesArchive(t, collectDebugLogFilesForTest(t, home, []string{
			"$HOME/kept.log",
			"relative.log",
			filepath.Join(outside, "outside.log"),
			"$HOME/missing.log",
			"$HOME/no-matches/**/*.log",
		}))

		require.Equal(t, "kept", string(entries.files["files/kept.log"]))
		require.Len(t, entries.manifest.Files, 1)
		requireDebugLogFilesManifestErrors(t, entries.manifest.Errors,
			"relative path",
			"outside home",
			"does not exist",
			"no matches",
		)
	})

	t.Run("SymlinkEscapeSkipped", func(t *testing.T) {
		t.Parallel()
		if runtime.GOOS == "windows" {
			t.Skip("symlink behavior differs on windows")
		}

		home := t.TempDir()
		outside := t.TempDir()
		writeTestLogFile(t, outside, "secret.log", "secret")
		require.NoError(t, os.Symlink(filepath.Join(outside, "secret.log"), filepath.Join(home, "link.log")))

		entries := readDebugLogFilesArchive(t, collectDebugLogFilesForTest(t, home, []string{"$HOME/link.log"}))

		require.Empty(t, entries.files)
		requireDebugLogFilesManifestErrors(t, entries.manifest.Errors, "outside home")
	})

	t.Run("TailBytesTruncation", func(t *testing.T) {
		t.Parallel()

		home := t.TempDir()
		writeTestLogFile(t, home, "large.log", "0123456789")

		entries := readDebugLogFilesArchive(t, collectDebugLogFilesForTest(t, home, []string{"$HOME/large.log"}, debugLogFilesLimits{
			MaxFiles:        debugLogFilesMaxFiles,
			MaxGlobMatches:  debugLogFilesMaxMatches,
			MaxBytesPerFile: 4,
			MaxTotalBytes:   debugLogFilesMaxTotal,
		}))

		require.Equal(t, "6789", string(entries.files["files/large.log"]))
		require.Len(t, entries.manifest.Files, 1)
		require.True(t, entries.manifest.Files[0].Truncated)
		require.Equal(t, int64(10), entries.manifest.Files[0].Size)
		require.Equal(t, int64(4), entries.manifest.Files[0].BytesWritten)
	})

	t.Run("Limits", func(t *testing.T) {
		t.Parallel()

		home := t.TempDir()
		writeTestLogFile(t, home, "one.log", "1111")
		writeTestLogFile(t, home, "two.log", "2222")
		writeTestLogFile(t, home, "three.log", "3333")

		entries := readDebugLogFilesArchive(t, collectDebugLogFilesForTest(t, home, []string{"$HOME/*.log"}, debugLogFilesLimits{
			MaxFiles:        1,
			MaxGlobMatches:  2,
			MaxBytesPerFile: debugLogFilesMaxBytes,
			MaxTotalBytes:   3,
		}))

		require.Len(t, entries.files, 1)
		require.True(t, entries.manifest.Truncated)
		require.Equal(t, int64(3), entries.manifest.Files[0].BytesWritten)
		requireDebugLogFilesManifestErrors(t, entries.manifest.Errors,
			"glob match limit",
			"file count limit",
		)
	})

	t.Run("ArchivePathCollisions", func(t *testing.T) {
		t.Parallel()

		home := t.TempDir()
		writeTestLogFile(t, home, "dup.log", "one")
		writeTestLogFile(t, home, "dir/../other.log", "two")

		entries := readDebugLogFilesArchive(t, collectDebugLogFilesForTest(t, home, []string{
			"$HOME/dup.log",
			"$HOME/./dup.log",
			"$HOME/other.log",
		}))

		require.Equal(t, "one", string(entries.files["files/dup.log"]))
		require.Equal(t, "two", string(entries.files["files/other.log"]))
		require.Len(t, entries.manifest.Files, 2)
	})
}

func TestHandleHTTPDebugLogFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestLogFile(t, home, "server.log", "server log")
	a := &agent{
		logger: slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug),
	}

	body, err := json.Marshal(workspacesdk.DebugLogFilesRequest{
		Paths: []string{"$HOME/server.log"},
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/debug/log-files", bytes.NewReader(body))
	res := httptest.NewRecorder()

	a.HandleHTTPDebugLogFiles(res, req)

	require.Equal(t, http.StatusOK, res.Code)
	require.Equal(t, "application/zip", res.Header().Get("Content-Type"))
	entries := readDebugLogFilesArchive(t, res.Body.Bytes())
	require.Equal(t, "server log", string(entries.files["files/server.log"]))
}

func collectDebugLogFilesForTest(t *testing.T, home string, paths []string, limit ...debugLogFilesLimits) []byte {
	t.Helper()

	limits := debugLogFilesLimits{
		MaxFiles:        debugLogFilesMaxFiles,
		MaxGlobMatches:  debugLogFilesMaxMatches,
		MaxBytesPerFile: debugLogFilesMaxBytes,
		MaxTotalBytes:   debugLogFilesMaxTotal,
	}
	if len(limit) > 0 {
		limits = limit[0]
	}

	var buf bytes.Buffer
	err := collectDebugLogFilesWithLimits(t.Context(), slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}), home, workspacesdk.DebugLogFilesRequest{Paths: paths}, &buf, limits)
	require.NoError(t, err)
	return buf.Bytes()
}

func writeTestLogFile(t *testing.T, home string, rel string, content string) {
	t.Helper()

	path := filepath.Join(home, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

type debugLogFilesArchive struct {
	manifest debugLogFilesManifest
	files    map[string][]byte
}

func readDebugLogFilesArchive(t *testing.T, data []byte) debugLogFilesArchive {
	t.Helper()

	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	entries := debugLogFilesArchive{files: map[string][]byte{}}
	for _, file := range zr.File {
		rc, err := file.Open()
		require.NoError(t, err)
		content, err := ioReadAllAndClose(rc)
		require.NoError(t, err)
		if file.Name == "manifest.json" {
			require.NoError(t, json.Unmarshal(content, &entries.manifest))
		} else {
			entries.files[file.Name] = content
		}
	}
	require.NotEmpty(t, entries.manifest.Requested)
	return entries
}

func ioReadAllAndClose(rc interface {
	Read([]byte) (int, error)
	Close() error
},
) ([]byte, error) {
	defer rc.Close()
	return io.ReadAll(rc)
}

func requireDebugLogFilesManifestErrors(t *testing.T, errs []debugLogFileError, contains ...string) {
	t.Helper()

	for _, want := range contains {
		found := false
		for _, err := range errs {
			if strings.Contains(err.Reason, want) {
				found = true
				break
			}
		}
		require.Truef(t, found, "expected manifest error containing %q in %#v", want, errs)
	}
}
