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

func TestBundleFilesCollectsExpandedPathsAndGlobs(t *testing.T) {
	t.Parallel()

	home := testutil.TempDirResolved(t)
	writeBundleSourceFile(t, home, ".vscode-server/data/logs/20260706T101112/remoteagent.log", "remote agent")
	writeBundleSourceFile(t, home, ".vscode-server/data/logs/20260706T101112/exthost1/exthost.log", "exthost")
	writeBundleSourceFile(t, home, ".vscode-server/data/logs/20260706T101112/exthost1/output.txt", "skip")
	writeBundleSourceFile(t, home, ".local/share/code-server/coder-logs/app.log", "code server log")
	writeBundleSourceFile(t, home, ".cache/JetBrains/RemoteDev/dist/241.15989.150/log/idea.log", "idea log")
	writeBundleSourceFile(t, home, "brace/one.log", "one")
	writeBundleSourceFile(t, home, "brace/two.txt", "two")
	writeBundleSourceFile(t, home, "brace/skip.json", "skip")

	entries := readBundleFilesArchive(t, requestBundleFiles(t, newBundleFilesHandler(t, home), []string{
		filepath.Join(home, ".vscode-server/data/logs/20260706T101112/remoteagent.log"),
		"~/.vscode-server/data/logs/**/*.log",
		"~/.local/share/code-server/coder-logs/app.log",
		"~/.cache/JetBrains/RemoteDev/dist/*/log/idea.log",
		"~/brace/*.{log,txt}",
	}))

	requireBundleEntry(t, entries, home, ".vscode-server/data/logs/20260706T101112/remoteagent.log", "remote agent")
	requireBundleEntry(t, entries, home, ".vscode-server/data/logs/20260706T101112/exthost1/exthost.log", "exthost")
	requireBundleEntry(t, entries, home, ".local/share/code-server/coder-logs/app.log", "code server log")
	requireBundleEntry(t, entries, home, ".cache/JetBrains/RemoteDev/dist/241.15989.150/log/idea.log", "idea log")
	requireBundleEntry(t, entries, home, "brace/one.log", "one")
	requireBundleEntry(t, entries, home, "brace/two.txt", "two")
	require.NotContains(t, entries.files, bundleArchivePath(t, home, ".vscode-server/data/logs/20260706T101112/exthost1/output.txt"))
	require.NotContains(t, entries.files, bundleArchivePath(t, home, "brace/skip.json"))
	require.Empty(t, entries.manifest.Errors)
	// remoteagent.log matches both the absolute path and the ** glob; it
	// must be archived once.
	require.Len(t, entries.manifest.Files, 6)
}

func TestBundleFilesCollectsAbsolutePathsOutsideHome(t *testing.T) {
	t.Parallel()

	home := testutil.TempDirResolved(t)
	outside := testutil.TempDirResolved(t)
	writeBundleSourceFile(t, outside, "service.log", "outside log")
	writeBundleSourceFile(t, outside, "glob/a.log", "glob a")

	entries := readBundleFilesArchive(t, requestBundleFiles(t, newBundleFilesHandler(t, home), []string{
		filepath.Join(outside, "service.log"),
		filepath.Join(outside, "glob", "*.log"),
	}))

	requireBundleEntry(t, entries, outside, "service.log", "outside log")
	requireBundleEntry(t, entries, outside, "glob/a.log", "glob a")
	require.Empty(t, entries.manifest.Errors)
	require.Len(t, entries.manifest.Files, 2)
}

func TestBundleFilesRejectedPathsAreNonFatal(t *testing.T) {
	t.Parallel()

	home := testutil.TempDirResolved(t)
	writeBundleSourceFile(t, home, "kept.log", "kept")
	require.NoError(t, os.MkdirAll(filepath.Join(home, "somedir"), 0o700))

	entries := readBundleFilesArchive(t, requestBundleFiles(t, newBundleFilesHandler(t, home), []string{
		"~/kept.log",
		"relative.log",
		"~/missing.log",
		"~/somedir",
		"~/no-matches/**/*.log",
	}))

	requireBundleEntry(t, entries, home, "kept.log", "kept")
	require.Len(t, entries.manifest.Files, 1)
	requireBundleFilesManifestErrors(t, entries.manifest.Errors,
		"is not absolute",
		"does not exist",
		"not a regular file: directory",
		"no matches",
	)
}

func TestBundleFilesTailBytesTruncation(t *testing.T) {
	t.Parallel()

	home := testutil.TempDirResolved(t)
	writeBundleSourceFile(t, home, "large.log", "0123456789")

	entries := readBundleFilesArchive(t, requestBundleFiles(t, newBundleFilesHandler(t, home, agentfiles.WithBundleFilesLimits(workspacesdk.BundleFilesLimits{
		MaxFiles:        10,
		MaxBytesPerFile: 4,
		MaxTotalBytes:   100 * 1024,
	})), []string{"~/large.log"}))

	requireBundleEntry(t, entries, home, "large.log", "6789")
	require.Len(t, entries.manifest.Files, 1)
	require.True(t, entries.manifest.Files[0].Truncated)
	require.Equal(t, int64(10), entries.manifest.Files[0].Size)
	require.Equal(t, int64(4), entries.manifest.Files[0].BytesWritten)
}

