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

// Tar archives a directory.
func Tar(directory string) ([]byte, error) {
	var buffer bytes.Buffer
	tarWriter := tar.NewWriter(&buffer)
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
		if _, err := io.Copy(tarWriter, data); err != nil {
			return err
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
		target := filepath.Join(directory, header.Name)
		switch header.Typeflag {
		case tar.TypeDir:
			if _, err := os.Stat(target); err != nil {
				if err := os.MkdirAll(target, 0755); err != nil {
					return err
				}
			}
		case tar.TypeReg:
			f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, reader); err != nil {
				return err
			}
			_ = f.Close()
		}
	}
}
