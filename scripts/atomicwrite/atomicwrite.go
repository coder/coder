package atomicwrite

import (
	"os"
	"path/filepath"

	"golang.org/x/xerrors"
)

// File atomically writes data to the named file. It writes to a
// temporary file in the same directory and renames it so that an
// interrupted write never leaves a partially-written target.
func File(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp.*")
	if err != nil {
		return xerrors.Errorf("create temp file: %w", err)
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return xerrors.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return xerrors.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return xerrors.Errorf("rename temp file: %w", err)
	}
	return nil
}
