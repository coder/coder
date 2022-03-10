package provisionersdk

import (
	"archive/tar"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
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
