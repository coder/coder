package archivefs

import (
	"archive/tar"
	"io"
	"io/fs"

	"github.com/spf13/afero"
	"github.com/spf13/afero/tarfs"
)

// FromTarReader creates a read-only in-memory FS
func FromTarReader(r io.Reader) fs.FS {
	tr := tar.NewReader(r)
	return afero.NewIOFS(tarfs.New(tr))
}
