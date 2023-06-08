package clistat

import (
	"time"

	"tailscale.com/types/ptr"

	"golang.org/x/xerrors"
)

const ()

// CGroupCPU returns the CPU usage of the container cgroup.
// On non-Linux platforms, this always returns nil.
func (s *Statter) ContainerCPU() (*Result, error) {
	// Firstly, check if we are containerized.
	if ok, err := IsContainerized(); err != nil || !ok {
		return nil, nil
	}

	used1, total1, err := cgroupCPU()
	if err != nil {
		return nil, xerrors.Errorf("get cgroup CPU usage: %w", err)
	}
	<-time.After(s.sampleInterval)
	used2, total2, err := cgroupCPU()
	if err != nil {
		return nil, xerrors.Errorf("get cgroup CPU usage: %w", err)
	}

	return &Result{
		Unit:  "cores",
		Used:  used2 - used1,
		Total: ptr.To(total2 - total1),
	}, nil
}

func cgroupCPU() (used, total float64, err error) {
	if isCGroupV2() {
		return cGroupv2CPU()
	}

	// Fall back to CGroupv1
	return cGroupv1CPU()
}

func isCGroupV2() bool {
	// TODO implement
	return false
}

func cGroupv2CPU() (float64, float64, error) {
	// TODO: implement
	return 0, 0, nil
}

func cGroupv1CPU() (float64, float64, error) {
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
