package resourcesmonitor_test

type resourcesFetcherMock struct {
	fetchResourcesMonitoredMemoryFunc func() (total int64, used int64, err error)
	fetchResourcesMonitoredVolumeFunc func(volume string) (total int64, used int64, err error)
}

func (r *resourcesFetcherMock) FetchResourceMonitoredMemory() (total int64, used int64, err error) {
	return r.fetchResourcesMonitoredMemoryFunc()
}

func (r *resourcesFetcherMock) FetchResourceMonitoredVolume(volume string) (total int64, used int64, err error) {
	return r.fetchResourcesMonitoredVolumeFunc(volume)
}
