package clistat

import (
	"testing"

	"tailscale.com/types/ptr"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResultString(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		Expected string
		Result   Result
	}{
		{
			Expected: "1.2/5.7 quatloos",
			Result:   Result{Used: 1.234, Total: ptr.To(5.678), Unit: "quatloos"},
		},
		{
			Expected: "0.0/0.0 HP",
			Result:   Result{Used: 0.0, Total: ptr.To(0.0), Unit: "HP"},
		},
		{
			Expected: "123.0 seconds",
			Result:   Result{Used: 123.01, Total: nil, Unit: "seconds"},
		},
		{
			Expected: "12.3",
			Result:   Result{Used: 12.34, Total: nil, Unit: ""},
		},
	} {
		assert.Equal(t, tt.Expected, tt.Result.String())
	}
}

// TestStatter does not test the actual values returned by the Statter.
func TestStatter(t *testing.T) {
	t.Parallel()

	s, err := New()
	require.NoError(t, err)

	t.Run("HostCPU", func(t *testing.T) {
		t.Parallel()
		cpu, err := s.HostCPU()
		require.NoError(t, err)
		assert.NotZero(t, cpu.Used)
		assert.NotZero(t, cpu.Total)
		assert.NotZero(t, cpu.Unit)
	})

	t.Run("HostMemory", func(t *testing.T) {
		t.Parallel()
		mem, err := s.HostMemory()
		require.NoError(t, err)
		assert.NotZero(t, mem.Used)
		assert.NotZero(t, mem.Total)
		assert.NotZero(t, mem.Unit)
	})

	t.Run("HostDisk", func(t *testing.T) {
		t.Parallel()
		disk, err := s.Disk("")
		require.NoError(t, err)
		assert.NotZero(t, disk.Used)
		assert.NotZero(t, disk.Total)
		assert.NotZero(t, disk.Unit)
	})

	t.Run("Uptime", func(t *testing.T) {
		t.Parallel()
		uptime, err := s.Uptime()
		require.NoError(t, err)
		assert.NotZero(t, uptime.Used)
		assert.Zero(t, uptime.Total)
		assert.NotZero(t, uptime.Unit)
	})

	t.Run("ContainerCPU", func(t *testing.T) {
		t.Parallel()
		if ok, err := IsContainerized(); err != nil || !ok {
			t.Skip("not running in container")
		}
		cpu, err := s.ContainerCPU()
		require.NoError(t, err)
		assert.NotNil(t, cpu)
		assert.NotZero(t, cpu.Used)
		assert.NotZero(t, cpu.Total)
		assert.NotZero(t, cpu.Unit)
	})

	t.Run("ContainerMemory", func(t *testing.T) {
		t.Parallel()
		if ok, err := IsContainerized(); err != nil || !ok {
			t.Skip("not running in container")
		}
		mem, err := s.ContainerMemory()
		require.NoError(t, err)
		assert.NotNil(t, mem)
		assert.NotZero(t, mem.Used)
		assert.NotZero(t, mem.Total)
		assert.NotZero(t, mem.Unit)
	})
}
