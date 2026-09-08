package agentdesktop

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agentexec"
)

// withPinnedSHA temporarily replaces the pinned checksum for this
// platform so tests can serve arbitrary bytes.
func withPinnedSHA(t *testing.T, body []byte) {
	t.Helper()
	old := pinnedReleaseSHA256[runtime.GOARCH]
	sum := sha256.Sum256(body)
	pinnedReleaseSHA256[runtime.GOARCH] = hex.EncodeToString(sum[:])
	t.Cleanup(func() { pinnedReleaseSHA256[runtime.GOARCH] = old })
}

func TestDownloadPinnedBinary(t *testing.T) {
	// Not parallel: withPinnedSHA mutates package state.
	if releaseAssetName(runtime.GOOS, runtime.GOARCH) == "" {
		t.Skip("no portabledesktop release for this platform")
	}

	body := []byte("#!/bin/sh\necho portabledesktop\n")
	withPinnedSHA(t, body)

	var hits atomic.Int32
	wantPath := "/" + pinnedReleaseVersion + "/" + releaseAssetName(runtime.GOOS, runtime.GOARCH)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path != wantPath {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	cacheDir := t.TempDir()
	pd := &portableDesktop{
		logger:          slogtest.Make(t, nil),
		execer:          agentexec.DefaultExecer,
		scriptBinDir:    t.TempDir(),
		releaseBaseURL:  srv.URL,
		pinnedBinaryDir: cacheDir,
	}

	// ensureBinary falls through PATH and the script dir to the download.
	t.Setenv("PATH", "")
	require.NoError(t, pd.ensureBinary(t.Context()))
	assert.Equal(t, filepath.Join(cacheDir, "portabledesktop"), pd.binPath)

	info, err := os.Stat(pd.binPath)
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&0o111, "downloaded binary must be executable")
	got, err := os.ReadFile(pd.binPath)
	require.NoError(t, err)
	assert.Equal(t, body, got)
	assert.EqualValues(t, 1, hits.Load())

	// A second resolution reuses the cached copy without a request.
	pd2 := &portableDesktop{
		logger:          slogtest.Make(t, nil),
		execer:          agentexec.DefaultExecer,
		scriptBinDir:    t.TempDir(),
		releaseBaseURL:  srv.URL,
		pinnedBinaryDir: cacheDir,
	}
	require.NoError(t, pd2.ensureBinary(t.Context()))
	assert.Equal(t, pd.binPath, pd2.binPath)
	assert.EqualValues(t, 1, hits.Load())

	// A cached file that does not match the checksum is replaced.
	require.NoError(t, os.WriteFile(pd.binPath, []byte("corrupt"), 0o600))
	pd3 := &portableDesktop{
		logger:          slogtest.Make(t, nil),
		execer:          agentexec.DefaultExecer,
		scriptBinDir:    t.TempDir(),
		releaseBaseURL:  srv.URL,
		pinnedBinaryDir: cacheDir,
	}
	require.NoError(t, pd3.ensureBinary(t.Context()))
	got, err = os.ReadFile(pd3.binPath)
	require.NoError(t, err)
	assert.Equal(t, body, got)
	assert.EqualValues(t, 2, hits.Load())
}

func TestDownloadPinnedBinary_ChecksumMismatch(t *testing.T) {
	// Not parallel: withPinnedSHA mutates package state.
	if releaseAssetName(runtime.GOOS, runtime.GOARCH) == "" {
		t.Skip("no portabledesktop release for this platform")
	}

	withPinnedSHA(t, []byte("expected"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("something else"))
	}))
	t.Cleanup(srv.Close)

	cacheDir := t.TempDir()
	pd := &portableDesktop{
		logger:          slogtest.Make(t, nil),
		execer:          agentexec.DefaultExecer,
		scriptBinDir:    t.TempDir(),
		releaseBaseURL:  srv.URL,
		pinnedBinaryDir: cacheDir,
	}
	t.Setenv("PATH", "")
	err := pd.ensureBinary(t.Context())
	require.ErrorContains(t, err, "checksum mismatch")
	assert.Empty(t, pd.binPath)

	// Nothing is left behind in the cache directory.
	entries, err := os.ReadDir(cacheDir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestReleaseAssetName(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "portabledesktop-linux-x64", releaseAssetName("linux", "amd64"))
	assert.Equal(t, "portabledesktop-linux-arm64", releaseAssetName("linux", "arm64"))
	assert.Empty(t, releaseAssetName("linux", "386"))
	assert.Empty(t, releaseAssetName("darwin", "arm64"))
	assert.Empty(t, releaseAssetName("windows", "amd64"))
	for arch := range pinnedReleaseSHA256 {
		assert.NotEmpty(t, releaseAssetName("linux", arch), "checksum pinned for %s without an asset", arch)
	}
}
