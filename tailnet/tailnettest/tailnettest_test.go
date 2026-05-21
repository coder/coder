package tailnettest_test

import (
	"testing"

	"go.uber.org/goleak"

	"github.com/coder/coder/v2/tailnet/tailnettest"
	"github.com/coder/coder/v2/testutil"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m, testutil.GoleakOptions...)
}

func TestRunDERPAndSTUN(t *testing.T) {
	t.Parallel()
	_, _ = tailnettest.RunDERPAndSTUN(t)
}

func TestRunDERPOnlyWebSockets(t *testing.T) {
	t.Parallel()
	_ = tailnettest.RunDERPOnlyWebSockets(t)
}
