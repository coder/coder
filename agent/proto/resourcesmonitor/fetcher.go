package resourcesmonitor

import (
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/clistat"
)

type ResourcesFetcher interface {
	FetchResourceMonitoredMemory() (total int64, used int64, err error)
	FetchResourceMonitoredVolume(volume string) (total int64, used int64, err error)
}

type resourcesFetcher struct {
	fetcher *clistat.Statter
}

//nolint:revive
func NewResourcesFetcher(fetcher *clistat.Statter) *resourcesFetcher {
	return &resourcesFetcher{
		fetcher: fetcher,
	}
}

func (f *resourcesFetcher) FetchResourceMonitoredMemory() (total int64, used int64, err error) {
	mem, err := f.fetcher.HostMemory(clistat.PrefixMebi)
	if err != nil {
		return 0, 0, err
	}

	var memTotal, memUsed int64
	if mem.Total == nil {
		return 0, 0, xerrors.New("memory total is nil - can not fetch memory")
	}

	memTotal = f.bytesToMegabytes(int64(*mem.Total))
	memUsed = f.bytesToMegabytes(int64(mem.Used))

	return memTotal, memUsed, nil
}

func (f *resourcesFetcher) FetchResourceMonitoredVolume(volume string) (total int64, used int64, err error) {
	vol, err := f.fetcher.Disk(clistat.PrefixMebi, volume)
	if err != nil {
		return 0, 0, err
	}

	var volTotal, volUsed int64
	if vol.Total == nil {
		return 0, 0, xerrors.New("volume total is nil - can not fetch volume")
	}

	volTotal = f.bytesToMegabytes(int64(*vol.Total))
	volUsed = f.bytesToMegabytes(int64(vol.Used))

	return volTotal, volUsed, nil
}

func (*resourcesFetcher) bytesToMegabytes(bytes int64) int64 {
	return bytes / (1024 * 1024)
}
