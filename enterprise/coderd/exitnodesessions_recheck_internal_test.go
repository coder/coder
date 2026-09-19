package coderd

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"cdr.dev/slog/v3/sloggers/slogtest"
	agplcoderd "github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/pubsub"
)

func TestExitNodeReplicaReaperRechecksHeartbeat(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	now := time.Now().UTC()
	exitNodeID := uuid.New()
	replicaID := uuid.New()
	store := dbmock.NewMockStore(gomock.NewController(t))
	store.EXPECT().GetAllLiveExitNodeReplicas(gomock.Any(), gomock.Any()).Return(nil, nil)
	store.EXPECT().GetExitNodeReplicaByID(gomock.Any(), replicaID).Return(database.ExitNodeReplica{
		ID: replicaID, ExitNodeID: exitNodeID, UpdatedAt: now,
	}, nil)
	ps := pubsub.NewInMemory()
	t.Cleanup(func() { require.NoError(t, ps.Close()) })

	canceled := false
	registry := newExitNodeReplicaSessionRegistry(nil)
	registry.live[replicaID] = exitNodeID
	registry.register(replicaID, func() { canceled = true })
	api := &API{
		ctx: ctx,
		Options: &Options{Options: &agplcoderd.Options{
			Database: store,
			Pubsub:   ps,
			Logger:   slogtest.Make(t, nil),
		}},
		exitNodeReplicaSessions: registry,
	}

	api.reapExitNodeReplicas(now)
	require.False(t, canceled)
	require.Equal(t, exitNodeID, registry.live[replicaID])
}