func TestBundleFilesFileAndByteLimits(t *testing.T) {
	t.Parallel()

	home := testutil.TempDirResolved(t)
	writeBundleSourceFile(t, home, "one.log", "1111")
	writeBundleSourceFile(t, home, "two.log", "2222")
	writeBundleSourceFile(t, home, "three.log", "3333")

	entries := readBundleFilesArchive(t, requestBundleFiles(t, newBundleFilesHandler(t, home, agentfiles.WithBundleFilesLimits(workspacesdk.BundleFilesLimits{
		MaxFiles:        1,
		MaxBytesPerFile: 100,
		// One 512-byte tar header plus 3 data bytes: the first file is
		// truncated to 3 bytes by the total budget.
		MaxTotalBytes: 515,
	})), []string{"~/*.log"}))

	require.Len(t, entries.files, 1)
	require.True(t, entries.manifest.Truncated)
	require.Equal(t, int64(3), entries.manifest.Files[0].BytesWritten)
	// The glob walk itself stops at the file cap.
	requireBundleFilesManifestErrors(t, entries.manifest.Errors, "file count limit reached")
}

func TestBundleFilesDedupeByCleanedPath(t *testing.T) {
	t.Parallel()

	home := testutil.TempDirResolved(t)
	writeBundleSourceFile(t, home, "dup.log", "one")
	writeBundleSourceFile(t, home, "other.log", "two")

	entries := readBundleFilesArchive(t, requestBundleFiles(t, newBundleFilesHandler(t, home), []string{
		"~/dup.log",
		"~/./dup.log",
		filepath.Join(home, "somedir", "..", "dup.log"),
		"~/other.log",
	}))

	requireBundleEntry(t, entries, home, "dup.log", "one")
	requireBundleEntry(t, entries, home, "other.log", "two")
	require.Len(t, entries.manifest.Files, 2)
}

// fakeBundleEnvInfo overrides the home directory so tests can point path
// expansion at a temp dir.
type fakeBundleEnvInfo struct {
	usershell.SystemEnvInfo
	home string
}

func (e fakeBundleEnvInfo) HomeDir() (string, error) {
	return e.home, nil
}

func newBundleFilesHandler(t *testing.T, home string, opts ...agentfiles.Option) http.Handler {
	t.Helper()

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	opts = append([]agentfiles.Option{agentfiles.WithEnvInfo(fakeBundleEnvInfo{home: home})}, opts...)
	return agentfiles.NewAPI(logger, afero.NewOsFs(), nil, opts...).Routes()
}

func requestBundleFiles(t *testing.T, handler http.Handler, paths []string) []byte {
	t.Helper()

	body, err := json.Marshal(workspacesdk.BundleFilesRequest{Paths: paths})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/bundle-files", bytes.NewReader(body))
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)
	require.Equal(t, "application/x-tar", res.Header().Get("Content-Type"))
	return res.Body.Bytes()
}

func writeBundleSourceFile(t *testing.T, dir string, rel string, content string) {
	t.Helper()

	path := filepath.Join(dir, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

// bundleArchivePath returns the expected archive entry name for the file
// at dir/rel.
func bundleArchivePath(t *testing.T, dir string, rel string) string {
	t.Helper()

	return agentfiles.BundleFilesArchivePath(filepath.Join(dir, filepath.FromSlash(rel)))
}

func requireBundleEntry(t *testing.T, entries bundleFilesArchive, dir string, rel string, content string) {
	t.Helper()

	require.Equal(t, content, string(entries.files[bundleArchivePath(t, dir, rel)]))
}

type bundleFilesArchive struct {
	manifest workspacesdk.BundleFilesManifest
	files    map[string][]byte
}

func readBundleFilesArchive(t *testing.T, data []byte) bundleFilesArchive {
	t.Helper()

	entries := bundleFilesArchive{files: testutil.ReadTar(t, data)}
	manifestJSON, ok := entries.files["manifest.json"]
	require.True(t, ok, "archive should contain manifest.json")
	delete(entries.files, "manifest.json")
	require.NoError(t, json.Unmarshal(manifestJSON, &entries.manifest))
	require.NotEmpty(t, entries.manifest.Requested)
	return entries
}

func requireBundleFilesManifestErrors(t *testing.T, errs []workspacesdk.BundleFilesManifestError, contains ...string) {
	t.Helper()

	for _, want := range contains {
		found := slices.ContainsFunc(errs, func(e workspacesdk.BundleFilesManifestError) bool {
			return strings.Contains(e.Reason, want)
		})
		require.Truef(t, found, "expected manifest error containing %q in %#v", want, errs)
	}
}
