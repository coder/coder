package provisionersdk

import (
	"archive/tar"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/util/xio"
)

const (
	// TemplateArchiveLimit represents the maximum size of a template in bytes.
	TemplateArchiveLimit = 1 << 20
)

// TarOptions are options for the Tar function.
type TarOptions struct {
	// FollowSymlinks follows symlinks and archives the contents of
	// the linked files/directories instead of archiving the symlink
	// itself. Symlinks that point outside the archive directory are
	// skipped with a warning.
	FollowSymlinks bool
}

func dirHasExt(dir string, exts ...string) (bool, error) {
	dirEnts, err := os.ReadDir(dir)
	if err != nil {
		return false, err
	}

	for _, fi := range dirEnts {
		for _, ext := range exts {
			if strings.HasSuffix(fi.Name(), ext) {
				return true, nil
			}
		}
	}

	return false, nil
}

func DirHasLockfile(dir string) (bool, error) {
	return dirHasExt(dir, ".terraform.lock.hcl")
}

// Tar archives a Terraform directory.
func Tar(w io.Writer, logger slog.Logger, directory string, limit int64) error {
	return TarWithOptions(w, logger, directory, limit, nil)
}

// TarWithOptions archives a Terraform directory with the given options.
func TarWithOptions(w io.Writer, logger slog.Logger, directory string, limit int64, options *TarOptions) error {
	if options == nil {
		options = &TarOptions{}
	}

	// The total bytes written must be under the limit, so use -1
	w = xio.NewLimitWriter(w, limit-1)
	tw := tar.NewWriter(w)

	tfExts := []string{".tf", ".tf.json"}
	hasTf, err := dirHasExt(directory, tfExts...)
	if err != nil {
		return err
	}
	if !hasTf {
		absPath, err := filepath.Abs(directory)
		if err != nil {
			return err
		}

		// Show absolute path to aid in debugging. E.g. showing "." is
		// useless.
		return xerrors.Errorf(
			"%s is not a valid template since it has no %s files",
			absPath, tfExts,
		)
	}

	// Resolve the archive root to an absolute, symlink-free path so
	// that we can reliably detect whether resolved symlink targets
	// escape the template directory.
	absDirectory, err := filepath.EvalSymlinks(directory)
	if err != nil {
		return xerrors.Errorf("eval symlinks for directory: %w", err)
	}
	absDirectory, err = filepath.Abs(absDirectory)
	if err != nil {
		return xerrors.Errorf("abs directory: %w", err)
	}

	// archiveDir recursively walks diskDir writing entries to the
	// tar archive. archiveBase is the path that diskDir maps to
	// inside the archive (relative to directory). When following
	// symlinks, resolved directory symlinks are recursed into by
	// calling archiveDir again with the resolved path on disk but
	// the symlink's name in the archive.
	var archiveDir func(diskDir, archiveBase string) error
	archiveDir = func(diskDir, archiveBase string) error {
		entries, err := os.ReadDir(diskDir)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			diskPath := filepath.Join(diskDir, entry.Name())
			archivePath := filepath.Join(archiveBase, entry.Name())
			rel := archivePath // already relative to directory

			info, err := entry.Info()
			if err != nil {
				return err
			}
			isSymlink := info.Mode()&os.ModeSymlink != 0

			// When following symlinks, resolve and replace info
			// with the target's so the archive gets real content.
			if options.FollowSymlinks && isSymlink {
				resolved, err := filepath.EvalSymlinks(diskPath)
				if err != nil {
					// Broken symlink — skip gracefully.
					logger.Warn(
						context.Background(), "skipping broken symlink",
						slog.F("path", diskPath), slog.Error(err),
					)
					continue
				}
				absResolved, err := filepath.Abs(resolved)
				if err != nil {
					return err
				}
				// Security: skip symlinks escaping the template
				// directory.
				if !isInsideDir(absResolved, absDirectory) {
					logger.Warn(
						context.Background(),
						"skipping symlink that points outside the template directory",
						slog.F("path", diskPath),
						slog.F("target", absResolved),
					)
					continue
				}
				info, err = os.Stat(resolved)
				if err != nil {
					return err
				}
				// Point diskPath to the resolved target so
				// directory recursion and file reads work.
				diskPath = resolved
				isSymlink = false
			}

			if shouldSkipEntry(rel, logger) {
				continue
			}

			// Write tar header.
			var link string
			if isSymlink {
				link, err = os.Readlink(diskPath)
				if err != nil {
					return err
				}
			}
			header, err := tar.FileInfoHeader(info, link)
			if err != nil {
				return err
			}
			header.Name = filepath.ToSlash(rel)
			if info.IsDir() {
				header.Name += "/"
			}
			if err := tw.WriteHeader(header); err != nil {
				return err
			}

			// Recurse into directories.
			if info.IsDir() {
				if err := archiveDir(diskPath, archivePath); err != nil {
					return err
				}
				continue
			}

			// Copy regular file content.
			if info.Mode().IsRegular() {
				f, err := os.Open(diskPath)
				if err != nil {
					return err
				}
				_, err = io.Copy(tw, f)
				f.Close()
				if err != nil {
					if xerrors.Is(err, xio.ErrLimitReached) {
						return xerrors.Errorf("Archive too big. Must be <= %d bytes", limit)
					}
					return err
				}
			}
		}
		return nil
	}

	if err := archiveDir(directory, "."); err != nil {
		if xerrors.Is(err, xio.ErrLimitReached) {
			return xerrors.Errorf("Archive too big. Must be <= %d bytes", limit)
		}
		return err
	}
	return tw.Flush()
}

