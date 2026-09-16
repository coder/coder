//go:build !linux

package sandbox

import "golang.org/x/xerrors"

// NewRuntime returns an error on systems without the Linux sandbox runtime.
func NewRuntime(RuntimeOptions) (Runtime, error) {
	return nil, xerrors.New("the native sandbox runtime requires a Linux host with containerd and runsc")
}
