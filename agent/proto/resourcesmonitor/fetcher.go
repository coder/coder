package resourcesmonitor

import (
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/clistat"
)

type Fetcher interface {
	FetchMemory() (total int64, used int64, err error)
	FetchVolume(volume string) (total int64, used int64, err error)
}

type fetcher struct {
	*clistat.Statter
}

//nolint:revive
func NewFetcher(f *clistat.Statter) *fetcher {
	return &fetcher{
		f,
	}
}

func (f *fetcher) FetchMemory() (total int64, used int64, err error) {
	mem, err := f.HostMemory(clistat.PrefixDefault)
	if err != nil {
		return 0, 0, xerrors.Errorf("failed to fetch memory: %w", err)
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