// shouldSkipEntry reports whether a file at the given relative path
// should be excluded from the archive (hidden files, state files,
// variable definitions, etc.).
func shouldSkipEntry(rel string, logger slog.Logger) bool {
	// We want to allow .terraform.lock.hcl files to be archived.
	// This allows provider plugins to be cached.
	if (strings.HasPrefix(rel, ".") || strings.HasPrefix(filepath.Base(rel), ".")) && filepath.Base(rel) != ".terraform.lock.hcl" {
		return true
	}
	if strings.Contains(rel, ".tfstate") {
		logger.Debug(context.Background(), "skip state", slog.F("name", rel))
		return true
	}
	if rel == "terraform.tfvars" || rel == "terraform.tfvars.json" || strings.HasSuffix(rel, ".auto.tfvars") || strings.HasSuffix(rel, ".auto.tfvars.json") {
		logger.Debug(context.Background(), "skip variable definitions", slog.F("name", rel))
		return true
	}
	return false
}

// isInsideDir reports whether path is inside (or equal to) dir.
// Both path and dir must be absolute and clean.
func isInsideDir(path, dir string) bool {
	// Append separator so "/home/user" doesn't match
	// "/home/userdata".
	dirWithSep := dir + string(filepath.Separator)
	return path == dir || strings.HasPrefix(path, dirWithSep)
}

// Untar extracts the archive to a provided directory.
func Untar(directory string, r io.Reader) error {
	tarReader := tar.NewReader(r)
	for {
		header, err := tarReader.Next()
		if xerrors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if header.Name == "." || strings.Contains(header.Name, "..") {
			continue
		}
		// #nosec
		target := filepath.Join(directory, filepath.FromSlash(header.Name))
		switch header.Typeflag {
		case tar.TypeDir:
			if _, err := os.Stat(target); err != nil {
				if err := os.MkdirAll(target, 0o755); err != nil {
					return err
				}
			}
		case tar.TypeReg:
			// #nosec G115 - Safe conversion as tar header mode fits within uint32
			err := os.MkdirAll(filepath.Dir(target), os.FileMode(header.Mode)|os.ModeDir|100)
			if err != nil {
				return err
			}
			// #nosec G115 - Safe conversion as tar header mode fits within uint32
			file, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			// Max file size of 10MB.
			_, err = io.CopyN(file, tarReader, (1<<20)*10)
			if xerrors.Is(err, io.EOF) {
				err = nil
			}
			if err != nil {
				return err
			}
			_ = file.Close()
		}
	}
}
