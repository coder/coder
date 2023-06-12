package clistat

import (
	"bufio"
	"bytes"
	"strconv"
	"time"

	"github.com/spf13/afero"
	"golang.org/x/xerrors"
	"tailscale.com/types/ptr"
)

const (
	cgroupV2CPUMax  = "/sys/fs/cgroup/cpu.max"
	cgroupV2CPUStat = "/sys/fs/cgroup/cpu.stat"
)

// ContainerCPU returns the CPU usage of the container cgroup.
// If the system is not containerized, this always returns nil.
func (s *Statter) ContainerCPU() (*Result, error) {
	// Firstly, check if we are containerized.
	if ok, err := IsContainerized(s.fs); err != nil || !ok {
		return nil, nil //nolint: nilnil
	}

	used1, total, err := s.cgroupCPU()
	if err != nil {
		return nil, xerrors.Errorf("get cgroup CPU usage: %w", err)
	}
	<-time.After(s.sampleInterval)

	// total is unlikely to change. Use the first value.
	used2, _, err := s.cgroupCPU()
	if err != nil {
		return nil, xerrors.Errorf("get cgroup CPU usage: %w", err)
	}

	r := &Result{
		Unit:  "cores",
		Used:  (used2 - used1).Seconds(),
		Total: ptr.To(total.Seconds() / s.sampleInterval.Seconds()), // close enough to the truth
	}
	return r, nil
}

func (s *Statter) cgroupCPU() (used, total time.Duration, err error) {
	if s.isCGroupV2() {
		return s.cGroupV2CPU()
	}

	// Fall back to CGroupv1
	return s.cGroupV1CPU()
}

func (s *Statter) isCGroupV2() bool {
	// Check for the presence of /sys/fs/cgroup/cpu.max
	_, err := s.fs.Stat("/sys/fs/cgroup/cpu.max")
	return err == nil
}

func (s *Statter) cGroupV2CPU() (used, total time.Duration, err error) {
	total, err = s.cGroupv2CPUTotal()
	if err != nil {
		return 0, 0, xerrors.Errorf("get cgroup v2 CPU cores: %w", err)
	}

	used, err = s.cGroupv2CPUUsed()
	if err != nil {
		return 0, 0, xerrors.Errorf("get cgroup v2 CPU used: %w", err)
	}

	return used, total, nil
}

func (s *Statter) cGroupv2CPUUsed() (used time.Duration, err error) {
	var data []byte
	data, err = afero.ReadFile(s.fs, cgroupV2CPUStat)
	if err != nil {
		return 0, xerrors.Errorf("read /sys/fs/cgroup/cpu.stat: %w", err)
	}

	bs := bufio.NewScanner(bytes.NewReader(data))
	for bs.Scan() {
		line := bs.Bytes()
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

func (s *Statter) cGroupv2CPUTotal() (total time.Duration, err error) {
	var data []byte
	var quotaUs int
	data, err = afero.ReadFile(s.fs, "/sys/fs/cgroup/cpu.max")
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
		quotaUs = s.nproc * int(time.Second.Microseconds())
	} else {
		quotaUs, err = strconv.Atoi(string(parts[0]))
		if err != nil {
			return 0, xerrors.Errorf("parse /sys/fs/cgroup/cpu.max: %w", err)
		}
	}

	return time.Duration(quotaUs) * time.Microsecond, nil
}

func (*Statter) cGroupV1CPU() (time.Duration, time.Duration, error) {
	// TODO: implement
	return 0, 0, nil
}

// ContainerMemory returns the memory usage of the container cgroup.
// If the system is not containerized, this always returns nil.
func (s *Statter) ContainerMemory() (*Result, error) {
	if ok, err := IsContainerized(s.fs); err != nil || !ok {
		return nil, nil
	}

	if s.isCGroupV2() {
		return s.cGroupv2Memory()
	}

	// Fall back to CGroupv1
	return s.cGroupv1Memory()
}

func (*Statter) cGroupv2Memory() (*Result, error) {
	// TODO implement
	return nil, nil
}

func (*Statter) cGroupv1Memory() (*Result, error) {
	// TODO implement
	return nil, nil
}
