// Package touchid provides Secure Enclave ECDSA P-256 key
// operations for macOS. On darwin with CGO enabled, it uses the
// Security and LocalAuthentication frameworks directly. On other
// platforms, all functions return ErrNotAvailable.
//
// The private key never leaves the Secure Enclave hardware.
// Signing operations trigger a Touch ID biometric prompt.
package touchid

import "errors"

var (
	// ErrNotAvailable is returned when the platform does not
	// support Secure Enclave operations (non-macOS or no CGO).
	ErrNotAvailable = errors.New("touchid: not available on this platform")
	// ErrUserCancelled is returned when the user cancels the
	// Touch ID biometric prompt.
	ErrUserCancelled = errors.New("touchid: user cancelled biometric")
)
