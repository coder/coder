//go:build !linux

package terraform

import (
	"context"
	"time"
)

// ProviderResourceUsage holds peak memory and cumulative CPU for a
// single Terraform provider process.
type ProviderResourceUsage struct {
	PeakRSSBytes   uint64
	CPUTimeSeconds float64
}

// ProcessSample is the final summary returned when the sampler stops.
type ProcessSample struct {
	Providers map[string]ProviderResourceUsage
}

// procSampler is a no-op on non-Linux platforms. /proc sampling is
// only meaningful on Linux where procfs is available.
type procSampler struct{}

func newProcSampler(_ int, _ time.Duration) *procSampler { //nolint:revive // pid param unused on non-Linux.
	return &procSampler{}
}

func (s *procSampler) Start(_ context.Context) {}

func (s *procSampler) Stop() ProcessSample {
	return ProcessSample{}
}
