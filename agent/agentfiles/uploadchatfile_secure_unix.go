//go:build unix

package agentfiles

import (
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/afero"
	"golang.org/x/sys/unix"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/x/chatfiles"
)

func (api *API) writeUploadExclusiveSecure(homeDir, chatID, dir, name string, r io.Reader) (finalName, finalPath string, size int64, err error) {
	switch api.filesystem.(type) {
	case afero.OsFs, *afero.OsFs:
	default:
		return "", "", 0, errUploadSecureUnsupported
	}

	filesFD, err := openWorkspaceUploadDirNoFollow(homeDir, chatID)
	if err != nil {
		return "", "", 0, err
	}
	defer func() {
		if cerr := unix.Close(filesFD); err == nil && cerr != nil {
			err = cerr
		}
	}()

	sweepStaleUploadTempsAt(filesFD)

	tmp, err := uploadTempName(name)
	if err != nil {
		return "", "", 0, err
	}
	fd, err := unix.Openat(
		filesFD,
		tmp,
		unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW,
		0o644,
	)
	if err != nil {
		return "", "", 0, xerrors.Errorf("create upload temp file: %w", err)
	}
	defer func() { _ = unix.Unlinkat(filesFD, tmp, 0) }()

	f := os.NewFile(uintptr(fd), tmp)
	n, err := io.Copy(f, r)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return "", "", 0, xerrors.Errorf("write upload: %w", err)
	}

	// linkat never replaces or follows an existing name, so publishing
	// the complete file cannot clobber a concurrent upload or a symlink.
	for i := 1; i <= maxUploadChatFileCollisionAttempts; i++ {
		candidate := chatfiles.AddCollisionSuffix(name, i)
		err := unix.Linkat(filesFD, tmp, filesFD, candidate, 0)
		if errors.Is(err, unix.EEXIST) {
			continue
		}
		if err != nil {
			return "", "", 0, xerrors.Errorf("publish upload: %w", err)
		}
		return candidate, filepath.Join(dir, candidate), n, nil
	}
	return "", "", 0, errUploadCollisionExhausted
}

func sweepStaleUploadTempsAt(dirFD int) {
	fd, err := unix.Openat(dirFD, ".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return
	}
	dir := os.NewFile(uintptr(fd), ".")
	names, _ := dir.Readdirnames(-1)
	_ = dir.Close()
	for _, name := range names {
		if !isUploadTempName(name) {
			continue
		}
		fd, err := unix.Openat(dirFD, name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
		if err != nil {
			continue
		}
		f := os.NewFile(uintptr(fd), name)
		info, err := f.Stat()
		_ = f.Close()
		if err == nil && isStaleUploadTemp(info) {
			_ = unix.Unlinkat(dirFD, name, 0)
		}
	}
}

func openWorkspaceUploadDirNoFollow(homeDir, chatID string) (fd int, err error) {
	current, err := unix.Open(homeDir, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, xerrors.Errorf("open home directory: %w", err)
	}

	openedPath := homeDir
	for _, component := range []string{".coder", "chats", chatID, chatfiles.WorkspaceUploadFilesSubdir} {
		openedPath = filepath.Join(openedPath, component)
		if err := unix.Mkdirat(current, component, 0o755); err != nil && !errors.Is(err, unix.EEXIST) {
			_ = unix.Close(current)
			return -1, xerrors.Errorf("create upload directory: %w", err)
		}
		next, err := unix.Openat(current, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if err != nil {
			_ = unix.Close(current)
			if errors.Is(err, unix.ELOOP) || errors.Is(err, unix.ENOTDIR) {
				return -1, xerrors.Errorf("%w: %s", errUploadDirSymlink, openedPath)
			}
			return -1, xerrors.Errorf("open upload directory: %w", err)
		}
		_ = unix.Close(current)
		current = next
	}
	return current, nil
}
