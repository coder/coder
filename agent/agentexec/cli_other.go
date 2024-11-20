//go:build !linux
// +build !linux

package agentexec

import "golang.org/x/xerrors"

func CLI(args []string, environ []string) error {
	return xerrors.Errorf("agent-exec is only supported on Linux")
}
