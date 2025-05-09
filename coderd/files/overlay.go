package files

import (
	"io/fs"
	"path"
	"strings"
)

type overlay struct {
	baseFS      fs.FS
	overlayFS   fs.FS
	overlayPath string
}

func NewOverlayFS(baseFS, overlayFS fs.FS, overlayPath string) fs.FS {
	return overlay{
		baseFS:      baseFS,
		overlayFS:   overlayFS,
		overlayPath: path.Clean(overlayPath),
	}
}

func (f overlay) Open(p string) (fs.File, error) {
	if strings.HasPrefix(path.Clean(p), f.overlayPath) {
		return f.overlayFS.Open(p)
	}
	return f.baseFS.Open(p)
}

func (f overlay) ReadDir(p string) ([]fs.DirEntry, error) {
	if strings.HasPrefix(path.Clean(p), f.overlayPath) {
		return f.overlayFS.(fs.ReadDirFS).ReadDir(p)
	}
	return f.baseFS.(fs.ReadDirFS).ReadDir(p)
}

func (f overlay) ReadFile(p string) ([]byte, error) {
	if strings.HasPrefix(path.Clean(p), f.overlayPath) {
		return f.overlayFS.(fs.ReadFileFS).ReadFile(p)
	}
	return f.baseFS.(fs.ReadFileFS).ReadFile(p)
}
