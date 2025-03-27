package archivefs

import (
	"archive/tar"
	"io"
	"io/fs"
	"path"

	"golang.org/x/xerrors"
)

func FromTar(r tar.Reader) (fs.FS, error) {
	fs := FS{files: make(map[string]fs.File)}
	for {
		it, err := r.Next()

		if err != nil {
			return nil, xerrors.Errorf("failed to read tar archive: %w", err)
		}

		// bufio.NewReader(&r).
		content, err := io.ReadAll(&r)
		fs.files[it.Name] = &File{
			info:    it.FileInfo(),
			content: content,
		}
	}

	path.Split(path string)

	return fs, nil
}
