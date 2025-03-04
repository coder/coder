package prebuilds

import (
	"context"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
)

type Reconciler interface {
	RunLoop(ctx context.Context)
	Stop(ctx context.Context, cause error)
	ReconcileAll(ctx context.Context) error
}

type Claimer interface {
	Claim(ctx context.Context, store database.Store, userID uuid.UUID, name string, presetID uuid.UUID) (*uuid.UUID, error)
	Initiator() uuid.UUID
}
