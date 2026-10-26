//go:build linux

package agentnative

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
	"golang.org/x/xerrors"
)

type privateStamp struct {
	Device   uint64
	Inode    uint64
	Size     int64
	Modified unix.Timespec
	Changed  unix.Timespec
}

type privateDirectory struct {
	file *os.File
	lock *os.File
}

// Every path component is opened relative to an already verified descriptor.
// A rename or symlink swap cannot redirect later private-file operations.
func openPrivateDirectory(path string, mode privateDirectoryMode) (_ *privateDirectory, err error) {
	if !filepath.IsAbs(path) {
		return nil, xerrors.New("native state directory must be absolute")
	}
	path = filepath.Clean(path)
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = unix.Close(fd)
		}
	}()
	for _, component := range strings.Split(strings.TrimPrefix(path, "/"), "/") {
		if component == "" {
			continue
		}
		next, openErr := unix.Openat(fd, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if errors.Is(openErr, unix.ENOENT) && mode == privateDirectoryCreate {
			if mkdirErr := unix.Mkdirat(fd, component, 0o700); mkdirErr != nil && !errors.Is(mkdirErr, unix.EEXIST) {
				return nil, mkdirErr
			}
			if err := unix.Fsync(fd); err != nil {
				return nil, err
			}
			next, openErr = unix.Openat(fd, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		}
		if openErr != nil {
			return nil, xerrors.Errorf("open native state directory: %w", openErr)
		}
		_ = unix.Close(fd)
		fd = next
		var stat unix.Stat_t
		if err := unix.Fstat(fd, &stat); err != nil {
			return nil, err
		}
		if (stat.Uid != 0 && !privateOwnedByCurrentUser(stat.Uid)) || (stat.Mode&0o022 != 0 && stat.Mode&unix.S_ISVTX == 0) {
			return nil, xerrors.New("native state path has unsafe ownership or permissions")
		}
	}
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return nil, err
	}
	if !privateOwnedByCurrentUser(stat.Uid) || stat.Mode&0o077 != 0 {
		return nil, xerrors.New("native state directory must be owner-only (0700)")
	}
	return &privateDirectory{file: os.NewFile(uintptr(fd), path)}, nil
}

func privateOwnedByCurrentUser(uid uint32) bool {
	currentUID := os.Geteuid()
	if currentUID < 0 {
		return false
	}
	return uint64(uid) == uint64(currentUID)
}

func privateFileStamp(fd int) (*privateStamp, error) {
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return nil, err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFREG || !privateOwnedByCurrentUser(stat.Uid) || stat.Mode&0o077 != 0 || stat.Nlink != 1 {
		return nil, xerrors.New("native state files must be private regular files without links (0600)")
	}
	return &privateStamp{Device: stat.Dev, Inode: stat.Ino, Size: stat.Size, Modified: stat.Mtim, Changed: stat.Ctim}, nil
}

func (d *privateDirectory) checkDirectory() error {
	var stat unix.Stat_t
	if err := unix.Fstat(int(d.file.Fd()), &stat); err != nil {
		return err
	}
	if !privateOwnedByCurrentUser(stat.Uid) || stat.Mode&0o077 != 0 || stat.Mode&unix.S_IFMT != unix.S_IFDIR {
		return xerrors.New("native state directory is no longer private")
	}
	return nil
}

func (d *privateDirectory) acquire() error {
	if err := d.checkDirectory(); err != nil {
		return err
	}
	fd, err := unix.Openat(int(d.file.Fd()), "manager.lock", unix.O_RDWR|unix.O_CREAT|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0o600)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(fd), "manager.lock")
	if _, err := privateFileStamp(fd); err != nil {
		_ = file.Close()
		return err
	}
	if err := unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = file.Close()
		return xerrors.New("native session manager is already running or its lock is unavailable")
	}
	d.lock = file
	return nil
}

func (d *privateDirectory) read(name string, limit int) ([]byte, *privateStamp, error) {
	if err := d.checkDirectory(); err != nil {
		return nil, nil, err
	}
	fd, err := unix.Openat(int(d.file.Fd()), name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, nil, err
	}
	file := os.NewFile(uintptr(fd), name)
	defer file.Close()
	before, err := privateFileStamp(fd)
	if err != nil {
		return nil, nil, err
	}
	if before.Size > int64(limit) {
		return nil, nil, xerrors.New("native state file exceeds size limit")
	}
	data, err := io.ReadAll(io.LimitReader(file, int64(limit)+1))
	if err != nil {
		return nil, nil, err
	}
	after, err := privateFileStamp(fd)
	if err != nil {
		return nil, nil, err
	}
	if len(data) > limit || *before != *after {
		return nil, nil, xerrors.New("native state file changed while reading or exceeds size limit")
	}
	return data, after, nil
}

func (d *privateDirectory) stamp(name string) (*privateStamp, error) {
	fd, err := unix.Openat(int(d.file.Fd()), name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if errors.Is(err, unix.ENOENT) {
		return nil, nil //nolint:nilnil // A nil stamp represents an absent optional file in overwrite guards.
	}
	if err != nil {
		return nil, err
	}
	defer unix.Close(fd)
	return privateFileStamp(fd)
}

func (d *privateDirectory) check(name string, expected *privateStamp) error {
	current, err := d.stamp(name)
	if err != nil {
		return err
	}
	if (current == nil) != (expected == nil) || (current != nil && *current != *expected) {
		return xerrors.New("native state file was changed externally; refusing to overwrite it")
	}
	return nil
}

func (d *privateDirectory) write(name string, data []byte, expected *privateStamp) (*privateStamp, error) {
	if err := d.checkDirectory(); err != nil {
		return nil, err
	}
	if err := d.check(name, expected); err != nil {
		return nil, err
	}
	fd := int(d.file.Fd())
	temporary := name + ".tmp"
	// Only validated interrupted temporary files are discarded. Their contents
	// are never promoted over a committed snapshot.
	if stamp, err := d.stamp(temporary); err != nil {
		return nil, err
	} else if stamp != nil {
		if err := unix.Unlinkat(fd, temporary, 0); err != nil {
			return nil, err
		}
	}
	out, err := unix.Openat(fd, temporary, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o600)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(out), temporary)
	defer file.Close()
	defer func() { _ = unix.Unlinkat(fd, temporary, 0) }()
	if _, err := file.Write(data); err != nil {
		return nil, err
	}
	if err := file.Sync(); err != nil {
		return nil, err
	}
	if err := d.check(name, expected); err != nil {
		return nil, err
	}
	if err := unix.Renameat(fd, temporary, fd, name); err != nil {
		return nil, err
	}
	if err := d.file.Sync(); err != nil {
		return nil, err
	}
	return privateFileStamp(out)
}

func (d *privateDirectory) close() error {
	var lockErr error
	if d.lock != nil {
		// A forked child may still hold this open file description until exec.
		// Closing only our descriptor would leave the shared flock held.
		unlockErr := unix.Flock(int(d.lock.Fd()), unix.LOCK_UN)
		lockErr = errors.Join(unlockErr, d.lock.Close())
	}
	return errors.Join(lockErr, d.file.Close())
}
