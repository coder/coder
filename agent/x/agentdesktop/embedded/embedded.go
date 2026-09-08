// Package embedded carries the portabledesktop release binary compiled into
// linux workspace agent binaries. The agent installs it from these bytes into
// its cache directory, so no download happens inside the workspace.
//
// The binary is only compiled in for linux/amd64 and linux/arm64 builds that
// set the portabledesktop_embed build tag (scripts/build_go.sh does this for
// slim linux binaries once scripts/portabledesktop/fetch.sh has placed the
// release under bin/). Every other build sees no binary and Available reports
// false.
package embedded

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"

	"golang.org/x/xerrors"
)

// Version is the pinned portabledesktop release. Keep in sync with
// scripts/portabledesktop/release.lock.
const Version = "v0.0.8"

// ErrNotAvailable is returned by Install when no binary is compiled in.
var ErrNotAvailable = xerrors.New("portabledesktop is not embedded in this binary")

// binary holds the embedded release for the current GOOS/GOARCH. Tests
// replace it to exercise Install without a real release.
var binary = embeddedBinary

// Available reports whether a portabledesktop binary is compiled in.
func Available() bool {
	return len(binary) > 0
}

// SHA256 returns the hex encoded SHA-256 of the embedded binary, or an empty
// string when Available is false. It hashes on every call; callers only need
// it when installing.
func SHA256() string {
	if len(binary) == 0 {
		return ""
	}
	h := sha256.Sum256(binary)
	return hex.EncodeToString(h[:])
}

// DefaultCacheDir returns $XDG_CACHE_HOME/coder, falling back to
// ~/.cache/coder.
func DefaultCacheDir() (string, error) {
	if dir := os.Getenv("XDG_CACHE_HOME"); dir != "" {
		return filepath.Join(dir, "coder"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", xerrors.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".cache", "coder"), nil
}

// InstallPath returns where Install places the binary below cacheDir. The
// directory name includes the content digest so a new agent binary never
// reuses a stale install.
func InstallPath(cacheDir string) string {
	return filepath.Join(cacheDir, "portabledesktop", Version+"-"+SHA256()[:12], "portabledesktop")
}

// Install writes the embedded binary below cacheDir if a matching copy is not
// already there and returns its path. The file is written to a temporary
// name and renamed into place so concurrent agents never execute a partial
// binary.
func Install(cacheDir string) (string, error) {
	if !Available() {
		return "", ErrNotAvailable
	}
	dest := InstallPath(cacheDir)
	if ok, err := matches(dest); err == nil && ok {
		return dest, nil
	}

	dir := filepath.Dir(dest)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", xerrors.Errorf("create install dir: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".portabledesktop-*")
	if err != nil {
		return "", xerrors.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(binary); err != nil {
		_ = tmp.Close()
		return "", xerrors.Errorf("write %s: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		return "", xerrors.Errorf("close %s: %w", tmpName, err)
	}
	if err := os.Chmod(tmpName, 0o755); err != nil {
		return "", xerrors.Errorf("chmod %s: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, dest); err != nil {
		return "", xerrors.Errorf("move binary into place: %w", err)
	}
	return dest, nil
}

// matches reports whether path exists, is executable, and has the embedded
// binary's digest.
func matches(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	if info.IsDir() || info.Mode()&0o111 == 0 || info.Size() != int64(len(binary)) {
		return false, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:]) == SHA256(), nil
}
