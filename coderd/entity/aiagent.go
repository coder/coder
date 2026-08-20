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
	ID uuid.UUID

	// AuthorizationID identifies the grant made to this AI agent at creation.
	AuthorizationID uuid.UUID

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
// Creation and the grant of authorization are two events and take two entries,
// in two journals, because they are two different things happening to two
// different entities. Both are written in this transaction, so nothing is left
// to reconcile between them.
//
// Issuing the credential is not journaled. Credential lifecycle is out of
// scope for the proof of concept, which reproduces P7 in
// poc_audit/security_findings.md on purpose and for now, and there is no
// credential journal to write to yet.
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
		// The order here is the order of dependency, not of necessity. Inside
		// one transaction nothing observes it, but source that reads in the
		// wrong order invites the reader to infer the wrong dependencies.
		//
		// The entry comes before the row it accounts for: the journal is the
		// book of original entry and the row is derived from it. Writing the
		// row first would read as the objection this work makes against
		// trigger-written journals, that an entry is subordinate to some prior
		// write.
		_, err := AppendEntry(ctx, tx, Entry{
			Event:   EventCreated,
			Subject: Ref{Type: TypeAIAgent, ID: created.ID},
			Actor:   params.Actor,
		})
		if err != nil {
			return xerrors.Errorf("append creation entry: %w", err)
		}

		_, err = tx.InsertEntityAIAgent(ctx, database.InsertEntityAIAgentParams{
			ID:      created.ID,
			OwnerID: params.OwnerID,
		})
		if err != nil {
			return xerrors.Errorf("insert AI agent: %w", err)
		}

		// The grant comes after the agent exists, because an entity that does
		// not exist cannot be party to an agency relation.
		//
		// The actor here is the owner, not params.Actor. A grant is an act of
		// the principal and of nobody else, and a workspace_agent relaying the
		// request confers nothing, holding no authority to confer.
		//
		// Nothing is asserted without warrant by recording the owner. Ordering
		// an AI agent into existence is itself the grant: the order confers
		// authority on the agent about to exist, and nothing further is
		// required of the principal. It cannot be perfected at that moment,
		// there being no identity yet to confer authority on, and an AI agent
		// is not identified until it has been embodied. This entry, written
		// after embodiment, is what perfects it. The interval is required by
		// the model rather than papered over by it. See "How the authorization
		// machine is read" in poc_audit/entity_model.md.
		created.AuthorizationID, err = GrantUniversalAuthorization(ctx, tx, GrantParams{
			Principal: Ref{Type: TypeUser, ID: params.OwnerID},
			Agent:     Ref{Type: TypeAIAgent, ID: created.ID},
		})
		if err != nil {
			return xerrors.Errorf("grant authorization: %w", err)
		}

		// The credential comes last. It is a means of exercising authority, so
		// it follows the authority it lets its holder exercise, just as that
		// authority follows the party it was conferred on. A credential issued
		// before either would evidence something that did not yet exist.
		created.Credential, err = IssueCredential(ctx, tx, Ref{Type: TypeAIAgent, ID: created.ID})
		if err != nil {
			return xerrors.Errorf("issue credential: %w", err)
		}
		return nil
	}, nil)
	if err != nil {
		return NewAIAgent{}, err
	}

	return created, nil
}
