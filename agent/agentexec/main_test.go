package agentexec_test

import (
	"fmt"
	"os"
	"os/exec"
	"testing"
)

const TestBin = "/tmp/agent-test"

func TestMain(m *testing.M) {
	buildBinary()

	os.Exit(m.Run())
}

func buildBinary() {
	out, err := exec.Command("go", "build", "-o", TestBin, "./cmdtest").CombinedOutput()
	mustf(err, "build binary: %s", out)
}

func mustf(err error, msg string, args ...any) {
	if err != nil {
		panic(fmt.Sprintf(msg, args...))
	}
}
