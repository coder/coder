//go:build unix

package agentpreview

import (
	"os"
	"syscall"
)

// isHardLinked reports whether f may have another link outside the preview.
// It returns true when the link count cannot be read.
func isHardLinked(_ *os.File, info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return !ok || stat.Nlink > 1
}
