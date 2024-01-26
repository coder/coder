package coderd

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"errors"
	"io"
)

const zipCopyBufferSize = 4096

func createTarFromZip(zipReader *zip.Reader) ([]byte, error) {
	var tarBuffer bytes.Buffer

	tarWriter := tar.NewWriter(&tarBuffer)
	defer tarWriter.Close()

	for _, zipFile := range zipReader.File {
		err := processFileInZipArchive(zipFile, tarWriter)
		if err != nil {
			return nil, err
		}
	}
	return tarBuffer.Bytes(), nil
}

func processFileInZipArchive(zipFile *zip.File, tarWriter *tar.Writer) error {
	zipFileReader, err := zipFile.Open()
	if err != nil {
		return err
	}
	defer zipFileReader.Close()

	tarHeader, err := tar.FileInfoHeader(zipFile.FileInfo(), "")
	if err != nil {
		return err
	}
	tarHeader.Name = zipFile.Name

	err = tarWriter.WriteHeader(tarHeader)
	if err != nil {
		return err
	}

	for {
		_, err := io.CopyN(tarWriter, zipFileReader, zipCopyBufferSize)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}
	}
	return nil
}
