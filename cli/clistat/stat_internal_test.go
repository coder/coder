package clistat

import (
	"testing"

	"tailscale.com/types/ptr"

	"github.com/spf13/afero"
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

func TestStatter(t *testing.T) {
	t.Parallel()

	// We cannot make many assertions about the data we get back
	// for host-specific measurements because these tests could
	// and should run successfully on any OS.
	// The best we can do is assert that it is non-zero.
	t.Run("HostOnly", func(t *testing.T) {
		t.Parallel()
		fs := initFS(t, fsHostOnly)
		s, err := New(WithFS(fs))
		require.NoError(t, err)
		t.Run("HostCPU", func(t *testing.T) {
			t.Parallel()
			cpu, err := s.HostCPU()
			require.NoError(t, err)
			assert.NotZero(t, cpu.Used)
			assert.NotZero(t, cpu.Total)
			assert.Equal(t, "cores", cpu.Unit)
		})

		t.Run("HostMemory", func(t *testing.T) {
			t.Parallel()
			mem, err := s.HostMemory()
			require.NoError(t, err)
			assert.NotZero(t, mem.Used)
			assert.NotZero(t, mem.Total)
			assert.Equal(t, "GB", mem.Unit)
		})

		t.Run("HostDisk", func(t *testing.T) {
			t.Parallel()
			disk, err := s.Disk("") // default to home dir
			require.NoError(t, err)
			assert.NotZero(t, disk.Used)
			assert.NotZero(t, disk.Total)
			assert.NotZero(t, disk.Unit)
		})
	})

	t.Run("CGroupV1", func(t *testing.T) {
		t.Parallel()
		t.Skip("not implemented")

		t.Run("Limit", func(t *testing.T) {
			t.Parallel()
		})

		t.Run("NoLimit", func(t *testing.T) {
			t.Parallel()
		})
	})

	t.Run("CGroupV2", func(t *testing.T) {
		t.Parallel()
		t.Run("Limit", func(t *testing.T) {
			fs := initFS(t, fsContainerCgroupV2)
			s, err := New(WithFS(fs))
			require.NoError(t, err)
			// We can make assertions about the below because these all read
			// data from known file paths, which we can control.
			t.Run("ContainerCPU", func(t *testing.T) {
				t.Parallel()
				cpu, err := s.ContainerCPU()
				require.NoError(t, err)
				assert.NotNil(t, cpu)
				// This value does not change in between tests so it is zero.
				assert.Zero(t, cpu.Used)
				// Eve
				require.NotNil(t, cpu.Total)
				assert.Equal(t, 2.5, *cpu.Total)
				assert.Equal(t, "cores", cpu.Unit)
			})

			t.Run("ContainerMemory", func(t *testing.T) {
				t.Parallel()
				t.Skip("not implemented")
				mem, err := s.ContainerMemory()
				require.NoError(t, err)
				assert.NotNil(t, mem)
				assert.NotZero(t, mem.Used)
				assert.NotZero(t, mem.Total)
				assert.Equal(t, "GB", mem.Unit)
			})
		})

		t.Run("NoLimit", func(t *testing.T) {
			fs := initFS(t, fsContainerCgroupV2)
			s, err := New(WithFS(fs), func(s *Statter) {
				s.nproc = 2
			})
			require.NoError(t, err)
			// We can make assertions about the below because these all read
			// data from known file paths, which we can control.
			t.Run("ContainerCPU", func(t *testing.T) {
				t.Parallel()
				cpu, err := s.ContainerCPU()
				require.NoError(t, err)
				assert.NotNil(t, cpu)
				// This value does not change in between tests so it is zero.
				assert.Zero(t, cpu.Used)
				// Eve
				require.NotNil(t, cpu.Total)
				assert.Equal(t, 2.5, *cpu.Total)
				assert.Equal(t, "cores", cpu.Unit)
			})

			t.Run("ContainerMemory", func(t *testing.T) {
				t.Parallel()
				t.Skip("not implemented")
				mem, err := s.ContainerMemory()
				require.NoError(t, err)
				assert.NotNil(t, mem)
				assert.NotZero(t, mem.Used)
				assert.NotZero(t, mem.Total)
				assert.Equal(t, "GB", mem.Unit)
			})
		})
	})
}

func TestIsContainerized(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		Name     string
		FS       map[string]string
		Expected bool
		Error    string
	}{
		{
			Name:     "Empty",
			FS:       map[string]string{},
			Expected: false,
			Error:    "",
		},
		{
			Name:     "BareMetal",
			FS:       fsHostOnly,
			Expected: false,
			Error:    "",
		},
		{
			Name:     "Docker",
			FS:       fsContainerCgroupV1,
			Expected: true,
			Error:    "",
		},
		{
			Name:     "Sysbox",
			FS:       fsContainerSysbox,
			Expected: true,
			Error:    "",
		},
	} {
		tt := tt
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()
			fs := initFS(t, tt.FS)
			actual, err := IsContainerized(fs)
			if tt.Error == "" {
				assert.NoError(t, err)
				assert.Equal(t, tt.Expected, actual)
			} else {
				assert.ErrorContains(t, err, tt.Error)
				assert.False(t, actual)
			}
		})
	}
}

