package archive

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"math"
	"strings"

	"golang.org/x/xerrors"
)

// Ref:
// https://github.com/golang/go/blob/go1.24.0/src/archive/tar/format.go
// https://github.com/golang/go/blob/go1.24.0/src/archive/tar/writer.go
const (
	tarBlockSize     = 512
	tarEndBlockBytes = 2 * tarBlockSize
)

// ErrArchiveTooLarge reports that archive expansion would exceed the
// configured limit.
var ErrArchiveTooLarge = xerrors.New("archive exceeds maximum size")

// ErrInvalidZipContent reports that a ZIP entry is malformed or its
// contents fail validation during conversion.
var ErrInvalidZipContent = xerrors.New("invalid zip content")

// CreateTarFromZip converts the given zipReader to a tar archive.
// maxSize limits the total tar output, including tar metadata.
func CreateTarFromZip(zipReader *zip.Reader, maxSize int64) ([]byte, error) {
	err := validateZipArchiveSize(zipReader, maxSize)
	if err != nil {
		return nil, err
	}

	var tarBuffer bytes.Buffer
	err = writeTarArchive(&tarBuffer, zipReader, maxSize)
	if err != nil {
		return nil, err
	}
	return tarBuffer.Bytes(), nil
}

// validateZipArchiveSize performs a metadata-based preflight size
// check before conversion. The actual tar output limit will still be
// enforced while streaming.
func validateZipArchiveSize(zipReader *zip.Reader, maxSize int64) error {
	if maxSize < 0 {
		return ErrArchiveTooLarge
	}

	maxBytes := uint64(maxSize)
	totalBytes := uint64(tarEndBlockBytes)
	if totalBytes > maxBytes {
		return ErrArchiveTooLarge
	}

	for _, file := range zipReader.File {
		entrySize, err := projectedTarEntrySize(file)
		if err != nil {
			return err
		}
		if entrySize > maxBytes-totalBytes {
			return ErrArchiveTooLarge
		}
		totalBytes += entrySize
	}

	return nil
}

func projectedTarEntrySize(file *zip.File) (uint64, error) {
	// Each tar entry contributes one header block plus its data
	// rounded up to the next tar block boundary.
	size := file.UncompressedSize64
	if remainder := size % tarBlockSize; remainder != 0 {
		padding := tarBlockSize - remainder
		if size > math.MaxUint64-padding {
			return 0, ErrArchiveTooLarge
		}
		size += padding
	}

	if size > math.MaxUint64-tarBlockSize {
		return 0, ErrArchiveTooLarge
	}

	return tarBlockSize + size, nil
}

type limitedWriter struct {
	w         io.Writer
	remaining int64
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if w.remaining <= 0 {
		return 0, ErrArchiveTooLarge
	}

	origLen := len(p)
	if int64(origLen) > w.remaining {
		p = p[:int(w.remaining)]
	}

	n, err := w.w.Write(p)
	// io.Writer may report both written bytes and an error, so
	// account for any accepted bytes before returning the error.
	w.remaining -= int64(n)
	if err != nil {
		return n, err
	}
	if n < origLen {
		return n, ErrArchiveTooLarge
	}
	return n, nil
}

func writeTarArchive(w io.Writer, zipReader *zip.Reader, maxSize int64) error {
	tarWriter := tar.NewWriter(&limitedWriter{
		w:         w,
		remaining: maxSize,
	})

	for _, file := range zipReader.File {
		err := processFileInZipArchive(file, tarWriter)
		if err != nil {
			return err
		}
	}

	return tarWriter.Close()
}

func processFileInZipArchive(file *zip.File, tarWriter *tar.Writer) error {
	fileReader, err := file.Open()
	if err != nil {
		return err
	}
	defer fileReader.Close()

	size := file.FileInfo().Size()
	if size < 0 {
		return ErrArchiveTooLarge
	}

	err = tarWriter.WriteHeader(&tar.Header{
		Name:    file.Name,
		Size:    size,
		Mode:    int64(file.Mode()),
		ModTime: file.Modified,
		// Note: Zip archives do not store ownership information.
		Uid: 1000,
		Gid: 1000,
	})
	if err != nil {
		return err
	}

	_, err = io.CopyN(tarWriter, fileReader, size)
	switch {
	case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
		return ErrInvalidZipContent
	case errors.Is(err, zip.ErrChecksum), errors.Is(err, zip.ErrFormat):
		return ErrInvalidZipContent
	case err != nil:
		return err
	default:
		return nil
	}
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
