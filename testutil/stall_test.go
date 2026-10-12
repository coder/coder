package testutil_test

import (
	"testing"
	"time"

	"github.com/coder/coder/v2/testutil"
)

func TestStartStallDetector(t *testing.T) {
	t.Parallel()

	stop := testutil.StartStallDetector(time.Hour)
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		stop()
	}()
	select {
	case <-stopped:
	case <-time.After(testutil.WaitShort):
		t.Fatal("stall detector did not stop")
	}
}
