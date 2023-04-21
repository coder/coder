package tailnettest_test

import (
	"testing"

	"go.uber.org/goleak"

	"github.com/coder/coder/tailnet/tailnettest"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestRunDERPAndSTUN(t *testing.T) {
	t.Parallel()
	_ = tailnettest.RunDERPAndSTUN(t)
}

func TestRunDERPOnlyWebSockets(t *testing.T) {
	t.Parallel()
	_ = tailnettest.RunDERPOnlyWebSockets(t)
}
