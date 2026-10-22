package agentfiles

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
	"golang.org/x/xerrors"
)

// renameNoReplace atomically renames tmp to path unless path exists.
// It returns an error matching os.ErrExist when path exists, and
// errNoReplaceUnsupported when neither renameat2(RENAME_NOREPLACE) nor
// a hard link works on the filesystem.
func renameNoReplace(tmp, path string) error {
	err := unix.Renameat2(unix.AT_FDCWD, tmp, unix.AT_FDCWD, path, unix.RENAME_NOREPLACE)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, unix.EEXIST):
		return xerrors.Errorf("%s already exists: %w", path, os.ErrExist)
	case errors.Is(err, unix.EINVAL), errors.Is(err, unix.ENOTSUP), errors.Is(err, unix.ENOSYS):
		return linkNoReplace(tmp, path)
	default:
		return xerrors.Errorf("rename %s: %w", path, err)
	}
}
