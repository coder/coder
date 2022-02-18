package usershell

import "os"

// Get returns the $SHELL environment variable.
// TODO: This should use "dscl" to fetch the proper value. See:
// https://stackoverflow.com/questions/16375519/how-to-get-the-default-shell
func Get(username string) (string, error) {
	return os.Getenv("SHELL"), nil
}
