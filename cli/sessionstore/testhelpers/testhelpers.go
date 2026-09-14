package testhelpers

import (
	"fmt"
	"os"
	"testing"
)

// KeyringServiceName generates a test service name for use with the OS keyring.
// It intends to prevent keyring usage collisions between parallel tests within a
// process and parallel test processes (which may occur on CI).
func KeyringServiceName(t *testing.T) string {
	t.Helper()
	return t.Name() + "_" + fmt.Sprintf("%v", os.Getpid())
}
