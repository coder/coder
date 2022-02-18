package usershell

// Get returns the command prompt binary name.
func Get(username string) (string, error) {
	return "cmd.exe", nil
}
