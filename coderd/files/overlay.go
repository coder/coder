package files

import (
	"io/fs"
	"path"
	"strings"
)

// overlayFS allows you to "join" together multiple fs.FS. Files in any specific
// overlay will only be accessible if their path starts with the base path
// provided for the overlay. eg. An overlay at the path .terraform/modules
// should contain files with paths inside the .terraform/modules folder.
type overlayFS struct {
	baseFS   fs.FS
	overlays []Overlay
}

type Overlay struct {
	Path string
	fs.FS
}

func NewOverlayFS(baseFS fs.FS, overlays []Overlay) fs.FS {
	return overlayFS{
		baseFS:   baseFS,
		overlays: overlays,
	}
}

func (f overlayFS) Open(p string) (fs.File, error) {
	target := f.baseFS
	for _, overlay := range f.overlays {
		if strings.HasPrefix(path.Clean(p), overlay.Path) {
			target = overlay.FS
			break
		}
	}
	return target.Open(p)
}

func (f overlayFS) ReadDir(p string) ([]fs.DirEntry, error) {
	target := f.baseFS
	for _, overlay := range f.overlays {
		if strings.HasPrefix(path.Clean(p), overlay.Path) {
			target = overlay.FS
			break
		}
	}
	return fs.ReadDir(target, p)
}

func (f overlayFS) ReadFile(p string) ([]byte, error) {
	target := f.baseFS
	for _, overlay := range f.overlays {
		if strings.HasPrefix(path.Clean(p), overlay.Path) {
			target = overlay.FS
			break
		}
	}
	return fs.ReadFile(target, p)
}
