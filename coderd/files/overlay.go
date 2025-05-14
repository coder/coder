package files

import (
	"io/fs"
	"path"
	"strings"

	"golang.org/x/xerrors"
)

// overlayFS allows you to "join" together the template files tar file fs.FS
// with the Terraform modules tar file fs.FS. We could potentially turn this
// into something more parameterized/configurable, but the requirements here are
// a _bit_ odd, because every file in the modulesFS includes the
// .terraform/modules/ folder at the beginning of it's path.
type overlayFS struct {
	baseFS   fs.FS
	overlays []Overlay
}

type Overlay struct {
	Path string
	fs.FS
}

func NewOverlayFS(baseFS fs.FS, overlays []Overlay) (fs.FS, error) {
	if err := valid(baseFS); err != nil {
		return nil, xerrors.Errorf("baseFS: %w", err)
	}

	for _, overlay := range overlays {
		if err := valid(overlay.FS); err != nil {
			return nil, xerrors.Errorf("overlayFS: %w", err)
		}
	}

	return overlayFS{
		baseFS:   baseFS,
		overlays: overlays,
	}, nil
}

func (f overlayFS) Open(p string) (fs.File, error) {
	for _, overlay := range f.overlays {
		if strings.HasPrefix(path.Clean(p), overlay.Path) {
			return overlay.FS.Open(p)
		}
	}
	return f.baseFS.Open(p)
}

func (f overlayFS) ReadDir(p string) ([]fs.DirEntry, error) {
	for _, overlay := range f.overlays {
		if strings.HasPrefix(path.Clean(p), overlay.Path) {
			//nolint:forcetypeassert
			return overlay.FS.(fs.ReadDirFS).ReadDir(p)
		}
	}
	//nolint:forcetypeassert
	return f.baseFS.(fs.ReadDirFS).ReadDir(p)
}

func (f overlayFS) ReadFile(p string) ([]byte, error) {
	for _, overlay := range f.overlays {
		if strings.HasPrefix(path.Clean(p), overlay.Path) {
			//nolint:forcetypeassert
			return overlay.FS.(fs.ReadFileFS).ReadFile(p)
		}
	}
	//nolint:forcetypeassert
	return f.baseFS.(fs.ReadFileFS).ReadFile(p)
}

// valid checks that the fs.FS implements the required interfaces.
// The fs.FS interface is not sufficient.
func valid(fsys fs.FS) error {
	_, ok := fsys.(fs.ReadDirFS)
	if !ok {
		return xerrors.New("overlayFS does not implement ReadDirFS")
	}
	_, ok = fsys.(fs.ReadFileFS)
	if !ok {
		return xerrors.New("overlayFS does not implement ReadFileFS")
	}
	return nil
}
