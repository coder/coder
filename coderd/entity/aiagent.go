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

// NewAIAgent is what creating an AI agent produced. The credential is here
// because this is the only time anything hands it out.
type NewAIAgent struct {
	ID         uuid.UUID
	Credential string
}

// CreateAIAgent creates an AI agent, issues it a credential, and returns both.
//
// The identity is minted here rather than accepted from a caller, so that the
// control plane is the only party that can name an AI agent.
//
// The row, the credential, and the entry accounting for the creation are
// written in one transaction, so there is no moment at which an AI agent exists
// unaccounted for, or is accounted for without existing. Where the entry and
// the effect are both rows in one transaction, the ordering problem that
// reconciliation exists to resolve does not arise.
//
// Issuing the credential is not journaled. Credential lifecycle is out of
// scope for the proof of concept, which reproduces P7 in
// poc_audit/security_findings.md on purpose and for now.
//
// store may be a transaction handle. Given one, this joins it and commits
// nothing itself, so that creation can be made atomic with work that is not
// creation. Given a plain store, it opens its own.
func CreateAIAgent(ctx context.Context, store database.Store, params CreateAIAgentParams) (NewAIAgent, error) {
	// The actor's type is not checked here. AppendEntry checks it, and an
	// absent type is one of the values it rejects.
	if params.Actor.ID == uuid.Nil {
		return NewAIAgent{}, xerrors.New("an entry needs an actor, so creation needs one")
	}
	if params.OwnerID == uuid.Nil {
		return NewAIAgent{}, xerrors.New("an AI agent acts for a principal, so creation needs an owner")
	}

	created := NewAIAgent{ID: uuid.New()}
	err := store.InTx(func(tx database.Store) error {
		_, err := tx.InsertEntityAIAgent(ctx, database.InsertEntityAIAgentParams{
			ID:      created.ID,
			OwnerID: params.OwnerID,
		})
		if err != nil {
			return xerrors.Errorf("insert AI agent: %w", err)
		}

		created.Credential, err = IssueCredential(ctx, tx, Ref{Type: TypeAIAgent, ID: created.ID})
		if err != nil {
			return xerrors.Errorf("issue credential: %w", err)
		}

		_, err = AppendEntry(ctx, tx, Entry{
			Event:   EventCreated,
			Subject: Ref{Type: TypeAIAgent, ID: created.ID},
			Actor:   params.Actor,
		})
		if err != nil {
			return xerrors.Errorf("append creation entry: %w", err)
		}
		return nil
	}, nil)
	if err != nil {
		return NewAIAgent{}, err
	}

	return created, nil
}
