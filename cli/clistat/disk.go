//go:build !windows

package clistat

import (
	"syscall"

	"tailscale.com/types/ptr"
)

// Disk returns the disk usage of the given path.
// If path is empty, it returns the usage of the root directory.
func (*Statter) Disk(path string) (*Result, error) {
	if path == "" {
		path = "/"
	}
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return nil, err
	}
	var r Result
	r.Total = ptr.To(float64(stat.Blocks * uint64(stat.Bsize)))
	r.Used = float64(stat.Blocks-stat.Bfree) * float64(stat.Bsize)
	r.Unit = "B"
	return &r, nil
}
