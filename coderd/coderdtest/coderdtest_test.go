package coderdtest_test

import (
	"testing"

	"github.com/coder/coder/coderd/coderdtest"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestNew(t *testing.T) {
	_ = coderdtest.New(t)
}
