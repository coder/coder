//go:build !windows

package agentfiles

func (api *API) renameUpload(from, to string, _ uploadPublishMode) error {
	return api.filesystem.Rename(from, to)
}
