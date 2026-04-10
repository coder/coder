//go:build linux

package terraform

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/procfs"
)

// linuxCommMaxLen is the kernel's limit for the comm field in
// /proc/<pid>/stat. Names longer than this are silently truncated,
// which affects provider identification.
const linuxCommMaxLen = 15

const providerPrefix = "terraform-provider-"

// ProviderResourceUsage holds peak memory and cumulative CPU for a
// single Terraform provider process (or group of processes with the
// same provider name).
type ProviderResourceUsage struct {
	PeakRSSBytes   uint64
	CPUTimeSeconds float64 // user + system
}

// ProcessSample is the final summary returned when the sampler stops.
type ProcessSample struct {
	Providers map[string]ProviderResourceUsage
}

// procSampler periodically reads /proc to collect resource usage for
// a terraform process and its direct children (provider plugins).
// Processes are identified by parent PID rather than process group,
// so this works regardless of whether Setpgid is used.
type procSampler struct {
	pid      int
	interval time.Duration
	fs       procfs.FS

	mu      sync.Mutex
	current map[string]ProviderResourceUsage

	done chan struct{}
}

func newProcSampler(pid int, interval time.Duration) *procSampler {
	// Default to the real /proc mount. Tests can override s.fs after
	// construction if needed, but in practice we always read the
	// host procfs.
	fs, err := procfs.NewDefaultFS()
	if err != nil {
		// This should never fail on Linux where /proc is always
		// mounted. Fall back to the standard path.
		fs, _ = procfs.NewFS("/proc")
	}
	return &procSampler{
		pid:      pid,
		interval: interval,
		fs:       fs,
		current:  make(map[string]ProviderResourceUsage),
		done:     make(chan struct{}),
	}
}

// Start begins periodic sampling in a background goroutine. The
// goroutine exits when ctx is canceled or Stop is called.
func (s *procSampler) Start(ctx context.Context) {
	go func() {
		defer close(s.done)
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()

		// Take an immediate first sample so callers that stop
		// quickly still get data.
		s.sample()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.sample()
			}
		}
	}()
}

// Stop cancels sampling and returns the final accumulated summary.
// It is safe to call Stop without calling Start; it returns an
// empty summary.
func (s *procSampler) Stop() ProcessSample {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := ProcessSample{
		Providers: make(map[string]ProviderResourceUsage, len(s.current)),
	}
	for k, v := range s.current {
		result.Providers[k] = v
	}
	return result
}

// sample performs a single pass over all processes, filtering to the
// target PID and its direct children (PPID match). This captures the
// terraform binary itself plus its provider plugin processes without
// requiring process group isolation.
func (s *procSampler) sample() {
	procs, err := s.fs.AllProcs()
	if err != nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, proc := range procs {
		stat, err := proc.NewStat()
		if err != nil {
			// Process vanished between listing and stat; this is
			// normal during Terraform runs.
			continue
		}

		// Match the terraform process itself or any of its direct
		// children (provider plugins).
		if proc.PID != s.pid && stat.PPID != s.pid {
			continue
		}

		name := resolveProviderName(proc, stat.Comm)

		// VmHWM is the kernel-tracked peak RSS (high-water mark)
		// for this process. It is monotonically non-decreasing,
		// so the last successfully read value is always the true
		// peak — no manual max-tracking required. We read it
		// from /proc/<pid>/status rather than computing RSS from
		// /proc/<pid>/stat, because stat only gives instantaneous
		// RSS and we would miss spikes between samples.
		var peakRSSBytes uint64
		if status, err := proc.NewStatus(); err == nil {
			peakRSSBytes = status.VmHWM * 1024 // VmHWM is in kB.
		}

		// CPU time is also monotonically non-decreasing, so the
		// last reading is the cumulative total.
		cpuTime := stat.CPUTime()

		existing := s.current[name]
		existing.PeakRSSBytes = peakRSSBytes
		existing.CPUTimeSeconds = cpuTime
		s.current[name] = existing
	}
}

// resolveProviderName determines the short provider name from a
// process. It first tries the comm field from /proc/<pid>/stat,
// but falls back to /proc/<pid>/cmdline when comm is truncated
// to the kernel's 15-character limit.
func resolveProviderName(proc procfs.Proc, comm string) string {
	if len(comm) >= linuxCommMaxLen {
		// The comm field is likely truncated. Read the full
		// command line to get the real binary name.
		if cmdline, err := proc.CmdLine(); err == nil && len(cmdline) > 0 {
			return extractProviderName(cmdline[0])
		}
	}
	return extractProviderName(comm)
}

// extractProviderName derives a short provider name from a binary
// name or path. It handles the full form
// ("terraform-provider-aws_v5.0.0_x5"), truncated comm values, and
// bare "terraform".
func extractProviderName(comm string) string {
	// Handle paths: use only the base name.
	if idx := strings.LastIndex(comm, "/"); idx >= 0 {
		comm = comm[idx+1:]
	}

	after, found := strings.CutPrefix(comm, providerPrefix)
	if !found {
		return comm
	}

	// Strip version suffix (everything from "_v" onward).
	if idx := strings.Index(after, "_v"); idx >= 0 {
		after = after[:idx]
	}

	return after
}
