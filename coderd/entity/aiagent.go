package entity

import (
	"context"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
)

// CreateAIAgentParams are the inputs to creating an AI agent.
type CreateAIAgentParams struct {
	// OwnerID is the principal on whose behalf the AI agent acts.
	OwnerID uuid.UUID

	// Actor is the party whose act this creation is. For now that is the
	// workspace_agent the request arrived over, which is the only party the
	// control plane has authenticated at the point of creation. Telling apart
	// the party that asked from the party that relayed needs data the API
	// calls do not yet carry.
	Actor Ref
}

// CreateAIAgent creates an AI agent and returns the identity minted for it.
//
// The identity is minted here rather than accepted from a caller, so that the
// control plane is the only party that can name an AI agent.
//
// The row and the entry accounting for it are written in one transaction, so
// there is no moment at which an AI agent exists unaccounted for, or is
// accounted for without existing. Where the entry and the effect are both rows
// in one transaction, the ordering problem that reconciliation exists to
// resolve does not arise.
//
// store may be a transaction handle. Given one, this joins it and commits
// nothing itself, so that creation can be made atomic with work that is not
// creation. Given a plain store, it opens its own.
func CreateAIAgent(ctx context.Context, store database.Store, params CreateAIAgentParams) (uuid.UUID, error) {
	// The actor's type is not checked here. AppendEntry checks it, and an
	// absent type is one of the values it rejects.
	if params.Actor.ID == uuid.Nil {
		return uuid.Nil, xerrors.New("an entry needs an actor, so creation needs one")
	}
	if params.OwnerID == uuid.Nil {
		return uuid.Nil, xerrors.New("an AI agent acts for a principal, so creation needs an owner")
	}

	id := uuid.New()
	err := store.InTx(func(tx database.Store) error {
		_, err := tx.InsertAIAgent(ctx, database.InsertAIAgentParams{
			ID:      id,
			OwnerID: params.OwnerID,
		})
		if err != nil {
			return xerrors.Errorf("insert AI agent: %w", err)
		}

		_, err = AppendEntry(ctx, tx, Entry{
			Event:   EventCreated,
			Subject: Ref{Type: TypeAIAgent, ID: id},
			Actor:   params.Actor,
		})
		if err != nil {
			return xerrors.Errorf("append creation entry: %w", err)
		}
		return nil
	}, nil)
	if err != nil {
		return uuid.Nil, err
	}

	return id, nil
}
