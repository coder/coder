package prebuilds

import (
	"context"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
)

type AGPLPrebuildClaimer struct{}

func (c AGPLPrebuildClaimer) Claim(context.Context, database.Store, uuid.UUID, string, uuid.UUID) (*uuid.UUID, error) {
	// Not entitled to claim prebuilds in AGPL version.
	return nil, nil
}

func (c AGPLPrebuildClaimer) Initiator() uuid.UUID {
	return uuid.Nil
}

var DefaultClaimer Claimer = AGPLPrebuildClaimer{}
