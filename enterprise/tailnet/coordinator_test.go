package tailnet_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/enterprise/tailnet"
	agpl "github.com/coder/coder/tailnet"
	"github.com/coder/coder/tailnet/coordinatortest"
)

type haCoordinator struct {
	pubsub database.Pubsub
}

func (h *haCoordinator) New(t testing.TB) agpl.Coordinator {
	coordinator, err := tailnet.NewCoordinator(slogtest.Make(t, nil), h.pubsub)
	require.NoError(t, err)
	return coordinator
}

func TestCoordinator(t *testing.T) {
	t.Parallel()

	t.Run("Local", func(t *testing.T) {
		t.Parallel()

		coordinatortest.RunCoordinatorSuite(t, func(t testing.TB) coordinatortest.CoordinatorFactory {
			coordinator, err := tailnet.NewCoordinator(slogtest.Make(t, nil), database.NewPubsubInMemory())
			require.NoError(t, err)
			return coordinatortest.NewLocalFactory(coordinator)
		})
	})

	t.Run("HA", func(t *testing.T) {
		t.Parallel()

		coordinatortest.RunCoordinatorSuite(t, func(t testing.TB) coordinatortest.CoordinatorFactory {
			return &haCoordinator{pubsub: database.NewPubsubInMemory()}
		})
	})
}
