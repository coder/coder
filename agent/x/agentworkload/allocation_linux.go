//go:build linux

package agentworkload

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/xerrors"
)

func privateOwner(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	//nolint:gosec // Linux effective UIDs are nonnegative 32-bit values.
	return ok && uint64(stat.Uid) == uint64(os.Geteuid()) && (info.IsDir() || stat.Nlink == 1)
}

// Inspect visible ancestors, including the mount root in a cgroup namespace.
// This cannot prove that a separately launched daemon has the same allocation.
func checkOuterAllocation(cfg Config) error {
	const root = "/sys/fs/cgroup"
	data, err := os.ReadFile("/proc/self/cgroup")
	if err != nil {
		return xerrors.New("read outer cgroup allocation")
	}
	group := ""
	for _, line := range strings.Split(string(data), "\n") {
		if path, ok := strings.CutPrefix(line, "0::/"); ok {
			group = filepath.Join(root, filepath.Clean("/"+path))
			break
		}
	}
	if group == "" {
		return xerrors.New("workload supervisor requires cgroup v2")
	}
	for {
		for _, file := range []string{"memory.max", "cpu.max"} {
			data, err := os.ReadFile(filepath.Join(group, file))
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return xerrors.New("read outer cgroup limits")
			}
			if err := checkLimit(cfg, file, strings.TrimSpace(string(data))); err != nil {
				return err
			}
		}
		if group == root {
			break
		}
		group = filepath.Dir(group)
		if !strings.HasPrefix(group, root) {
			return xerrors.New("invalid outer cgroup path")
		}
	}
	return nil
}

func checkLimit(cfg Config, file, value string) error {
	parts := strings.Fields(value)
	if len(parts) == 0 {
		return xerrors.New("invalid outer cgroup limit")
	}
	if parts[0] == "max" {
		return nil
	}
	limit, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || limit <= 0 {
		return xerrors.New("invalid outer cgroup limit")
	}
	if file == "memory.max" {
		if cfg.MemoryBytes > limit {
			return xerrors.New("memory_bytes exceeds outer cgroup allocation")
		}
		return nil
	}
	if len(parts) != 2 {
		return xerrors.New("invalid outer CPU limit")
	}
	period, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || period <= 0 {
		return xerrors.New("invalid outer CPU period")
	}
	if cfg.CPU > limit/period {
		return xerrors.New("cpu exceeds outer cgroup allocation")
	}
	return nil
}
