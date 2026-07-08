package agentfiles_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agentfiles"
	"github.com/coder/coder/v2/agent/usershell"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
)

// zipFilesTestMaxPatterns matches the agent's requested path pattern limit.
const zipFilesTestMaxPatterns = 32

func TestHandleZipFiles(t *testing.T) {
	t.Parallel()

	t.Run("CollectsExpandedPathsAndGlobs", func(t *testing.T) {
		t.Parallel()

		home := t.TempDir()
		writeZipTestFile(t, home, ".vscode-server/data/logs/20260706T101112/remoteagent.log", "remote agent")
		writeZipTestFile(t, home, ".vscode-server/data/logs/20260706T101112/exthost1/exthost.log", "exthost")
		writeZipTestFile(t, home, ".vscode-server/data/logs/20260706T101112/exthost1/output.txt", "skip")
		writeZipTestFile(t, home, ".local/share/code-server/coder-logs/app.log", "code server log")
		writeZipTestFile(t, home, ".cache/JetBrains/RemoteDev/dist/241.15989.150/log/idea.log", "idea log")
		writeZipTestFile(t, home, "brace/one.log", "one")
		writeZipTestFile(t, home, "brace/two.txt", "two")
		writeZipTestFile(t, home, "brace/skip.json", "skip")

		entries := readZipFilesArchive(t, requestZipFiles(t, newZipFilesHandler(t, home), []string{
			filepath.Join(home, ".vscode-server/data/logs/20260706T101112/remoteagent.log"),
			"$HOME/.vscode-server/data/logs/**/*.log",
			"$HOME/.local/share/code-server/coder-logs/app.log",
			"~/.cache/JetBrains/RemoteDev/dist/*/log/idea.log",
			"${HOME}/brace/*.{log,txt}",
		}))

		requireZipEntry(t, entries, home, ".vscode-server/data/logs/20260706T101112/remoteagent.log", "remote agent")
		requireZipEntry(t, entries, home, ".vscode-server/data/logs/20260706T101112/exthost1/exthost.log", "exthost")
		requireZipEntry(t, entries, home, ".local/share/code-server/coder-logs/app.log", "code server log")
		requireZipEntry(t, entries, home, ".cache/JetBrains/RemoteDev/dist/241.15989.150/log/idea.log", "idea log")
		requireZipEntry(t, entries, home, "brace/one.log", "one")
		requireZipEntry(t, entries, home, "brace/two.txt", "two")
		require.NotContains(t, entries.files, zipArchivePath(t, home, ".vscode-server/data/logs/20260706T101112/exthost1/output.txt"))
		require.NotContains(t, entries.files, zipArchivePath(t, home, "brace/skip.json"))
		require.Empty(t, entries.manifest.Errors)
		// The absolute path and the ** glob both match remoteagent.log; it
		// must be archived once.
		require.Len(t, entries.manifest.Files, 6)
	})

	t.Run("CollectsAbsolutePathsOutsideHome", func(t *testing.T) {
		t.Parallel()

		home := t.TempDir()
		outside := t.TempDir()
		writeZipTestFile(t, outside, "service.log", "outside log")
		writeZipTestFile(t, outside, "glob/a.log", "glob a")

		entries := readZipFilesArchive(t, requestZipFiles(t, newZipFilesHandler(t, home), []string{
			filepath.Join(outside, "service.log"),
			filepath.Join(outside, "glob", "*.log"),
		}))

		requireZipEntry(t, entries, outside, "service.log", "outside log")
		requireZipEntry(t, entries, outside, "glob/a.log", "glob a")
		require.Empty(t, entries.manifest.Errors)
		require.Len(t, entries.manifest.Files, 2)
	})

	t.Run("PatternLimit", func(t *testing.T) {
		t.Parallel()

		home := t.TempDir()
		writeZipTestFile(t, home, "kept.log", "kept")

		paths := []string{"$HOME/kept.log"}
		for len(paths) < zipFilesTestMaxPatterns {
			paths = append(paths, "$HOME/missing.log")
		}
		paths = append(paths, "$HOME/skipped-one.log", "$HOME/skipped-two.log")

		entries := readZipFilesArchive(t, requestZipFiles(t, newZipFilesHandler(t, home), paths))

		requireZipEntry(t, entries, home, "kept.log", "kept")
		require.Equal(t, paths, entries.manifest.Requested)
		require.True(t, entries.manifest.Truncated)
		// Patterns beyond the limit are named in the manifest, not silently
		// dropped. The repeated missing.log is reported once thanks to dedupe.
		require.Equal(t, []workspacesdk.ZipFilesManifestError{
			{Requested: "$HOME/skipped-one.log", Reason: "requested path pattern limit reached"},
			{Requested: "$HOME/skipped-two.log", Reason: "requested path pattern limit reached"},
			{Requested: "$HOME/missing.log", Path: filepath.Join(home, "missing.log"), Reason: "does not exist"},
		}, entries.manifest.Errors)
	})

	t.Run("RejectedPathsAreNonFatal", func(t *testing.T) {
		t.Parallel()

		home := t.TempDir()
		writeZipTestFile(t, home, "kept.log", "kept")
		require.NoError(t, os.MkdirAll(filepath.Join(home, "somedir"), 0o700))

		entries := readZipFilesArchive(t, requestZipFiles(t, newZipFilesHandler(t, home), []string{
			"$HOME/kept.log",
			"relative.log",
			"$HOME/missing.log",
			"$HOME/somedir",
			"$HOME/no-matches/**/*.log",
		}))

		requireZipEntry(t, entries, home, "kept.log", "kept")
		require.Len(t, entries.manifest.Files, 1)
		requireZipFilesManifestErrors(t, entries.manifest.Errors,
			"relative path",
			"does not exist",
			"not a regular file: directory",
			"no matches",
		)
	})

	t.Run("TailBytesTruncation", func(t *testing.T) {
		t.Parallel()

		home := t.TempDir()
		writeZipTestFile(t, home, "large.log", "0123456789")

		entries := readZipFilesArchive(t, requestZipFiles(t, newZipFilesHandler(t, home, agentfiles.WithZipFilesLimits(workspacesdk.ZipFilesLimits{
			MaxFiles:        10,
			MaxBytesPerFile: 4,
			MaxTotalBytes:   100,
		})), []string{"$HOME/large.log"}))

		requireZipEntry(t, entries, home, "large.log", "6789")
		require.Len(t, entries.manifest.Files, 1)
		require.True(t, entries.manifest.Files[0].Truncated)
		require.Equal(t, int64(10), entries.manifest.Files[0].Size)
		require.Equal(t, int64(4), entries.manifest.Files[0].BytesWritten)
	})

	t.Run("FileAndByteLimits", func(t *testing.T) {
		t.Parallel()

		home := t.TempDir()
		writeZipTestFile(t, home, "one.log", "1111")
		writeZipTestFile(t, home, "two.log", "2222")
		writeZipTestFile(t, home, "three.log", "3333")

		entries := readZipFilesArchive(t, requestZipFiles(t, newZipFilesHandler(t, home, agentfiles.WithZipFilesLimits(workspacesdk.ZipFilesLimits{
			MaxFiles:        1,
			MaxBytesPerFile: 100,
			MaxTotalBytes:   3,
		})), []string{"$HOME/*.log"}))

		require.Len(t, entries.files, 1)
		require.True(t, entries.manifest.Truncated)
		require.Equal(t, int64(3), entries.manifest.Files[0].BytesWritten)
		// The glob walk itself stops at the file cap.
		requireZipFilesManifestErrors(t, entries.manifest.Errors, "file count limit reached")
	})

	t.Run("DedupeByCleanedPath", func(t *testing.T) {
		t.Parallel()

		home := t.TempDir()
		writeZipTestFile(t, home, "dup.log", "one")
		writeZipTestFile(t, home, "other.log", "two")

		entries := readZipFilesArchive(t, requestZipFiles(t, newZipFilesHandler(t, home), []string{
			"$HOME/dup.log",
			"$HOME/./dup.log",
			filepath.Join(home, "somedir", "..", "dup.log"),
			"$HOME/other.log",
		}))

		requireZipEntry(t, entries, home, "dup.log", "one")
		requireZipEntry(t, entries, home, "other.log", "two")
		require.Len(t, entries.manifest.Files, 2)
	})
}

