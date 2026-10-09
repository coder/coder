package cli_test

import (
	"os"
	"testing"
	"time"

	"github.com/coder/coder/v2/testutil"
)

func TestMain(m *testing.M) {
	// Report multi-second pauses of the whole process, see coder/internal#1365.
	testutil.StartStallDetector(5 * time.Second)
	// Fail with a heap profile before a runaway allocation pushes the runner into swap.
	testutil.StartMemoryGuard(testutil.MemoryLimit)
	os.Exit(m.Run())
}
