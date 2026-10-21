//go:build unix

package agentfiles

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/afero"
	"golang.org/x/sys/unix"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/x/chatfiles"
)

func (api *API) writeUploadExclusiveSecure(target uploadTarget, name string, r io.Reader) (finalName, finalPath string, size int64, err error) {
	switch api.filesystem.(type) {
	case afero.OsFs, *afero.OsFs:
	default:
		return "", "", 0, errUploadSecureUnsupported
	}

	filesFD, err := openWorkspaceUploadDirNoFollow(target)
	if err != nil {
		return "", "", 0, err
	}
	defer func() {
		if cerr := unix.Close(filesFD); err == nil && cerr != nil {
			err = cerr
		}
	}()

	sweepStaleUploadTempsAt(filesFD)

	if target.mode() == uploadPublishWriteOnce {
		size, err = publishedUploadSizeAt(filesFD, name)
		if err != nil {
			return "", "", 0, err
		}
		if size > 0 {
			return name, filepath.Join(target.dir, name), size, nil
		}
	}

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

	candidate, size, err := publishUploadAt(filesFD, tmp, name, n, target.mode(), unix.Linkat)
	if err != nil {
		return "", "", 0, err
	}
	return candidate, filepath.Join(target.dir, candidate), size, nil
}

// publishUploadAt gives the complete temp file its final name. linkat
// never replaces or follows an existing name, so publishing cannot
// clobber a concurrent upload or a symlink. Filesystems without hard
// links fall back to an exclusive placeholder replaced by renameat. In
// write-once mode the per-file directory only receives copies of one
// file, so a placeholder created here is ours, and an existing regular
// file is reported as the published upload with its current size unless
// it is another upload's empty placeholder, which is replaced once it is
// old enough to be abandoned.
func publishUploadAt(dirFD int, tmp, name string, size int64, mode uploadPublishMode, link func(oldDirFD int, oldPath string, newDirFD int, newPath string, flags int) error) (string, int64, error) {
	useLink := true
	for i := 1; i <= maxUploadChatFileCollisionAttempts; i++ {
		candidate := chatfiles.AddCollisionSuffix(name, i)
		if useLink {
			err := link(dirFD, tmp, dirFD, candidate, 0)
			if err == nil {
				return candidate, size, nil
			}
			if errors.Is(err, unix.EEXIST) {
				if mode == uploadPublishWriteOnce {
					return existingUploadAt(dirFD, tmp, candidate, size)
				}
				continue
			}
			if !errors.Is(err, unix.EPERM) && !errors.Is(err, unix.ENOTSUP) &&
				!errors.Is(err, unix.EOPNOTSUPP) && !errors.Is(err, unix.EXDEV) {
				return "", 0, xerrors.Errorf("publish upload: %w", err)
			}
			useLink = false
		}

		fd, err := unix.Openat(dirFD, candidate, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o644)
		if errors.Is(err, unix.EEXIST) {
			if mode == uploadPublishWriteOnce {
				return existingUploadAt(dirFD, tmp, candidate, size)
			}
			continue
		}
		if err != nil {
			return "", 0, xerrors.Errorf("create upload target: %w", err)
		}
		err = unix.Close(fd)
		if err == nil {
			err = unix.Renameat(dirFD, tmp, dirFD, candidate)
		}
		if err != nil {
			_ = unix.Unlinkat(dirFD, candidate, 0)
			return "", 0, xerrors.Errorf("publish upload: %w", err)
		}
		return candidate, size, nil
	}
	return "", 0, errUploadCollisionExhausted
}

// existingUploadAt returns the name and size of the regular file a
// write-once upload of size bytes found at name, without following a
// symlink there. An abandoned placeholder at name is replaced by tmp.
func existingUploadAt(dirFD int, tmp, name string, size int64) (string, int64, error) {
	st, err := statUploadAt(dirFD, name)
	if err != nil {
		return "", 0, err
	}
	replace, err := checkWriteOnceExisting(st.Size, time.Unix(st.Mtim.Unix()), size)
	if err != nil {
		return "", 0, err
	}
	if !replace {
		return name, st.Size, nil
	}
	if err := unix.Renameat(dirFD, tmp, dirFD, name); err != nil {
		return "", 0, xerrors.Errorf("replace abandoned upload placeholder: %w", err)
	}
	return name, size, nil
}

// publishedUploadSizeAt returns the size of the regular file at a
// write-once upload's target name, or 0 when the name is absent.
func publishedUploadSizeAt(dirFD int, name string) (int64, error) {
	st, err := statUploadAt(dirFD, name)
	if errors.Is(err, unix.ENOENT) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return st.Size, nil
}

// statUploadAt describes the regular file at name without following a
// symlink there.
func statUploadAt(dirFD int, name string) (unix.Stat_t, error) {
	var st unix.Stat_t
	if err := unix.Fstatat(dirFD, name, &st, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return st, xerrors.Errorf("check existing upload: %w", err)
	}
	if st.Mode&unix.S_IFMT != unix.S_IFREG {
		return st, xerrors.Errorf("%w: %s", errUploadTargetNotRegular, name)
	}
	return st, nil
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

func openWorkspaceUploadDirNoFollow(target uploadTarget) (fd int, err error) {
	current, err := unix.Open(target.homeDir, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, xerrors.Errorf("open home directory: %w", err)
	}

	components := []string{".coder", "chats", target.chatID, chatfiles.WorkspaceUploadFilesSubdir}
	if target.fileID != "" {
		components = append(components, target.fileID)
	}
	openedPath := target.homeDir
	for _, component := range components {
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
