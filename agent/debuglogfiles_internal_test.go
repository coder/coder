package agent

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/usershell"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
)

func TestCollectDebugLogFiles(t *testing.T) {
	t.Parallel()

	t.Run("CollectsExpandedPathsAndGlobs", func(t *testing.T) {
		t.Parallel()

		home := t.TempDir()
		writeTestLogFile(t, home, ".vscode-server/data/logs/20260706T101112/remoteagent.log", "remote agent")
		writeTestLogFile(t, home, ".vscode-server/data/logs/20260706T101112/exthost1/exthost.log", "exthost")
		writeTestLogFile(t, home, ".vscode-server/data/logs/20260706T101112/exthost1/output.txt", "skip")
		writeTestLogFile(t, home, ".local/share/code-server/coder-logs/app.log", "code server log")
		writeTestLogFile(t, home, ".cache/JetBrains/RemoteDev/dist/241.15989.150/log/idea.log", "idea log")
		writeTestLogFile(t, home, "brace/one.log", "one")
		writeTestLogFile(t, home, "brace/two.txt", "two")
		writeTestLogFile(t, home, "brace/skip.json", "skip")

		entries := readDebugLogFilesArchive(t, collectDebugLogFilesForTest(t, home, []string{
			filepath.Join(home, ".vscode-server/data/logs/20260706T101112/remoteagent.log"),
			"$HOME/.vscode-server/data/logs/**/*.log",
			"$HOME/.local/share/code-server/coder-logs/app.log",
			"~/.cache/JetBrains/RemoteDev/dist/*/log/idea.log",
			"$HOME/brace/*.{log,txt}",
		}))

		require.Equal(t, "remote agent", string(entries.files["files/.vscode-server/data/logs/20260706T101112/remoteagent.log"]))
		require.Equal(t, "exthost", string(entries.files["files/.vscode-server/data/logs/20260706T101112/exthost1/exthost.log"]))
		require.Equal(t, "code server log", string(entries.files["files/.local/share/code-server/coder-logs/app.log"]))
		require.Equal(t, "idea log", string(entries.files["files/.cache/JetBrains/RemoteDev/dist/241.15989.150/log/idea.log"]))
		require.Equal(t, "one", string(entries.files["files/brace/one.log"]))
		require.Equal(t, "two", string(entries.files["files/brace/two.txt"]))
		require.NotContains(t, entries.files, "files/.vscode-server/data/logs/20260706T101112/exthost1/output.txt")
		require.NotContains(t, entries.files, "files/brace/skip.json")
		require.Empty(t, entries.manifest.Errors)
		// The absolute path and the ** glob both match remoteagent.log; it
		// must be archived once.
		require.Len(t, entries.manifest.Files, 6)
	})

	t.Run("PatternLimit", func(t *testing.T) {
		t.Parallel()

		home := t.TempDir()
		writeTestLogFile(t, home, "kept.log", "kept")

		paths := []string{"$HOME/kept.log"}
		for len(paths) < debugLogFilesMaxPatterns {
			paths = append(paths, "$HOME/missing.log")
		}
		paths = append(paths, "$HOME/skipped-one.log", "$HOME/skipped-two.log")

		entries := readDebugLogFilesArchive(t, collectDebugLogFilesForTest(t, home, paths))

		require.Equal(t, "kept", string(entries.files["files/kept.log"]))
		require.Equal(t, paths, entries.manifest.Requested)
		require.True(t, entries.manifest.Truncated)
		// Patterns beyond the limit are named in the manifest, not silently
		// dropped. The repeated missing.log is reported once thanks to dedupe.
		require.Equal(t, []workspacesdk.DebugLogFilesManifestError{
			{Requested: "$HOME/skipped-one.log", Reason: "requested path pattern limit reached"},
			{Requested: "$HOME/skipped-two.log", Reason: "requested path pattern limit reached"},
			{Requested: "$HOME/missing.log", Path: "missing.log", Reason: "does not exist"},
		}, entries.manifest.Errors)
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
			t.Skip("creating symlinks requires elevated privileges on windows")
		}

		home := t.TempDir()
		outside := t.TempDir()
		writeTestLogFile(t, outside, "secret.log", "secret")
		require.NoError(t, os.Symlink(filepath.Join(outside, "secret.log"), filepath.Join(home, "link.log")))

		entries := readDebugLogFilesArchive(t, collectDebugLogFilesForTest(t, home, []string{"$HOME/link.log"}))

		// os.Root refuses to follow the symlink out of the home directory.
		require.Empty(t, entries.files)
		requireDebugLogFilesManifestErrors(t, entries.manifest.Errors, "escapes")
	})

	t.Run("TailBytesTruncation", func(t *testing.T) {
		t.Parallel()

		home := t.TempDir()
		writeTestLogFile(t, home, "large.log", "0123456789")

		limits := defaultDebugLogFilesLimits
		limits.MaxBytesPerFile = 4
		entries := readDebugLogFilesArchive(t, collectDebugLogFilesForTest(t, home, []string{"$HOME/large.log"}, limits))

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

		limits := defaultDebugLogFilesLimits
		limits.MaxFiles = 1
		limits.MaxGlobMatches = 2
		limits.MaxTotalBytes = 3
		entries := readDebugLogFilesArchive(t, collectDebugLogFilesForTest(t, home, []string{"$HOME/*.log"}, limits))

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
	// SystemEnvInfo.HomeDir reads HOME on Unix and USERPROFILE on Windows.
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	writeTestLogFile(t, home, "server.log", "server log")
	a := &agent{
		logger:  slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug),
		envInfo: &usershell.SystemEnvInfo{},
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

func collectDebugLogFilesForTest(t *testing.T, home string, paths []string, limit ...workspacesdk.DebugLogFilesLimits) []byte {
	t.Helper()

	limits := defaultDebugLogFilesLimits
	if len(limit) > 0 {
		limits = limit[0]
	}

	var buf bytes.Buffer
	err := collectDebugLogFiles(t.Context(), home, workspacesdk.DebugLogFilesRequest{Paths: paths}, &buf, limits)
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
	manifest workspacesdk.DebugLogFilesManifest
	files    map[string][]byte
}

func readDebugLogFilesArchive(t *testing.T, data []byte) debugLogFilesArchive {
	t.Helper()

	entries := debugLogFilesArchive{files: testutil.ReadZip(t, data)}
	manifestJSON, ok := entries.files["manifest.json"]
	require.True(t, ok, "archive should contain manifest.json")
	delete(entries.files, "manifest.json")
	require.NoError(t, json.Unmarshal(manifestJSON, &entries.manifest))
	require.NotEmpty(t, entries.manifest.Requested)
	return entries
}

func requireDebugLogFilesManifestErrors(t *testing.T, errs []workspacesdk.DebugLogFilesManifestError, contains ...string) {
	t.Helper()

	for _, want := range contains {
		found := slices.ContainsFunc(errs, func(e workspacesdk.DebugLogFilesManifestError) bool {
			return strings.Contains(e.Reason, want)
		})
		require.Truef(t, found, "expected manifest error containing %q in %#v", want, errs)
	}
}
