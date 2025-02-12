//go:build !linux
// +build !linux

package agentexec

import "golang.org/x/xerrors"

func CLI() error {
	return xerrors.New("agent-exec is only supported on Linux")
}
