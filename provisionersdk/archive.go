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

	// defaultFileMode is used when a tar entry has no mode set.
	defaultFileMode = 0o600
)

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

	err = filepath.Walk(directory, func(file string, fileInfo os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// Skip symlinks entirely. Neither Untar nor extractArchive
		// restores symlinks, so including them is pointless and can
		// be a security concern.
		// See: https://github.com/coder/coder/issues/16163
		if fileInfo.Mode()&os.ModeSymlink != 0 {
			if fileInfo.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		header, err := tar.FileInfoHeader(fileInfo, "")
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

// Untar extracts the archive to a provided directory.
func Untar(directory string, r io.Reader) error {
	tarReader := tar.NewReader(r)
	for {
		header, err := tarReader.Next()
		if xerrors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return xerrors.Errorf("read tar archive: %w", err)
		}

		// Use filepath.IsLocal for robust path traversal protection.
		// This rejects absolute paths, paths containing "..", and
		// other non-local path constructs.
		if !filepath.IsLocal(header.Name) {
			continue
		}

		// Skip symlinks explicitly. Neither Tar nor extractArchive
		// handles restoring symlinks, and they can be a security
		// concern (e.g. symlink-following attacks).
		// See: https://github.com/coder/coder/issues/16163
		if header.Typeflag == tar.TypeSymlink || header.Typeflag == tar.TypeLink {
			continue
		}

		mode := header.FileInfo().Mode()
		if mode == 0 {
			mode = defaultFileMode
		}

		// nolint: gosec // filepath.IsLocal check above prevents traversal.
		target := filepath.Join(directory, filepath.FromSlash(header.Name))
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, mode|os.ModeDir); err != nil {
				return xerrors.Errorf("mkdir %q: %w", target, err)
			}
		case tar.TypeReg:
			err := os.MkdirAll(filepath.Dir(target), 0o755)
			if err != nil {
				return xerrors.Errorf("mkdir parent of %q: %w", target, err)
			}
			file, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR|os.O_TRUNC, mode)
			if err != nil {
				return xerrors.Errorf("create file %q: %w", target, err)
			}
			// Max file size of 10 MiB.
			_, err = io.CopyN(file, tarReader, 10<<20)
			if xerrors.Is(err, io.EOF) {
				err = nil
			}
			if err != nil {
				_ = file.Close()
				return xerrors.Errorf("copy file %q: %w", target, err)
			}
			if err = file.Close(); err != nil {
				return xerrors.Errorf("close file %q: %w", target, err)
			}
		}
	}
}
