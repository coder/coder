package chatsanitize_test

import (
	"testing"

	"go.uber.org/goleak"

	"github.com/coder/coder/v2/testutil"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m, testutil.GoleakOptions...)
}
