package desktopruntime

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/klauspost/compress/zstd"
	"golang.org/x/xerrors"
)

// ErrNotAvailable is returned by Unpack when no runtime archive is compiled
// into this binary.
var ErrNotAvailable = xerrors.New("desktop runtime is not embedded in this binary")

// DefaultCacheDir returns the directory under which runtimes are unpacked:
// $XDG_CACHE_HOME/coder, falling back to ~/.cache/coder.
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

// RuntimeDir returns the directory the embedded runtime unpacks into below
// cacheDir. The name includes a prefix of the archive digest so that a new
// agent binary never reuses a runtime unpacked by an older one.
func RuntimeDir(cacheDir string) string {
	return filepath.Join(cacheDir, "desktop-runtime-"+SHA256()[:16])
}

// Unpack extracts the embedded runtime below cacheDir if it is not already
// present and returns the runtime root. Extraction happens into a temporary
// sibling directory that is renamed into place, so concurrent callers and
// interrupted runs never observe a partially written runtime.
func Unpack(ctx context.Context, cacheDir string) (string, error) {
	if !Available() {
		return "", ErrNotAvailable
	}
	dir := RuntimeDir(cacheDir)
	if err := Validate(dir); err == nil {
		return dir, nil
	}

	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", xerrors.Errorf("create cache dir: %w", err)
	}
	tmp, err := os.MkdirTemp(cacheDir, ".desktop-runtime-*")
	if err != nil {
		return "", xerrors.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmp)

	if err := extract(ctx, Archive(), tmp); err != nil {
		return "", xerrors.Errorf("extract runtime: %w", err)
	}
	if err := Validate(tmp); err != nil {
		return "", xerrors.Errorf("extracted runtime is invalid: %w", err)
	}

	err = os.Rename(tmp, dir)
	switch {
	case err == nil:
	case errors.Is(err, os.ErrExist), errors.Is(err, syscall.ENOTEMPTY):
		// Another process won the race. Use its copy if it is valid.
		if verr := Validate(dir); verr != nil {
			return "", xerrors.Errorf("runtime dir %q exists but is invalid: %w", dir, verr)
		}
	default:
		return "", xerrors.Errorf("move runtime into place: %w", err)
	}
	return dir, nil
}

// Validate checks that dir looks like an unpacked runtime root.
func Validate(dir string) error {
	info, err := os.Stat(filepath.Join(dir, XvncPath))
	if err != nil {
		return err
	}
	if info.IsDir() || info.Mode()&0o111 == 0 {
		return xerrors.Errorf("%s is not an executable file", XvncPath)
	}
	return nil
}

func extract(ctx context.Context, data []byte, dest string) error {
	zr, err := zstd.NewReader(nil)
	if err != nil {
		return xerrors.Errorf("create zstd reader: %w", err)
	}
	defer zr.Close()
	if err := zr.Reset(bytes.NewReader(data)); err != nil {
		return xerrors.Errorf("reset zstd reader: %w", err)
	}
	tr := tar.NewReader(zr)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return xerrors.Errorf("read tar header: %w", err)
		}
		target, err := securePath(dest, hdr.Name)
		if err != nil {
			return err
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return xerrors.Errorf("create dir %q: %w", hdr.Name, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return xerrors.Errorf("create parent of %q: %w", hdr.Name, err)
			}
			//nolint:gosec // Mode is a 12-bit permission set; Perm masks it.
			mode := os.FileMode(hdr.Mode).Perm() | 0o600
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
			if err != nil {
				return xerrors.Errorf("create file %q: %w", hdr.Name, err)
			}
			// The archive is compiled into this binary, so its size is
			// bounded by the build and it is not attacker controlled.
			//nolint:gosec // G110: trusted embedded archive.
			_, err = io.Copy(f, tr)
			cerr := f.Close()
			if err != nil {
				return xerrors.Errorf("write file %q: %w", hdr.Name, err)
			}
			if cerr != nil {
				return xerrors.Errorf("close file %q: %w", hdr.Name, cerr)
			}
		case tar.TypeSymlink:
			// Only allow links that stay inside the runtime root.
			if filepath.IsAbs(hdr.Linkname) {
				return xerrors.Errorf("absolute symlink %q -> %q rejected", hdr.Name, hdr.Linkname)
			}
			//nolint:gosec // G305: securePath rejects anything outside dest.
			if _, err := securePath(dest, filepath.Join(filepath.Dir(hdr.Name), hdr.Linkname)); err != nil {
				return xerrors.Errorf("symlink %q escapes runtime root: %w", hdr.Name, err)
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return xerrors.Errorf("create parent of %q: %w", hdr.Name, err)
			}
			if err := os.Symlink(hdr.Linkname, target); err != nil {
				return xerrors.Errorf("create symlink %q: %w", hdr.Name, err)
			}
		default:
			// Hard links, devices and other entry types are not expected in
			// the runtime archive and would be a build error.
			return xerrors.Errorf("unsupported tar entry type %d for %q", hdr.Typeflag, hdr.Name)
		}
	}
}

// securePath joins name onto dest and rejects anything that would land
// outside dest.
func securePath(dest, name string) (string, error) {
	if filepath.IsAbs(name) {
		return "", xerrors.Errorf("tar entry %q escapes destination", name)
	}
	clean := filepath.Clean(name)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", xerrors.Errorf("tar entry %q escapes destination", name)
	}
	return filepath.Join(dest, clean), nil
}
