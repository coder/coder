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
	tfs := tarfs.New(tr)
	rofs := afero.NewReadOnlyFs(tfs)
	return afero.NewIOFS(rofs)
}
