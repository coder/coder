package coderd

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"log"
)

func CreateTarFromZip(zipReader *zip.Reader) ([]byte, error) {
	var tarBuffer bytes.Buffer
	err := writeTarArchive(&tarBuffer, zipReader)
	if err != nil {
		return nil, err
	}
	return tarBuffer.Bytes(), nil
}

func writeTarArchive(tarBuffer *bytes.Buffer, zipReader *zip.Reader) error {
	tarWriter := tar.NewWriter(tarBuffer)
	defer tarWriter.Close()

	for _, file := range zipReader.File {
		err := processFileInZipArchive(file, tarWriter)
		if err != nil {
			return err
		}
	}
	return tarWriter.Flush()
}

func processFileInZipArchive(file *zip.File, tarWriter *tar.Writer) error {
	fileReader, err := file.Open()
	if err != nil {
		return err
	}
	defer fileReader.Close()

	err = tarWriter.WriteHeader(&tar.Header{
		Name: file.Name,
		Size: file.FileInfo().Size(),
		Mode: 0o644,
	})
	if err != nil {
		return err
	}

	n, err := io.CopyN(tarWriter, fileReader, httpFileMaxBytes)
	log.Println(file.Name, n, err)
	if errors.Is(err, io.EOF) {
		err = nil
	}
	return err
}

func CreateZipFromTar(tarReader *tar.Reader) ([]byte, error) {
	var zipBuffer bytes.Buffer
	err := writeZipArchive(&zipBuffer, tarReader)
	if err != nil {
		return nil, err
	}
	return zipBuffer.Bytes(), nil
}

func writeZipArchive(zipBuffer *bytes.Buffer, tarReader *tar.Reader) error {
	zipWriter := zip.NewWriter(zipBuffer)
	defer zipWriter.Close()

	for {
		tarHeader, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return err
		}

		zipHeader, err := zip.FileInfoHeader(tarHeader.FileInfo())
		if err != nil {
			return err
		}
		zipHeader.Name = tarHeader.Name

		zipEntry, err := zipWriter.CreateHeader(zipHeader)
		if err != nil {
			return err
		}

		_, err = io.CopyN(zipEntry, tarReader, httpFileMaxBytes)
		if errors.Is(err, io.EOF) {
			err = nil
		}
		if err != nil {
			return err
		}
	}
	return zipWriter.Flush()
}
