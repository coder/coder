// User docs and tests cover previews only on Linux and macOS.

//go:build linux || darwin

package agentpreview

import (
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

// os.OpenRoot and Root.OpenRoot open the last path component without
// O_DIRECTORY, so a FIFO there would block. Appending "/." makes name an
// intermediate component, which os opens with O_DIRECTORY. Do not
// filepath.Clean these paths; that would drop the "/.".
func openPreviewRoot(name string) (*os.Root, error) {
	return os.OpenRoot(name + "/.")
}

func openPreviewSubroot(root *os.Root, name string) (*os.Root, error) {
	return root.OpenRoot(name + "/.")
}

func openPreviewFile(root *os.Root, name string) (*os.File, error) {
	// O_NONBLOCK keeps a raced-in FIFO from blocking. os.Root follows
	// in-root symlinks even with O_NOFOLLOW, so openFile rejects them
	// with Lstat and SameFile instead.
	return root.OpenFile(name, os.O_RDONLY|unix.O_NONBLOCK, 0)
}

func isHardLinked(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return !ok || stat.Nlink > 1
}
