package provisionersdk

import (
	"archive/tar"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/afero"
	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/coderd/util/xio"
)

const (
	// TemplateArchiveLimit represents the maximum size of a template in bytes.
	TemplateArchiveLimit = 1 << 20
)

// osFS is an afero.Fs implementation that performs real filesystem operations.
// It's initialized this way because NewOsFs() returns the abstract fs implementation
// when we require the ability to resolve symbolic links.
var osFS LinkReaderFS = &afero.OsFs{}

// LinkReaderFS is an afero.Fs that supports reading symlinks.
type LinkReaderFS interface {
	afero.Fs
	afero.LinkReader
}

func dirHasExt(afs afero.Fs, dir string, exts ...string) (bool, error) {
	dirEnts, err := afero.ReadDir(afs, dir)
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
	return DirHasLockfileFS(osFS, dir)
}

func DirHasLockfileFS(afs afero.Fs, dir string) (bool, error) {
	return dirHasExt(afs, dir, ".terraform.lock.hcl")
}

// Tar archives a Terraform directory.
func Tar(w io.Writer, logger slog.Logger, directory string, limit int64) error {
	return TarFS(osFS, w, logger, directory, limit)
}

// TarFS archives a Terraform directory to afs.
// The filesystem **must** support reading symbolic links.
func TarFS(afs LinkReaderFS, w io.Writer, logger slog.Logger, directory string, limit int64) error {
	// The total bytes written must be under the limit, so use -1
	w = xio.NewLimitWriter(w, limit-1)
	tarWriter := tar.NewWriter(w)

	tfExts := []string{".tf", ".tf.json"}
	hasTf, err := dirHasExt(afs, directory, tfExts...)
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

	err = afero.Walk(afs, directory, func(file string, fileInfo os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		var link string
		if fileInfo.Mode()&os.ModeSymlink == os.ModeSymlink {
			link, err = afs.ReadlinkIfPossible(file)
			if errors.Is(err, afero.ErrNoReadlink) {
				// It's unclear how we could hit this code path -- if the underlying FS implementation
				// does not support symbolic links, how could there exist a symlink on the FS?
				// Instead of blocking though, just log the error and continue without resolving the
				// symlink.
				logger.Error(context.Background(), "developer error: fs %T should support reading symlinks yet still returned %w", afs, err)
			} else if err != nil {
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
		// We want to allow .terraform.lock.hcl files to be archived. This
		// allows provider plugins to be cached.
		if (strings.HasPrefix(rel, ".") || strings.HasPrefix(filepath.Base(rel), ".")) && filepath.Base(rel) != ".terraform.lock.hcl" {
			if fileInfo.IsDir() && rel != "." {
				// Don't archive hidden files!
				return filepath.SkipDir
			}
			// Don't archive hidden files!
			return nil
		}
		if strings.Contains(rel, ".tfstate") {
			// Don't store tfstate!
			logger.Debug(context.Background(), "skip state", slog.F("name", rel))
			return nil
		}
		if rel == "terraform.tfvars" || rel == "terraform.tfvars.json" || strings.HasSuffix(rel, ".auto.tfvars") || strings.HasSuffix(rel, ".auto.tfvars.json") {
			// Don't store .tfvars, as Coder uses their own variables file.
			logger.Debug(context.Background(), "skip variable definitions", slog.F("name", rel))
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

		data, err := afs.Open(file)
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

// Untar extracts the archive to a provided directory.
func Untar(directory string, r io.Reader) error {
	return UntarFS(osFS, directory, r)
}

// UntarFS extracts the archive to a provided directory in afs.
func UntarFS(afs afero.Fs, directory string, r io.Reader) error {
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
			if _, err := afs.Stat(target); err != nil {
				if err := afs.MkdirAll(target, 0o755); err != nil {
					return err
				}
			}
		case tar.TypeReg:
			err := afs.MkdirAll(filepath.Dir(target), os.FileMode(header.Mode)|os.ModeDir|100)
			if err != nil {
				return err
			}
			file, err := afs.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
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
