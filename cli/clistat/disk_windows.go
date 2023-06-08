package clistat

import (
	"golang.org/x/sys/windows"
	"tailscale.com/types/ptr"
)

// Disk returns the disk usage of the given path.
// If path is empty, it defaults to C:\
func (s *Statter) Disk(path string) (*Result, error) {
	if path == "" {
		path = `C:\`
	}

	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}

	var freeBytes, totalBytes, availBytes uint64
	if err := windows.GetDiskFreeSpaceEx(
		pathPtr,
		&freeBytes,
		&totalBytes,
		&availBytes,
	); err != nil {
		return nil, err
	}

	var r Result
	r.Total = ptr.To(float64(totalBytes) / 1024 / 1024 / 1024)
	r.Used = float64(totalBytes-freeBytes) / 1024 / 1024 / 1024
	r.Unit = "GB"
	return &r, nil
}
