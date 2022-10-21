package tailnet_test

import (
	"testing"

	"github.com/coder/coder/tailnet"
	"github.com/coder/coder/tailnet/coordinatortest"
)

func TestCoordinator(t *testing.T) {
	t.Parallel()

	coordinatortest.RunCoordinatorSuite(t, func(testing.TB) coordinatortest.CoordinatorFactory {
		return coordinatortest.NewLocalFactory(tailnet.NewCoordinator())
	})
}
