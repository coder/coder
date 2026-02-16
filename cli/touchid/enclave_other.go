//go:build !darwin || !cgo

package touchid

// IsAvailable always returns false on non-darwin platforms.
func IsAvailable() bool { return false }

// GenerateKey is not supported on this platform.
func GenerateKey() (string, string, error) { return "", "", ErrNotAvailable }

// Sign is not supported on this platform.
func Sign(_, _, _ string) (string, error) { return "", ErrNotAvailable }

// DeleteKey is not supported on this platform.
func DeleteKey(_ string) error { return ErrNotAvailable }

// Diagnostic is not supported on this platform.
func Diagnostic() (bool, bool, bool, error) { return false, false, false, ErrNotAvailable }
