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
	tarWriter := tar.NewWriter(w)

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

	err = filepath.Walk(directory, func(file string, fileInfo os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		isSymlink := fileInfo.Mode()&os.ModeSymlink == os.ModeSymlink

		// When following symlinks, resolve the symlink and use the
		// target's FileInfo so the archive contains the real content
		// instead of a symlink entry.
		if options.FollowSymlinks && isSymlink {
			resolved, resolveErr := filepath.EvalSymlinks(file)
			if resolveErr != nil {
				// Skip broken symlinks instead of failing the
				// entire archive.
				logger.Warn(
					context.Background(), "skipping broken symlink",
					slog.F("path", file), slog.Error(resolveErr),
				)
				return nil
			}
			absResolved, resolveErr := filepath.Abs(resolved)
			if resolveErr != nil {
				return resolveErr
			}
			// Security: reject symlinks that escape the template
			// directory to prevent accidentally bundling files
			// from the broader filesystem.
			if !isInsideDir(absResolved, absDirectory) {
				logger.Warn(
					context.Background(),
					"skipping symlink that points outside the template directory",
					slog.F("path", file),
					slog.F("target", absResolved),
				)
				return nil
			}

			targetInfo, resolveErr := os.Stat(resolved)
			if resolveErr != nil {
				return resolveErr
			}

			// If the symlink points to a directory, filepath.Walk
			// won't descend into it. We need to walk the target
			// ourselves and write each entry under the symlink's
			// name in the archive.
			if targetInfo.IsDir() {
				return walkDir(tarWriter, logger, resolved, directory, file, absDirectory, options, limit)
			}

			fileInfo = targetInfo
			isSymlink = false
		}

		var link string
		if isSymlink {
			link, err = os.Readlink(file)
			if err != nil {
				return err
			}
		}
		header, err := tar.FileInfoHeader(fileInfo, link)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(directory, file)
		if err != nil {
			return err
		}
		if shouldSkipEntry(rel, logger) {
			if fileInfo.IsDir() && rel != "." {
				return filepath.SkipDir
			}
			return nil
		}
		// Use unix paths in the tar archive.
		header.Name = filepath.ToSlash(rel)
		// tar.FileInfoHeader() will do this, but filepath.Rel() calls filepath.Clean()
		// which strips trailing path separators for directories.
		if fileInfo.IsDir() {
			header.Name += "/"
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}
		if !fileInfo.Mode().IsRegular() {
			return nil
		}

		data, err := os.Open(file)
		if err != nil {
			return err
		}
		defer data.Close()
		_, err = io.Copy(tarWriter, data)
		if err != nil {
			if xerrors.Is(err, xio.ErrLimitReached) {
				return xerrors.Errorf("Archive too big. Must be <= %d bytes", limit)
			}
			return err
		}

		return data.Close()
	})
	if err != nil {
		if xerrors.Is(err, xio.ErrLimitReached) {
			return xerrors.Errorf("Archive too big. Must be <= %d bytes", limit)
		}
		return err
	}
	err = tarWriter.Flush()
	if err != nil {
		return err
	}
	return nil
}

// walkDir recursively archives a directory tree (resolvedDir on disk)
// into the tar archive, mapping entries so they appear under archivePath
// relative to baseDir. This is used to inline the contents of a symlinked
// directory when following symlinks.
func walkDir(
	tw *tar.Writer,
	logger slog.Logger,
	resolvedDir, baseDir, archivePath, absBaseDir string,
	options *TarOptions,
	limit int64,
) error {
	return filepath.Walk(resolvedDir, func(file string, fileInfo os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		isSymlink := fileInfo.Mode()&os.ModeSymlink == os.ModeSymlink

		if options.FollowSymlinks && isSymlink {
			resolved, resolveErr := filepath.EvalSymlinks(file)
			if resolveErr != nil {
				logger.Warn(
					context.Background(), "skipping broken symlink",
					slog.F("path", file), slog.Error(resolveErr),
				)
				return nil
			}
			absResolved, resolveErr := filepath.Abs(resolved)
			if resolveErr != nil {
				return resolveErr
			}
			if !isInsideDir(absResolved, absBaseDir) {
				logger.Warn(
					context.Background(),
					"skipping symlink that points outside the template directory",
					slog.F("path", file),
					slog.F("target", absResolved),
				)
				return nil
			}

			targetInfo, resolveErr := os.Stat(resolved)
			if resolveErr != nil {
				return resolveErr
			}
			if targetInfo.IsDir() {
				relFromResolved, resolveErr := filepath.Rel(resolvedDir, file)
				if resolveErr != nil {
					return resolveErr
				}
				nestedArchivePath := filepath.Join(archivePath, relFromResolved)
				return walkDir(tw, logger, resolved, baseDir, nestedArchivePath, absBaseDir, options, limit)
			}
			fileInfo = targetInfo
			isSymlink = false
		}

		// Compute the relative path within the archive.
		relFromResolved, err := filepath.Rel(resolvedDir, file)
		if err != nil {
			return err
		}
		archiveEntry := filepath.Join(archivePath, relFromResolved)
		rel, err := filepath.Rel(baseDir, archiveEntry)
		if err != nil {
			return err
		}

		if shouldSkipEntry(rel, logger) {
			if fileInfo.IsDir() && rel != "." {
				return filepath.SkipDir
			}
			return nil
		}

		var link string
		if isSymlink {
			link, err = os.Readlink(file)
			if err != nil {
				return err
			}
		}
		header, err := tar.FileInfoHeader(fileInfo, link)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(rel)
		if fileInfo.IsDir() {
			header.Name += "/"
		}
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if !fileInfo.Mode().IsRegular() {
			return nil
		}

		data, err := os.Open(file)
		if err != nil {
			return err
		}
		defer data.Close()
		_, err = io.Copy(tw, data)
		if err != nil {
			if xerrors.Is(err, xio.ErrLimitReached) {
				return xerrors.Errorf("Archive too big. Must be <= %d bytes", limit)
			}
			return err
		}

		return data.Close()
	})
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
