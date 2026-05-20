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

	for i := 1; i <= maxUploadChatFileCollisionAttempts; i++ {
		candidate := chatfiles.AddCollisionSuffix(name, i)
		fd, err := unix.Openat(
			filesFD,
			candidate,
			unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW,
			0o644,
		)
		if err != nil {
			if errors.Is(err, unix.EEXIST) {
				continue
			}
			if errors.Is(err, unix.ELOOP) {
				return "", "", 0, xerrors.Errorf("%w: %s", errUploadDirSymlink, filepath.Join(dir, candidate))
			}
			return "", "", 0, xerrors.Errorf("create upload target: %w", err)
		}

		f := os.NewFile(uintptr(fd), candidate)
		n, err := io.Copy(f, r)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			_ = unix.Unlinkat(filesFD, candidate, 0)
			return "", "", 0, xerrors.Errorf("write upload: %w", err)
		}
		return candidate, filepath.Join(dir, candidate), n, nil
	}
	return "", "", 0, errUploadCollisionExhausted
}

func openWorkspaceUploadDirNoFollow(homeDir, chatID string) (fd int, err error) {
	current, err := unix.Open(homeDir, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, xerrors.Errorf("open home directory: %w", err)
	}

	for _, component := range []string{".coder", "chats", chatID, chatfiles.WorkspaceUploadFilesSubdir} {
		if err := unix.Mkdirat(current, component, 0o755); err != nil && !errors.Is(err, unix.EEXIST) {
			_ = unix.Close(current)
			return -1, xerrors.Errorf("create upload directory: %w", err)
		}
		next, err := unix.Openat(current, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if err != nil {
			_ = unix.Close(current)
			if errors.Is(err, unix.ELOOP) || errors.Is(err, unix.ENOTDIR) {
				return -1, xerrors.Errorf("%w: %s", errUploadDirSymlink, filepath.Join(homeDir, component))
			}
			return -1, xerrors.Errorf("open upload directory: %w", err)
		}
		_ = unix.Close(current)
		current = next
	}
	return current, nil
}
