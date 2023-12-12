//go:build !linux

package agentssh

func getListeningPortProcessCmdline(uint32) (string, error) {
	// We are not worrying about other platforms at the moment because Gateway
	// only supports Linux anyway.
	return "", nil
}
