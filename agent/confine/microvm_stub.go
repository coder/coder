//go:build !linux

package confine

import (
	"context"
	"errors"
)

// StartEmbeddedMicroVM reports that the embedded microVM runtime requires Linux.
func StartEmbeddedMicroVM(context.Context, MicroVMOptions) (*EmbeddedSandbox, error) {
	return nil, errors.New("embedded microVMs are supported only on Linux")
}
