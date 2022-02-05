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
	server := coderdtest.New(t)
	_ = coderdtest.NewInitialUser(t, server.Client)
	_ = coderdtest.NewProvisionerDaemon(t, server.Client)
}
