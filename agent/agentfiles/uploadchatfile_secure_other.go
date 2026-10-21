//go:build !unix

package agentfiles

import (
	"context"
	"io"
)

func (*API) writeUploadExclusiveSecure(context.Context, uploadTarget, string, io.Reader) (finalName, finalPath string, size int64, err error) {
	return "", "", 0, errUploadSecureUnsupported
}
