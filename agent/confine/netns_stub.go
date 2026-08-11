//go:build !linux

package confine

import (
	"context"

	"golang.org/x/xerrors"
)

const NetNSSubnet = "100.115.92.0/30"

type networkNamespace struct {
	hostIP string
}

func newNetworkNamespace(context.Context) (*networkNamespace, error) {
	return nil, xerrors.New("network namespace confinement requires Linux")
}

func (*networkNamespace) execArgs(args []string) []string {
	return args
}

func (*networkNamespace) Close() error {
	return nil
}
