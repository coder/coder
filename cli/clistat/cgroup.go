package clistat

import (
	"bufio"
	"bytes"
	"strconv"
	"strings"

	"github.com/spf13/afero"
	"golang.org/x/xerrors"
	"tailscale.com/types/ptr"
)

// Paths for CGroupV1.
// Ref: https://www.kernel.org/doc/Documentation/cgroup-v1/cpuacct.txt
const (
	// CPU usage of all tasks in cgroup in nanoseconds.
	cgroupV1CPUAcctUsage = "/sys/fs/cgroup/cpu/cpuacct.usage"
	// Alternate path
	cgroupV1CPUAcctUsageAlt = "/sys/fs/cgroup/cpu,cpuacct/cpuacct.usage"
	// CFS quota and period for cgroup in MICROseconds
	cgroupV1CFSQuotaUs  = "/sys/fs/cgroup/cpu,cpuacct/cpu.cfs_quota_us"
	cgroupV1CFSPeriodUs = "/sys/fs/cgroup/cpu,cpuacct/cpu.cfs_period_us"
	// Maximum memory usable by cgroup in bytes
	cgroupV1MemoryMaxUsageBytes = "/sys/fs/cgroup/memory/memory.max_usage_in_bytes"
	// Current memory usage of cgroup in bytes
	cgroupV1MemoryUsageBytes = "/sys/fs/cgroup/memory/memory.usage_in_bytes"
	// Other memory stats - we are interested in total_inactive_file
	cgroupV1MemoryStat = "/sys/fs/cgroup/memory/memory.stat"
)

// Paths for CGroupV2.
// Ref: https://docs.kernel.org/admin-guide/cgroup-v2.html
const (
	// Contains quota and period in microseconds separated by a space.
	cgroupV2CPUMax = "/sys/fs/cgroup/cpu.max"
	// Contains current CPU usage under usage_usec
	cgroupV2CPUStat = "/sys/fs/cgroup/cpu.stat"
	// Contains current cgroup memory usage in bytes.
	cgroupV2MemoryUsageBytes = "/sys/fs/cgroup/memory.current"
	// Contains max cgroup memory usage in bytes.
	cgroupV2MemoryMaxBytes = "/sys/fs/cgroup/memory.max"
	// Other memory stats - we are interested in total_inactive_file
	cgroupV2MemoryStat = "/sys/fs/cgroup/memory.stat"
)

// ContainerCPU returns the CPU usage of the container cgroup.
// This is calculated as difference of two samples of the
// CPU usage of the container cgroup.
// The total is read from the relevant path in /sys/fs/cgroup.
// If there is no limit set, the total is assumed to be the
// number of host cores multiplied by the CFS period.
// If the system is not containerized, this always returns nil.
func (s *Statter) ContainerCPU() (*Result, error) {
	// Firstly, check if we are containerized.
	if ok, err := IsContainerized(s.fs); err != nil || !ok {
		return nil, nil //nolint: nilnil
	}

	total, err := s.cGroupCPUTotal()
	if err != nil {
		return nil, xerrors.Errorf("get total cpu: %w", err)
	}

	used1, err := s.cGroupCPUUsed()
	if err != nil {
		return nil, xerrors.Errorf("get cgroup CPU usage: %w", err)
	}

	// The measurements in /sys/fs/cgroup are counters.
	// We need to wait for a bit to get a difference.
	// Note that someone could reset the counter in the meantime.
	// We can't do anything about that.
	s.wait(s.sampleInterval)

	used2, err := s.cGroupCPUUsed()
	if err != nil {
		return nil, xerrors.Errorf("get cgroup CPU usage: %w", err)
	}

	if used2 < used1 {
		// Someone reset the counter. Best we can do is count from zero.
		used1 = 0
	}

	r := &Result{
		Unit:   "cores",
		Used:   used2 - used1,
		Total:  ptr.To(total),
		Prefix: PrefixDefault,
	}
	return r, nil
}

func (s *Statter) cGroupCPUTotal() (used float64, err error) {
	if s.isCGroupV2() {
		return s.cGroupV2CPUTotal()
	}

	// Fall back to CGroupv1
	return s.cGroupV1CPUTotal()
}

func (s *Statter) cGroupCPUUsed() (used float64, err error) {
	if s.isCGroupV2() {
		return s.cGroupV2CPUUsed()
	}

	return s.cGroupV1CPUUsed()
}

func (s *Statter) isCGroupV2() bool {
	// Check for the presence of /sys/fs/cgroup/cpu.max
	_, err := s.fs.Stat(cgroupV2CPUMax)
	return err == nil
}

func (s *Statter) cGroupV2CPUUsed() (used float64, err error) {
	usageUs, err := readInt64Prefix(s.fs, cgroupV2CPUStat, "usage_usec")
	if err != nil {
		return 0, xerrors.Errorf("get cgroupv2 cpu used: %w", err)
	}
	periodUs, err := readInt64SepIdx(s.fs, cgroupV2CPUMax, " ", 1)
	if err != nil {
		return 0, xerrors.Errorf("get cpu period: %w", err)
	}

	return float64(usageUs) / float64(periodUs), nil
}

func (s *Statter) cGroupV2CPUTotal() (total float64, err error) {
	var quotaUs, periodUs int64
	periodUs, err = readInt64SepIdx(s.fs, cgroupV2CPUMax, " ", 1)
	if err != nil {
		return 0, xerrors.Errorf("get cpu period: %w", err)
	}

	quotaUs, err = readInt64SepIdx(s.fs, cgroupV2CPUMax, " ", 0)
	if err != nil {
		// Fall back to number of cores
		quotaUs = int64(s.nproc) * periodUs
	}

	return float64(quotaUs) / float64(periodUs), nil
}

