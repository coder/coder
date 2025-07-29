package archivefs

import (
	"archive/zip"
	"io"
	"io/fs"

	"github.com/spf13/afero"
	"github.com/spf13/afero/zipfs"
)

// FromZipReader creates a read-only in-memory FS
func FromZipReader(r io.ReaderAt, size int64) (fs.FS, error) {
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return nil, err
	}
	return afero.NewIOFS(zipfs.New(zr)), nil
}
