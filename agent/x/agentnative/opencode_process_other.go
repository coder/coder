//go:build !linux

package agentnative

import (
	"os"
	"os/exec"
)

// The POC requires Linux proc and pidfd identity proofs. Other platforms must
// not fall back to signaling an unverified PID or managed-service discovery.
func openCodePrivateDirectory(string, openCodeDirectoryMode) error { return ErrUnsupported }
func openCodePrivateFile(string, int) (*os.File, error)            { return nil, ErrUnsupported }
func openCodeProcessDomain() (string, error)                       { return "", ErrUnsupported }
func openCodeProcessIdentity(int) (*openCodeProcess, error)        { return nil, ErrUnsupported }
func openCodePrepareCommand(*exec.Cmd) error                       { return ErrUnsupported }
func openCodeSignal(openCodeDescriptor) error                      { return ErrUnsupported }
func openCodeLockFile(*os.File) error                              { return ErrUnsupported }
func openCodeUnlockFile(*os.File) error                            { return ErrUnsupported }
