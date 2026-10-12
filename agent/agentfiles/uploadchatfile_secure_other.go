//go:build !unix

package agentfiles

import "io"

func (api *API) writeUploadExclusiveSecure(homeDir, chatID, dir, name string, r io.Reader) (finalName, finalPath string, size int64, err error) {
	return "", "", 0, errUploadSecureUnsupported
}
