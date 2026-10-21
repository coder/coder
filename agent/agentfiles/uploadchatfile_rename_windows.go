//go:build windows

package agentfiles

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/afero"
	"golang.org/x/sys/windows"
)

// renameUpload publishes a finished upload temp file. os.Rename returns
// before the rename reaches the disk, and coderd drops its only stored
// copy of a write-once file once the upload is acknowledged, so
// write-once uploads rename with MOVEFILE_WRITE_THROUGH.
func (api *API) renameUpload(from, to string, mode uploadPublishMode) error {
	switch api.filesystem.(type) {
	case afero.OsFs, *afero.OsFs:
	default:
		return api.filesystem.Rename(from, to)
	}
	if mode != uploadPublishWriteOnce {
		return api.filesystem.Rename(from, to)
	}
	from16, err := windows.UTF16PtrFromString(extendedLengthPath(from))
	if err != nil {
		return &os.LinkError{Op: "rename", Old: from, New: to, Err: err}
	}
	to16, err := windows.UTF16PtrFromString(extendedLengthPath(to))
	if err != nil {
		return &os.LinkError{Op: "rename", Old: from, New: to, Err: err}
	}
	if err := windows.MoveFileEx(from16, to16, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH); err != nil {
		return &os.LinkError{Op: "rename", Old: from, New: to, Err: err}
	}
	return nil
}

// extendedLengthPath lets MoveFileEx accept upload paths longer than
// MAX_PATH, which os.Rename handles internally.
func extendedLengthPath(p string) string {
	const maxShortPath = 248
	if len(p) < maxShortPath || !filepath.IsAbs(p) || strings.HasPrefix(p, `\\`) {
		return p
	}
	return `\\?\` + filepath.Clean(p)
}
