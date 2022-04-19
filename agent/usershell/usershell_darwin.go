package usershell

import "os"

// Get returns the $SHELL environment variable.
func Get(username string) (string, error) {
	return os.Getenv("SHELL"), nil
}
