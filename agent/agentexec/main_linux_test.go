//go:build linux
// +build linux

package agentexec_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

var TestBin string

func TestMain(m *testing.M) {
	code := func() int {
		// We generate a unique directory per test invocation to avoid collisions between two
		// processes attempting to create the same temp file.
		dir := genDir()
		defer os.RemoveAll(dir)
		TestBin = buildBinary(dir)
		return m.Run()
	}()

	os.Exit(code)
}

func buildBinary(dir string) string {
	path := filepath.Join(dir, "agent-test")
	out, err := exec.Command("go", "build", "-o", path, "./cmdtest").CombinedOutput()
	mustf(err, "build binary: %s", out)
	return path
}

func mustf(err error, msg string, args ...any) {
	if err != nil {
		panic(fmt.Sprintf(msg, args...))
	}
}

func genDir() string {
	dir, err := os.MkdirTemp(os.TempDir(), "agentexec")
	mustf(err, "create temp dir: %v", err)
	return dir
}
