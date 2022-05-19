package usershell

import "os"

// Get returns the $SHELL environment variable.
func Get(_ string) (string, error) {
	return os.Getenv("SHELL"), nil
}
