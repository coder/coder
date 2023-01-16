package testutil

import (
	"os"
	"runtime"
	"testing"
)

func TempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Cleanup(func() {
		if runtime.GOOS == "windows" {
			tries := 50
			var err error
			for i := 0; i < tries; i++ {
				err = os.RemoveAll(dir)
				if err == nil {
					return
				}
			}
			// Note that even though we're giving up here, t.TempDir
			// cleanup _could_ still succeed (or fail the test).
			t.Logf("TempDir: delete %q: giving up after %d tries: %v", dir, tries, err)
		}
	})
	return dir
}
