package archive

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"log"
	"strings"
)

// CreateTarFromZip converts the given zipReader to a tar archive.
func CreateTarFromZip(zipReader *zip.Reader, maxSize int64) ([]byte, error) {
	var tarBuffer bytes.Buffer
	err := writeTarArchive(&tarBuffer, zipReader, maxSize)
	if err != nil {
		return nil, err
	}
	return tarBuffer.Bytes(), nil
}

func writeTarArchive(w io.Writer, zipReader *zip.Reader, maxSize int64) error {
	tarWriter := tar.NewWriter(w)
	defer tarWriter.Close()

	for _, file := range zipReader.File {
		err := processFileInZipArchive(file, tarWriter, maxSize)
		if err != nil {
			return err
		}
	}
	return nil
}

func processFileInZipArchive(file *zip.File, tarWriter *tar.Writer, maxSize int64) error {
	fileReader, err := file.Open()
	if err != nil {
		return err
	}
	defer fileReader.Close()

	err = tarWriter.WriteHeader(&tar.Header{
		Name:    file.Name,
		Size:    file.FileInfo().Size(),
		Mode:    int64(file.Mode()),
		ModTime: file.Modified,
		// Note: Zip archives do not store ownership information.
		Uid: 1000,
		Gid: 1000,
	})
	if err != nil {
		return err
	}

	n, err := io.CopyN(tarWriter, fileReader, maxSize)
	log.Println(file.Name, n, err)
	if errors.Is(err, io.EOF) {
		err = nil
	}
	return err
}

// CreateZipFromTar converts the given tarReader to a zip archive.
func CreateZipFromTar(tarReader *tar.Reader, maxSize int64) ([]byte, error) {
	var zipBuffer bytes.Buffer
	err := WriteZip(&zipBuffer, tarReader, maxSize)
	if err != nil {
		return nil, err
	}
	return zipBuffer.Bytes(), nil
}

// WriteZip writes the given tarReader to w.
func WriteZip(w io.Writer, tarReader *tar.Reader, maxSize int64) error {
	zipWriter := zip.NewWriter(w)
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
		// Some versions of unzip do not check the mode on a file entry and
		// simply assume that entries with a trailing path separator (/) are
		// directories, and that everything else is a file. Give them a hint.
		if tarHeader.FileInfo().IsDir() && !strings.HasSuffix(tarHeader.Name, "/") {
			zipHeader.Name += "/"
		}

		zipEntry, err := zipWriter.CreateHeader(zipHeader)
		if err != nil {
			return err
		}

		_, err = io.CopyN(zipEntry, tarReader, maxSize)
		if errors.Is(err, io.EOF) {
			err = nil
		}
		if err != nil {
			return err
		}
	}
	return nil // don't need to flush as we call `writer.Close()`
}
