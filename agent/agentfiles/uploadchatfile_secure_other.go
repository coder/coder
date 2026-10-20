//go:build !unix

package agentfiles

import "io"

func (api *API) writeUploadExclusiveSecure(target uploadTarget, name string, r io.Reader) (finalName, finalPath string, size int64, err error) {
	return "", "", 0, errUploadSecureUnsupported
}
