//go:build windows

package agentpreview

import (
	"os"

	"golang.org/x/sys/windows"
)

// isHardLinked reports whether f may have another link outside the preview.
// It returns true when the link count cannot be read. It reads the count from
// the open file, because os.FileInfo does not report it on Windows.
func isHardLinked(f *os.File, _ os.FileInfo) bool {
	var info windows.ByHandleFileInformation
	err := windows.GetFileInformationByHandle(windows.Handle(f.Fd()), &info)
	return err != nil || info.NumberOfLinks > 1
}
