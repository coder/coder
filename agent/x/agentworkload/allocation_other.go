//go:build !linux

package agentworkload

import (
	"os"

	"golang.org/x/xerrors"
)

func checkOuterAllocation(Config) error { return xerrors.New("workload supervisor requires Linux") }
func privateOwner(os.FileInfo) bool     { return false }