func (s *Statter) cGroupV1CPUTotal() (float64, error) {
	periodUs, err := readInt64(s.fs, cgroupV1CFSPeriodUs)
	if err != nil {
		return 0, xerrors.Errorf("read cpu period: %w", err)
	}

	quotaUs, err := readInt64(s.fs, cgroupV1CFSQuotaUs)
	if err != nil {
		return 0, xerrors.Errorf("read cpu quota: %w", err)
	}

	if quotaUs < 0 {
		// Fall back to the number of cores
		quotaUs = int64(s.nproc) * periodUs
	}

	return float64(quotaUs) / float64(periodUs), nil
}

func (s *Statter) cGroupV1CPUUsed() (float64, error) {
	usageNs, err := readInt64(s.fs, cgroupV1CPUAcctUsage)
	if err != nil {
		// try alternate path
		usageNs, err = readInt64(s.fs, cgroupV1CPUAcctUsageAlt)
		if err != nil {
			return 0, xerrors.Errorf("read cpu used: %w", err)
		}
	}

	// usage is in ns, convert to us
	usageNs /= 1000
	periodUs, err := readInt64(s.fs, cgroupV1CFSPeriodUs)
	if err != nil {
		return 0, xerrors.Errorf("get cpu period: %w", err)
	}

	return float64(usageNs) / float64(periodUs), nil
}

// ContainerMemory returns the memory usage of the container cgroup.
// If the system is not containerized, this always returns nil.
func (s *Statter) ContainerMemory(p Prefix) (*Result, error) {
	if ok, err := IsContainerized(s.fs); err != nil || !ok {
		return nil, nil //nolint:nilnil
	}

	if s.isCGroupV2() {
		return s.cGroupV2Memory(p)
	}

	// Fall back to CGroupv1
	return s.cGroupV1Memory(p)
}

func (s *Statter) cGroupV2Memory(p Prefix) (*Result, error) {
	maxUsageBytes, err := readInt64(s.fs, cgroupV2MemoryMaxBytes)
	if err != nil {
		return nil, xerrors.Errorf("read memory total: %w", err)
	}

	currUsageBytes, err := readInt64(s.fs, cgroupV2MemoryUsageBytes)
	if err != nil {
		return nil, xerrors.Errorf("read memory usage: %w", err)
	}

	inactiveFileBytes, err := readInt64Prefix(s.fs, cgroupV2MemoryStat, "inactive_file")
	if err != nil {
		return nil, xerrors.Errorf("read memory stats: %w", err)
	}

	return &Result{
		Total:  ptr.To(float64(maxUsageBytes)),
		Used:   float64(currUsageBytes - inactiveFileBytes),
		Unit:   "B",
		Prefix: p,
	}, nil
}

func (s *Statter) cGroupV1Memory(p Prefix) (*Result, error) {
	maxUsageBytes, err := readInt64(s.fs, cgroupV1MemoryMaxUsageBytes)
	if err != nil {
		return nil, xerrors.Errorf("read memory total: %w", err)
	}

	// need a space after total_rss so we don't hit something else
	usageBytes, err := readInt64(s.fs, cgroupV1MemoryUsageBytes)
	if err != nil {
		return nil, xerrors.Errorf("read memory usage: %w", err)
	}

	totalInactiveFileBytes, err := readInt64Prefix(s.fs, cgroupV1MemoryStat, "total_inactive_file")
	if err != nil {
		return nil, xerrors.Errorf("read memory stats: %w", err)
	}

	// Total memory used is usage - total_inactive_file
	return &Result{
		Total:  ptr.To(float64(maxUsageBytes)),
		Used:   float64(usageBytes - totalInactiveFileBytes),
		Unit:   "B",
		Prefix: p,
	}, nil
}

// read an int64 value from path
func readInt64(fs afero.Fs, path string) (int64, error) {
	data, err := afero.ReadFile(fs, path)
	if err != nil {
		return 0, xerrors.Errorf("read %s: %w", path, err)
	}

	val, err := strconv.ParseInt(string(bytes.TrimSpace(data)), 10, 64)
	if err != nil {
		return 0, xerrors.Errorf("parse %s: %w", path, err)
	}

	return val, nil
}

// read an int64 value from path at field idx separated by sep
func readInt64SepIdx(fs afero.Fs, path, sep string, idx int) (int64, error) {
	data, err := afero.ReadFile(fs, path)
	if err != nil {
		return 0, xerrors.Errorf("read %s: %w", path, err)
	}

	parts := strings.Split(string(data), sep)
	if len(parts) < idx {
		return 0, xerrors.Errorf("expected line %q to have at least %d parts", string(data), idx+1)
	}

	val, err := strconv.ParseInt(strings.TrimSpace(parts[idx]), 10, 64)
	if err != nil {
		return 0, xerrors.Errorf("parse %s: %w", path, err)
	}

	return val, nil
}

// read the first int64 value from path prefixed with prefix
func readInt64Prefix(fs afero.Fs, path, prefix string) (int64, error) {
	data, err := afero.ReadFile(fs, path)
	if err != nil {
		return 0, xerrors.Errorf("read %s: %w", path, err)
	}

	scn := bufio.NewScanner(bytes.NewReader(data))
	for scn.Scan() {
		line := scn.Text()
		if !strings.HasPrefix(line, prefix) {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) != 2 {
			return 0, xerrors.Errorf("parse %s: expected two fields but got %s", path, line)
		}

		val, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
		if err != nil {
			return 0, xerrors.Errorf("parse %s: %w", path, err)
		}

		return val, nil
	}

	return 0, xerrors.Errorf("parse %s: did not find line with prefix %s", path, prefix)
}
