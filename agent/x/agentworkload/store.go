package agentworkload

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/gofrs/flock"
	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

type store struct {
	root *os.Root
	lock *flock.Flock
}

func openStore(path string) (*store, error) {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return nil, xerrors.New("create private workload state directory")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0o077 != 0 || !privateOwner(info) {
		return nil, xerrors.New("state_dir must be an owner-only directory (0700)")
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, xerrors.New("open workload state directory")
	}
	lockPath := filepath.Join(path, "supervisor.lock")
	f, err := root.OpenFile("supervisor.lock", os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err == nil {
		_ = f.Close()
	} else if !errors.Is(err, os.ErrExist) {
		_ = root.Close()
		return nil, xerrors.New("create workload state lock")
	}
	if _, err := readPrivateFile(lockPath, 1024); err != nil {
		_ = root.Close()
		return nil, xerrors.New("invalid workload state lock")
	}
	lock := flock.New(lockPath)
	ok, err := lock.TryLock()
	if err != nil || !ok {
		_ = root.Close()
		return nil, xerrors.New("workload supervisor already running or state lock unavailable")
	}
	st := &store{root: root, lock: lock}
	// Persist newly created ancestor entries as well as snapshots. Otherwise
	// a host crash could lose the directory after a completed run was removed.
	for parent := filepath.Dir(filepath.Clean(path)); ; parent = filepath.Dir(parent) {
		dir, err := os.Open(parent)
		if err != nil {
			_ = st.close()
			return nil, xerrors.New("open workload state parent for durability")
		}
		err = dir.Sync()
		_ = dir.Close()
		if err != nil {
			_ = st.close()
			return nil, xerrors.New("sync workload state parent")
		}
		if parent == filepath.Dir(parent) {
			break
		}
	}
	return st, nil
}

func (s *store) read(name string, limit int64, value any) error {
	info, err := s.root.Lstat(name)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 || !privateOwner(info) || info.Size() > limit {
		return xerrors.New("invalid private state file")
	}
	f, err := s.root.Open(name)
	if err != nil {
		return err
	}
	defer f.Close()
	return decodeJSON(io.LimitReader(f, limit+1), value)
}

func (s *store) write(name string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	tmp := ".write-" + uuid.NewString()
	f, err := s.root.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	//nolint:errcheck // Best-effort cleanup; a successful rename removes tmp.
	defer func() { _ = s.root.Remove(tmp) }()
	_, writeErr := f.Write(data)
	syncErr := f.Sync()
	closeErr := f.Close()
	if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
		return err
	}
	if err := s.root.Rename(tmp, name); err != nil {
		return err
	}
	dir, err := s.root.Open(".")
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func (s *store) close() error {
	return errors.Join(s.lock.Unlock(), s.lock.Close(), s.root.Close())
}
