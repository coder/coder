//go:build !windows

package cli

import (
	"io"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
	"golang.org/x/xerrors"
)

func nativeReadPrivate(path string) ([]byte, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int64(stat.Uid) != int64(os.Geteuid()) || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 || info.Size() > nativePrivateLimit {
		return nil, xerrors.New("native agent file must be an owner-only regular file")
	}
	data, err := io.ReadAll(io.LimitReader(file, nativePrivateLimit+1))
	if err != nil {
		return nil, err
	}
	if len(data) > nativePrivateLimit {
		return nil, xerrors.New("native agent file is too large")
	}
	return data, nil
}

func nativeCheckPrivateDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int64(stat.Uid) != int64(os.Geteuid()) || !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		return xerrors.New("native agent directory must be an owner-only directory, not a symlink")
	}
	return nil
}

func nativePrivateDirectory(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return xerrors.New("create private native agent directory")
	}
	return nativeCheckPrivateDirectory(path)
}
