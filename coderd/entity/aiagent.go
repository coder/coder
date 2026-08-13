package entity

import (
	"context"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
)

// CreateAIAgentParams are the inputs to creating an AI agent.
type CreateAIAgentParams struct {
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
// Only the journal entry is written so far. When the table holding AI agent
// identities exists, its row is inserted inside the same transaction as the
// entry, which is what the transaction below is for: the entry and the state
// change it accounts for commit together or not at all.
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

	id := uuid.New()
	err := store.InTx(func(tx database.Store) error {
		_, err := AppendEntry(ctx, tx, Entry{
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