// fakeZipEnvInfo overrides the home directory so tests can point path
// expansion at a temp dir.
type fakeZipEnvInfo struct {
	usershell.SystemEnvInfo
	home string
}

func (e fakeZipEnvInfo) HomeDir() (string, error) {
	return e.home, nil
}

func newZipFilesHandler(t *testing.T, home string, opts ...agentfiles.Option) http.Handler {
	t.Helper()

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	return agentfiles.NewAPI(logger, afero.NewOsFs(), nil, fakeZipEnvInfo{home: home}, opts...).Routes()
}

func requestZipFiles(t *testing.T, handler http.Handler, paths []string) []byte {
	t.Helper()

	body, err := json.Marshal(workspacesdk.ZipFilesRequest{Paths: paths})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/zip-files", bytes.NewReader(body))
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)
	require.Equal(t, "application/zip", res.Header().Get("Content-Type"))
	return res.Body.Bytes()
}

func writeZipTestFile(t *testing.T, dir string, rel string, content string) {
	t.Helper()

	path := filepath.Join(dir, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

// zipArchivePath returns the expected archive entry name for the file at
// dir/rel, mirroring the agent's mapping of an absolute path to files/...
// including the Windows drive-colon drop.
func zipArchivePath(t *testing.T, dir string, rel string) string {
	t.Helper()

	abs := filepath.Join(dir, filepath.FromSlash(rel))
	p := strings.TrimPrefix(filepath.ToSlash(abs), "/")
	if len(p) >= 2 && p[1] == ':' {
		p = p[:1] + p[2:]
	}
	return "files/" + p
}

func requireZipEntry(t *testing.T, entries zipFilesArchive, dir string, rel string, content string) {
	t.Helper()

	require.Equal(t, content, string(entries.files[zipArchivePath(t, dir, rel)]))
}

type zipFilesArchive struct {
	manifest workspacesdk.ZipFilesManifest
	files    map[string][]byte
}

func readZipFilesArchive(t *testing.T, data []byte) zipFilesArchive {
	t.Helper()

	entries := zipFilesArchive{files: testutil.ReadZip(t, data)}
	manifestJSON, ok := entries.files["manifest.json"]
	require.True(t, ok, "archive should contain manifest.json")
	delete(entries.files, "manifest.json")
	require.NoError(t, json.Unmarshal(manifestJSON, &entries.manifest))
	require.NotEmpty(t, entries.manifest.Requested)
	return entries
}

func requireZipFilesManifestErrors(t *testing.T, errs []workspacesdk.ZipFilesManifestError, contains ...string) {
	t.Helper()

	for _, want := range contains {
		found := slices.ContainsFunc(errs, func(e workspacesdk.ZipFilesManifestError) bool {
			return strings.Contains(e.Reason, want)
		})
		require.Truef(t, found, "expected manifest error containing %q in %#v", want, errs)
	}
}
