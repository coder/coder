package agentfiles

import (
	"errors"
	"os"

	"golang.org/x/xerrors"
)

// errNoReplaceUnsupported reports that the filesystem cannot publish a
// file without replacing an existing one.
var errNoReplaceUnsupported = xerrors.New("publishing without replacement is not supported")

// linkNoReplace publishes tmp at path with a hard link, which fails if
// path exists, then removes tmp. It returns an error matching
// os.ErrExist when path exists, and errNoReplaceUnsupported for any
// other link failure, such as a filesystem without hard links.
func linkNoReplace(tmp, path string) error {
	if err := os.Link(tmp, path); err != nil {
		if errors.Is(err, os.ErrExist) {
			return xerrors.Errorf("%s already exists: %w", path, os.ErrExist)
		}
		return xerrors.Errorf("link %s: %v: %w", path, err, errNoReplaceUnsupported)
	}
	_ = os.Remove(tmp)
	return nil
}
