//go:build !linux

package clistat

// ContainerCPU returns the CPU usage of the container cgroup.
// On non-Linux platforms, this always returns nil.
func (s *Statter) ContainerCPU() (*Result, error) {
	return nil, nil
}

// ContainerMemory returns the memory usage of the container cgroup.
// On non-Linux platforms, this always returns nil.
func (s *Statter) ContainerMemory() (*Result, error) {
	return nil, nil
}
