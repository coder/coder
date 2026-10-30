//go:build !linux && !darwin

package agentpreview

import (
	"errors"
	"os"
)

func openPreviewRoot(_ string) (*os.Root, error) {
	return nil, errors.ErrUnsupported
}

func openPreviewSubroot(_ *os.Root, _ string) (*os.Root, error) {
	return nil, errors.ErrUnsupported
}

func openPreviewFile(_ *os.Root, _ string) (*os.File, error) {
	// Fail closed until no-follow, nonblocking opens are supported here.
	return nil, errors.ErrUnsupported
}

func isHardLinked(_ os.FileInfo) bool {
	return true
}
