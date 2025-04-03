package archivefs

import (
	"archive/tar"
	"io"
	"io/fs"

	"github.com/spf13/afero"
	"github.com/spf13/afero/tarfs"
)

func FromTarReader(r io.Reader) fs.FS {
	tr := tar.NewReader(r)
	tfs := afero.Afero{Fs: tarfs.New(tr)}
	return afero.NewIOFS(tfs)
}
