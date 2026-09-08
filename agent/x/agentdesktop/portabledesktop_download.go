package agentdesktop

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
)

// pinnedReleaseVersion is the portabledesktop release the agent downloads
// when no binary is installed in the workspace. Bump it together with the
// checksums below, taken from the release's SHA256SUMS.txt.
const pinnedReleaseVersion = "v0.0.8"

// pinnedReleaseSHA256 maps GOARCH to the sha256 of the release binary.
var pinnedReleaseSHA256 = map[string]string{
	"amd64": "abd71b1a44dcfb896b9aca5759735a7fb385fd9286ea77b82e96683d2be35c68",
	"arm64": "749494fc13658a10da2201ed3ea58a858d78fe3fb445132f72dfd797d0d2191e",
}

// defaultReleaseBaseURL is where release assets are downloaded from. The
// asset name follows portabledesktop's release layout.
const defaultReleaseBaseURL = "https://github.com/coder/portabledesktop/releases/download"

// releaseAssetName returns the release asset for goarch, or an empty string
// when no release exists for it.
func releaseAssetName(goos, goarch string) string {
	if goos != "linux" {
		return ""
	}
	switch goarch {
	case "amd64":
		return "portabledesktop-linux-x64"
	case "arm64":
		return "portabledesktop-linux-arm64"
	default:
		return ""
	}
}

// pinnedBinaryDir returns the directory the pinned release is cached in:
// $XDG_CACHE_HOME/coder/portabledesktop/<version>, falling back to
// ~/.cache/coder/portabledesktop/<version>.
func pinnedBinaryDir() (string, error) {
	base := os.Getenv("XDG_CACHE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", xerrors.Errorf("resolve home directory: %w", err)
		}
		base = filepath.Join(home, ".cache")
	}
	return filepath.Join(base, "coder", "portabledesktop", pinnedReleaseVersion), nil
}

// downloadPinnedBinary fetches the pinned portabledesktop release for this
// platform into the cache directory, verifies its checksum, and returns the
// path. An existing cached copy with a matching checksum is reused.
func (p *portableDesktop) downloadPinnedBinary(ctx context.Context) (string, error) {
	asset := releaseAssetName(runtime.GOOS, runtime.GOARCH)
	want, ok := pinnedReleaseSHA256[runtime.GOARCH]
	if asset == "" || !ok {
		return "", xerrors.Errorf("no portabledesktop release for %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	dir := p.pinnedBinaryDir
	if dir == "" {
		var err error
		dir, err = pinnedBinaryDir()
		if err != nil {
			return "", err
		}
	}
	dest := filepath.Join(dir, "portabledesktop")
	if sum, err := fileSHA256(dest); err == nil && sum == want {
		return dest, nil
	}

	baseURL := p.releaseBaseURL
	if baseURL == "" {
		baseURL = defaultReleaseBaseURL
	}
	url := fmt.Sprintf("%s/%s/%s", baseURL, pinnedReleaseVersion, asset)
	p.logger.Info(ctx, "downloading pinned portabledesktop release",
		slog.F("version", pinnedReleaseVersion),
		slog.F("url", url),
		slog.F("dest", dest),
	)

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", xerrors.Errorf("create cache dir: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".portabledesktop-*")
	if err != nil {
		return "", xerrors.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", xerrors.Errorf("create request: %w", err)
	}
	client := p.httpClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", xerrors.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", xerrors.Errorf("download %s: unexpected status %s", url, resp.Status)
	}

	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, h), resp.Body); err != nil {
		return "", xerrors.Errorf("write %s: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		return "", xerrors.Errorf("close %s: %w", tmpName, err)
	}
	got := hex.EncodeToString(h.Sum(nil))
	if got != want {
		return "", xerrors.Errorf("checksum mismatch for %s: expected %s, got %s", asset, want, got)
	}
	if err := os.Chmod(tmpName, 0o755); err != nil {
		return "", xerrors.Errorf("chmod %s: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, dest); err != nil {
		return "", xerrors.Errorf("move binary into place: %w", err)
	}
	return dest, nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
