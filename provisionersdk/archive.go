package provisionersdk

import (
	"archive/tar"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/xerrors"
)

const (
	// TemplateArchiveLimit represents the maximum size of a template in bytes.
	TemplateArchiveLimit = 1 << 20
)

func dirHasExt(dir string, ext string) (bool, error) {
	dirEnts, err := os.ReadDir(dir)
	if err != nil {
		return false, err
	}

	for _, fi := range dirEnts {
		if strings.HasSuffix(fi.Name(), ext) {
			return true, nil
		}
	}

	return false, nil
}

// Tar archives a Terraform directory.
func Tar(w io.Writer, directory string, limit int64) error {
	tarWriter := tar.NewWriter(w)
	totalSize := int64(0)

	const tfExt = ".tf"
	hasTf, err := dirHasExt(directory, tfExt)
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
			absPath, tfExt,
		)
	}

	err = filepath.Walk(directory, func(file string, fileInfo os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		var link string
		if fileInfo.Mode()&os.ModeSymlink == os.ModeSymlink {
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
		if strings.HasPrefix(rel, ".") || strings.HasPrefix(filepath.Base(rel), ".") {
			if fileInfo.IsDir() && rel != "." {
				// Don't archive hidden files!
				return filepath.SkipDir
			}
			// Don't archive hidden files!
			return nil
		}
		if strings.Contains(rel, ".tfstate") {
			// Don't store tfstate!
			return nil
		}
		// Use unix paths in the tar archive.
		header.Name = filepath.ToSlash(rel)
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
		wrote, err := io.Copy(tarWriter, data)
		if err != nil {
			return err
		}
		totalSize += wrote
		if limit != 0 && totalSize >= limit {
			return xerrors.Errorf("Archive too big. Must be <= %d bytes", limit)
		}
		return data.Close()
	})
	if err != nil {
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
				if err := os.MkdirAll(target, 0755); err != nil {
					return err
				}
			}
		case tar.TypeReg:
			file, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
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
