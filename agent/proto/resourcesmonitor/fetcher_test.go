package resourcesmonitor_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/clistat"
	"github.com/coder/coder/v2/agent/proto/resourcesmonitor"
	"github.com/coder/coder/v2/coderd/util/ptr"
)

type mockStatter struct {
	isContainerized bool
	containerMemory clistat.Result
	hostMemory      clistat.Result
	disk            map[string]clistat.Result
}

func (s *mockStatter) IsContainerized() (bool, error) {
	return s.isContainerized, nil
}

func (s *mockStatter) ContainerMemory(_ clistat.Prefix) (*clistat.Result, error) {
	return &s.containerMemory, nil
}

func (s *mockStatter) HostMemory(_ clistat.Prefix) (*clistat.Result, error) {
	return &s.hostMemory, nil
}

func (s *mockStatter) Disk(_ clistat.Prefix, path string) (*clistat.Result, error) {
	disk, ok := s.disk[path]
	if !ok {
		return nil, xerrors.New("path not found")
	}
	return &disk, nil
}

func TestFetchMemory(t *testing.T) {
	t.Parallel()

	t.Run("IsContainerized", func(t *testing.T) {
		t.Parallel()

		t.Run("WithMemoryLimit", func(t *testing.T) {
			t.Parallel()

			fetcher, err := resourcesmonitor.NewFetcher(&mockStatter{
				isContainerized: true,
				containerMemory: clistat.Result{
					Used:  10.0,
					Total: ptr.Ref(20.0),
				},
				hostMemory: clistat.Result{
					Used:  20.0,
					Total: ptr.Ref(30.0),
				},
			})
			require.NoError(t, err)

			total, used, err := fetcher.FetchMemory()
			require.NoError(t, err)
			require.Equal(t, int64(10), used)
			require.Equal(t, int64(20), total)
		})

		t.Run("WithoutMemoryLimit", func(t *testing.T) {
			t.Parallel()

			fetcher, err := resourcesmonitor.NewFetcher(&mockStatter{
				isContainerized: true,
				containerMemory: clistat.Result{
					Used:  10.0,
					Total: nil,
				},
				hostMemory: clistat.Result{
					Used:  20.0,
					Total: ptr.Ref(30.0),
				},
			})
			require.NoError(t, err)

			total, used, err := fetcher.FetchMemory()
			require.NoError(t, err)
			require.Equal(t, int64(10), used)
			require.Equal(t, int64(30), total)
		})
	})

	t.Run("IsHost", func(t *testing.T) {
		t.Parallel()

		fetcher, err := resourcesmonitor.NewFetcher(&mockStatter{
			isContainerized: false,
			hostMemory: clistat.Result{
				Used:  20.0,
				Total: ptr.Ref(30.0),
			},
		})
		require.NoError(t, err)

		total, used, err := fetcher.FetchMemory()
		require.NoError(t, err)
		require.Equal(t, int64(20), used)
		require.Equal(t, int64(30), total)
	})
}
