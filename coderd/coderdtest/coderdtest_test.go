package coderdtest_test

import (
	"testing"

	"go.uber.org/goleak"

	"github.com/coder/coder/coderd/coderdtest"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestNew(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t)
	_ = coderdtest.NewInitialUser(t, client)
	_ = coderdtest.NewProvisionerDaemon(t, client)
}
