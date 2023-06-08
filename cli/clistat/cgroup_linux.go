package clistat

import (
	"bufio"
	"bytes"
	"os"
	"strconv"
	"time"

	"golang.org/x/xerrors"
	"tailscale.com/types/ptr"
)

const ()

// CGroupCPU returns the CPU usage of the container cgroup.
// On non-Linux platforms, this always returns nil.
func (s *Statter) ContainerCPU() (*Result, error) {
	// Firstly, check if we are containerized.
	if ok, err := IsContainerized(); err != nil || !ok {
		return nil, nil //nolint: nilnil
	}

	used1, total, err := cgroupCPU()
	if err != nil {
		return nil, xerrors.Errorf("get cgroup CPU usage: %w", err)
	}
	<-time.After(s.sampleInterval)

	// total is unlikely to change. Use the first value.
	used2, _, err := cgroupCPU()
	if err != nil {
		return nil, xerrors.Errorf("get cgroup CPU usage: %w", err)
	}

	r := &Result{
		Unit:  "cores",
		Used:  (used2 - used1).Seconds(),
		Total: ptr.To(total.Seconds()), // close enough to the truth
	}
	return r, nil
}

func cgroupCPU() (used, total time.Duration, err error) {
	if isCGroupV2() {
		return cGroupV2CPU()
	}

	// Fall back to CGroupv1
	return cGroupV1CPU()
}

func isCGroupV2() bool {
	// Check for the presence of /sys/fs/cgroup/cpu.max
	_, err := os.Stat("/sys/fs/cgroup/cpu.max")
	return err == nil
}

func cGroupV2CPU() (used, total time.Duration, err error) {
	total, err = cGroupv2CPUTotal()
	if err != nil {
		return 0, 0, xerrors.Errorf("get cgroup v2 CPU cores: %w", err)
	}

	used, err = cGroupv2CPUUsed()
	if err != nil {
		return 0, 0, xerrors.Errorf("get cgroup v2 CPU used: %w", err)
	}

	return used, total, nil
}

func cGroupv2CPUUsed() (used time.Duration, err error) {
	var data []byte
	data, err = os.ReadFile("/sys/fs/cgroup/cpu.stat")
	if err != nil {
		return 0, xerrors.Errorf("read /sys/fs/cgroup/cpu.stat: %w", err)
	}

	s := bufio.NewScanner(bytes.NewReader(data))
	for s.Scan() {
		line := s.Bytes()
		if !bytes.HasPrefix(line, []byte("usage_usec ")) {
			continue
		}

		parts := bytes.Split(line, []byte(" "))
		if len(parts) != 2 {
			return 0, xerrors.Errorf("unexpected line in /sys/fs/cgroup/cpu.stat: %s", line)
		}

		iused, err := strconv.Atoi(string(parts[1]))
		if err != nil {
			return 0, xerrors.Errorf("parse /sys/fs/cgroup/cpu.stat: %w", err)
		}

		return time.Duration(iused) * time.Microsecond, nil
	}

	return 0, xerrors.Errorf("did not find expected usage_usec in /sys/fs/cgroup/cpu.stat")
}

func cGroupv2CPUTotal() (total time.Duration, err error) {
	var data []byte
	var quotaUs int
	data, err = os.ReadFile("/sys/fs/cgroup/cpu.max")
	if err != nil {
		return 0, xerrors.Errorf("read /sys/fs/cgroup/cpu.max: %w", err)
	}

	lines := bytes.Split(data, []byte("\n"))
	if len(lines) < 1 {
		return 0, xerrors.Errorf("unexpected empty /sys/fs/cgroup/cpu.max")
	}

	line := lines[0]
	parts := bytes.Split(line, []byte(" "))
	if len(parts) != 2 {
		return 0, xerrors.Errorf("unexpected line in /sys/fs/cgroup/cpu.max: %s", line)
	}

	if bytes.Equal(parts[0], []byte("max")) {
		quotaUs = nproc * int(time.Second.Microseconds())
	} else {
		quotaUs, err = strconv.Atoi(string(parts[0]))
		if err != nil {
			return 0, xerrors.Errorf("parse /sys/fs/cgroup/cpu.max: %w", err)
		}
	}

	return time.Duration(quotaUs) * time.Microsecond, nil
}

func cGroupV1CPU() (time.Duration, time.Duration, error) {
	// TODO: implement
	return 0, 0, nil
}

func (s *Statter) ContainerMemory() (*Result, error) {
	if ok, err := IsContainerized(); err != nil || !ok {
		return nil, nil
	}

	if isCGroupV2() {
		return cGroupv2Memory()
	}

	// Fall back to CGroupv1
	return cGroupv1Memory()
}

func cGroupv2Memory() (*Result, error) {
	// TODO implement
	return nil, nil
}

func cGroupv1Memory() (*Result, error) {
	// TODO implement
	return nil, nil
}
