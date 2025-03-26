package resourcesmonitor

import (
	"github.com/coder/clistat"
	"golang.org/x/xerrors"
)

type Statter interface {
	IsContainerized() (bool, error)
	ContainerMemory(p clistat.Prefix) (*clistat.Result, error)
	HostMemory(p clistat.Prefix) (*clistat.Result, error)
	Disk(p clistat.Prefix, path string) (*clistat.Result, error)
}

type Fetcher interface {
	FetchMemory() (total int64, used int64, err error)
	FetchVolume(volume string) (total int64, used int64, err error)
}

type fetcher struct {
	Statter
	isContainerized bool
}

//nolint:revive
func NewFetcher(f Statter) (*fetcher, error) {
	isContainerized, err := f.IsContainerized()
	if err != nil {
		return nil, xerrors.Errorf("check is containerized: %w", err)
	}

	return &fetcher{f, isContainerized}, nil
}

func (f *fetcher) FetchMemory() (total int64, used int64, err error) {
	var mem *clistat.Result

	if f.isContainerized {
		mem, err = f.ContainerMemory(clistat.PrefixDefault)
		if err != nil {
			return 0, 0, xerrors.Errorf("fetch container memory: %w", err)
		}

		// A container might not have a memory limit set. If this
		// happens we want to fallback to querying the host's memory
		// to know what the total memory is on the host.
		if mem.Total == nil {
			hostMem, err := f.HostMemory(clistat.PrefixDefault)
			if err != nil {
				return 0, 0, xerrors.Errorf("fetch host memory: %w", err)
			}

			mem.Total = hostMem.Total
		}
	} else {
		mem, err = f.HostMemory(clistat.PrefixDefault)
		if err != nil {
			return 0, 0, xerrors.Errorf("fetch host memory: %w", err)
		}
	}

	if mem.Total == nil {
		return 0, 0, xerrors.New("memory total is nil - can not fetch memory")
	}

	return int64(*mem.Total), int64(mem.Used), nil
}

func (f *fetcher) FetchVolume(volume string) (total int64, used int64, err error) {
	vol, err := f.Disk(clistat.PrefixDefault, volume)
	if err != nil {
		return 0, 0, err
	}

	if vol.Total == nil {
		return 0, 0, xerrors.New("volume total is nil - can not fetch volume")
	}

	return int64(*vol.Total), int64(vol.Used), nil
}
