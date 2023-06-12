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
	cgroupV1CPUAcctUsage = "/sys/fs/cgroup/cpu,cpuacct/cpuacct.usage"
	cgroupV1CFSQuotaUs   = "/sys/fs/cgroup/cpu,cpuacct/cpu.cfs_quota_us"
	cgroupV2CPUMax       = "/sys/fs/cgroup/cpu.max"
	cgroupV2CPUStat      = "/sys/fs/cgroup/cpu.stat"
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
		Used:  (used2 - used1).Seconds() * s.sampleInterval.Seconds(),
		Total: ptr.To(total.Seconds()), // close enough to the truth
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
		return 0, 0, xerrors.Errorf("get cgroup v2 cpu total: %w", err)
	}

	used, err = s.cGroupv2CPUUsed()
	if err != nil {
		return 0, 0, xerrors.Errorf("get cgroup v2 cpu used: %w", err)
	}

	return used, total, nil
}

func (s *Statter) cGroupv2CPUUsed() (used time.Duration, err error) {
	var data []byte
	data, err = afero.ReadFile(s.fs, cgroupV2CPUStat)
	if err != nil {
		return 0, xerrors.Errorf("read %s: %w", cgroupV2CPUStat, err)
	}

	bs := bufio.NewScanner(bytes.NewReader(data))
	for bs.Scan() {
		line := bs.Bytes()
		if !bytes.HasPrefix(line, []byte("usage_usec ")) {
			continue
		}

		parts := bytes.Split(line, []byte(" "))
		if len(parts) != 2 {
			return 0, xerrors.Errorf("unexpected line in %s: %s", cgroupV2CPUStat, line)
		}

		iused, err := strconv.Atoi(string(parts[1]))
		if err != nil {
			return 0, xerrors.Errorf("parse %s: %w", err, cgroupV2CPUStat)
		}

		return time.Duration(iused) * time.Microsecond, nil
	}

	return 0, xerrors.Errorf("did not find expected usage_usec in %s", cgroupV2CPUStat)
}

func (s *Statter) cGroupv2CPUTotal() (total time.Duration, err error) {
	var data []byte
	var quotaUs int64
	data, err = afero.ReadFile(s.fs, cgroupV2CPUMax)
	if err != nil {
		return 0, xerrors.Errorf("read %s: %w", cgroupV2CPUMax, err)
	}

	lines := bytes.Split(data, []byte("\n"))
	if len(lines) < 1 {
		return 0, xerrors.Errorf("unexpected empty %s", cgroupV2CPUMax)
	}

	parts := bytes.Split(lines[0], []byte(" "))
	if len(parts) != 2 {
		return 0, xerrors.Errorf("unexpected line in %s: %s", cgroupV2CPUMax, lines[0])
	}

	if bytes.Equal(parts[0], []byte("max")) {
		quotaUs = int64(s.nproc) * time.Second.Microseconds()
	} else {
		quotaUs, err = strconv.ParseInt(string(parts[0]), 10, 64)
		if err != nil {
			return 0, xerrors.Errorf("parse %s: %w", cgroupV2CPUMax, err)
		}
	}

	return time.Duration(quotaUs) * time.Microsecond, nil
}

func (s *Statter) cGroupV1CPU() (used, total time.Duration, err error) {
	total, err = s.cGroupV1CPUTotal()
	if err != nil {
		return 0, 0, xerrors.Errorf("get cgroup v1 CPU total: %w", err)
	}

	used, err = s.cgroupV1CPUUsed()
	if err != nil {
		return 0, 0, xerrors.Errorf("get cgruop v1 cpu used: %w", err)
	}

	return used, total, nil
}

func (s *Statter) cGroupV1CPUTotal() (time.Duration, error) {
	var data []byte
	var err error
	var quotaUs int64

	data, err = afero.ReadFile(s.fs, cgroupV1CFSQuotaUs)
	if err != nil {
		return 0, xerrors.Errorf("read %s: %w", cgroupV1CFSQuotaUs, err)
	}

	quotaUs, err = strconv.ParseInt(string(bytes.TrimSpace(data)), 10, 64)
	if err != nil {
		return 0, xerrors.Errorf("parse %s: %w", cgroupV1CFSQuotaUs, err)
	}

	if quotaUs < 0 {
		quotaUs = int64(s.nproc) * time.Second.Microseconds()
	}

	return time.Duration(quotaUs) * time.Microsecond, nil
}

func (s *Statter) cgroupV1CPUUsed() (time.Duration, error) {
	var data []byte
	var err error
	var usageUs int64

	data, err = afero.ReadFile(s.fs, cgroupV1CPUAcctUsage)
	if err != nil {
		return 0, xerrors.Errorf("read %s: %w", cgroupV1CPUAcctUsage, err)
	}

	usageUs, err = strconv.ParseInt(string(bytes.TrimSpace(data)), 10, 64)
	if err != nil {
		return 0, xerrors.Errorf("parse %s: %w", cgroupV1CPUAcctUsage, err)
	}

	return time.Duration(usageUs) * time.Microsecond, nil
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
