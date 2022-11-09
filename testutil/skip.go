package testutil

import (
	"fmt"
	"runtime"
	"testing"
)

func SkipIfWindows(t *testing.T, msg string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip(fmt.Sprintf("skipping test on windows - %s", msg))
	}
}
