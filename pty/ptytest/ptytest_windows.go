//go:build windows
// +build windows

package ptytest

import (
	"testing"

	"github.com/coder/coder/pty"
)

func initializePTY(t *testing.T, pty pty.PTY) {
	// Nothing to initialize on Windows!
}
