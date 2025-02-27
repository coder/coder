package prebuilds

import (
	"context"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/google/uuid"
)

type Claimer interface {
	Claim(ctx context.Context, store database.Store, userID uuid.UUID, name string, presetID uuid.UUID) (*uuid.UUID, error)
	Initiator() uuid.UUID
}

type AGPLPrebuildClaimer struct{}

func (c AGPLPrebuildClaimer) Claim(context.Context, database.Store, uuid.UUID, string, uuid.UUID) (*uuid.UUID, error) {
	// Not entitled to claim prebuilds in AGPL version.
	return nil, nil
}

func (c AGPLPrebuildClaimer) Initiator() uuid.UUID {
	return uuid.Nil
}

var DefaultClaimer Claimer = AGPLPrebuildClaimer{}
