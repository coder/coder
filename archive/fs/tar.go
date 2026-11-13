package archivefs

import (
	"archive/tar"
	"bufio"
	"io"
	"io/fs"

	"github.com/spf13/afero"
	"github.com/spf13/afero/tarfs"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/archive"
)

// FromTarReader creates a read-only in-memory FS. It returns an error if r
// is not a readable tar archive.
func FromTarReader(r io.Reader) (fsys fs.FS, err error) {
	br := bufio.NewReader(r)
	// A Peek error means fewer than two bytes are available, which tarfs
	// reads as an empty archive.
	if magic, peekErr := br.Peek(2); peekErr == nil && archive.IsGzip(magic) {
		return nil, archive.ErrGzipCompressed
	}

	// tarfs.New panics when an entry's contents cannot be read in full.
	defer func() {
		if r := recover(); r != nil {
			fsys = nil
			err = xerrors.Errorf("invalid tar archive: %v", r)
		}
	}()
	// tarfs.New returns nil when a tar header cannot be read, and wrapping
	// that nil would panic on the first file access.
	tfs := tarfs.New(tar.NewReader(br))
	if tfs == nil {
		return nil, xerrors.New("invalid tar archive: unable to read tar header")
	}
	return afero.NewIOFS(tfs), nil
}
