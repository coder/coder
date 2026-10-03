//go:build !windows

package ptytest

import "github.com/coder/coder/v2/pty"

func newTestPTY(opts ...pty.Option) (pty.PTY, error) {
	return pty.New(opts...)
}
