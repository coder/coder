package coderdtest_test

import (
	"testing"

	"github.com/coder/coder/coderd/coderdtest"
	"go.uber.org/goleak"
)

func TestNew(t *testing.T) {
	defer goleak.VerifyNone(t)

	_ = coderdtest.New(t)
}
