package provisionersdk

import (
	"archive/tar"
	"bytes"
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

// Tar archives a directory.
func Tar(directory string, limit int64) ([]byte, error) {
	var buffer bytes.Buffer
	tarWriter := tar.NewWriter(&buffer)
	totalSize := int64(0)
	err := filepath.Walk(directory, func(file string, fileInfo os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		header, err := tar.FileInfoHeader(fileInfo, file)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(directory, file)
		if err != nil {
			return err
		}
		if strings.HasPrefix(rel, ".") {
			// Don't archive hidden files!
			return err
		}
		if strings.Contains(rel, ".tfstate") {
			// Don't store tfstate!
			return err
		}
		header.Name = rel
		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}
		if fileInfo.IsDir() {
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
		return nil, err
	}
	err = tarWriter.Flush()
	if err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// Untar extracts the archive to a provided directory.
func Untar(directory string, archive []byte) error {
	reader := tar.NewReader(bytes.NewReader(archive))
	for {
		header, err := reader.Next()
		if xerrors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		// #nosec
		target := filepath.Join(directory, header.Name)
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
			_, err = io.CopyN(file, reader, (1<<20)*10)
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