func initFS(t testing.TB, m map[string]string) afero.Fs {
	t.Helper()
	fs := afero.NewMemMapFs()
	for k, v := range m {
		require.NoError(t, afero.WriteFile(fs, k, []byte(v+"\n"), 0o600))
	}
	return fs
}

var (
	fsHostOnly = map[string]string{
		procOneCgroup: "0::/",
		procMounts:    "/dev/sda1 / ext4 rw,relatime 0 0",
	}
	fsContainerCgroupV2 = map[string]string{
		procOneCgroup: "0::/docker/aa86ac98959eeedeae0ecb6e0c9ddd8ae8b97a9d0fdccccf7ea7a474f4e0bb1f",
		procMounts: `overlay / overlay rw,relatime,lowerdir=/some/path:/some/path,upperdir=/some/path:/some/path,workdir=/some/path:/some/path 0 0
proc /proc/sys proc ro,nosuid,nodev,noexec,relatime 0 0`,
		cgroupV2CPUMax:  "250000 100000",
		cgroupV2CPUStat: "usage_usec 1000000",
	}
	fsContainerCgroupV1 = map[string]string{
		procOneCgroup: "0::/docker/aa86ac98959eeedeae0ecb6e0c9ddd8ae8b97a9d0fdccccf7ea7a474f4e0bb1f",
		procMounts: `overlay / overlay rw,relatime,lowerdir=/some/path:/some/path,upperdir=/some/path:/some/path,workdir=/some/path:/some/path 0 0
proc /proc/sys proc ro,nosuid,nodev,noexec,relatime 0 0`,
	}
	fsContainerCgroupV2NoLimit = map[string]string{
		procOneCgroup: "0::/docker/aa86ac98959eeedeae0ecb6e0c9ddd8ae8b97a9d0fdccccf7ea7a474f4e0bb1f",
		procMounts: `overlay / overlay rw,relatime,lowerdir=/some/path:/some/path,upperdir=/some/path:/some/path,workdir=/some/path:/some/path 0 0
proc /proc/sys proc ro,nosuid,nodev,noexec,relatime 0 0`,
		cgroupV2CPUMax:  "max 100000",
		cgroupV2CPUStat: "usage_usec 1000000",
	}
	fsContainerSysbox = map[string]string{
		procOneCgroup: "0::/docker/aa86ac98959eeedeae0ecb6e0c9ddd8ae8b97a9d0fdccccf7ea7a474f4e0bb1f",
		procMounts: `overlay / overlay rw,relatime,lowerdir=/some/path:/some/path,upperdir=/some/path:/some/path,workdir=/some/path:/some/path 0 0
sysboxfs /proc/sys proc ro,nosuid,nodev,noexec,relatime 0 0`,
		cgroupV2CPUMax:  "250000 100000",
		cgroupV2CPUStat: "usage_usec 1000000",
	}
)
