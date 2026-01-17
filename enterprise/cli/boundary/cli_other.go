//go:build !linux

//nolint:revive,gocritic,errname,unconvert

package boundary

import (
	"runtime"

	"golang.org/x/xerrors"

	"github.com/coder/serpent"
)

// BaseCommand returns the boundary serpent command. On non-Linux platforms,
// boundary is not supported and returns an error.
func BaseCommand(_ string) *serpent.Command {
	return &serpent.Command{
		Use:   "boundary",
		Short: "Network isolation tool for monitoring and restricting HTTP/HTTPS requests",
		Long:  `boundary creates an isolated network environment for target processes, intercepting HTTP/HTTPS traffic through a transparent proxy that enforces user-defined allow rules.`,
		Handler: func(_ *serpent.Invocation) error {
			return xerrors.Errorf("boundary is only supported on Linux (current OS: %s)", runtime.GOOS)
		},
	}
}
