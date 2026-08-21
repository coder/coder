package entity

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
)

// AI agent states. See the AI agent lifecycle in poc_audit/entity_model.md.
//
// AIAgentStateDormant is reserved for future use and is unreachable in the
// machine implemented here, which has active and retired only. It exists so
// that supporting reconstitution later costs no migration, which means a switch
// over these values must handle a state that cannot occur.
const (
	AIAgentStateActive  = "active"
	AIAgentStateDormant = "dormant"
	AIAgentStateRetired = "retired"
)

// AI agent lifecycle events.
//
// EventAIAgentFinish is observed: an embodiment returned of its own accord and
// somebody noticed. EventAIAgentKill is commanded: a party ended it
// deliberately. Both reach retired, and which of them a reader finds is how
// they are told apart, since the state alone cannot say.
const (
	EventAIAgentCreate Event = "create"
	EventAIAgentFinish Event = "finish"
	EventAIAgentKill   Event = "kill"
)

// CreateAIAgentParams are the inputs to creating an AI agent.
type CreateAIAgentParams struct {
	// Owner is the principal the AI agent belongs to, and is also the actor:
	// creation is commanded, and what commands it is the order that brought
	// the AI agent about. A workspace_agent relaying that order to the control
	// plane creates nothing.
	//
	// Ownership is not authorization, though the two coincide here. See
	// "Ownership is not authorization" in poc_audit/entity_model.md.
	Owner Ref
}

// NewAIAgent is what creating an AI agent produced.
type NewAIAgent struct {
	ID uuid.UUID

	// AuthorizationID identifies the grant made to this AI agent at creation.
	AuthorizationID uuid.UUID

	// CredentialID identifies the credential issued at creation, and is not
	// derived from the authenticator.
	CredentialID uuid.UUID

	// Authenticator is what the AI agent possesses and controls. This is the
	// only time it can be had.
	Authenticator string
}

// CreateAIAgent creates an AI agent, grants it authority, issues it a
// credential, and returns what only this moment can give.
//
// The identity is minted here rather than accepted from a caller, so that the
// control plane is the only party that can name an AI agent.
//
// Creation, the grant of authorization, and the issuance of a credential are
// three events and take three entries, in three journals, because they are
// three different things happening to three different entities. All are written
// in this transaction, so nothing is left to reconcile between them.
//
// store may be a transaction handle. Given one, this joins it and commits
// nothing itself, so that creation can be made atomic with work that is not
// creation. Given a plain store, it opens its own.
func CreateAIAgent(ctx context.Context, store database.Store, params CreateAIAgentParams) (NewAIAgent, error) {
	if !params.Owner.Type.Valid() {
		return NewAIAgent{}, xerrors.Errorf("owner type %q names no kind of entity", params.Owner.Type)
	}
	if params.Owner.ID == uuid.Nil {
		return NewAIAgent{}, xerrors.New("an AI agent belongs to a principal, so creation needs one")
	}

	created := NewAIAgent{ID: uuid.New()}
	err := store.InTx(func(tx database.Store) error {
		// The order here is the order of dependency, not of necessity. Inside
		// one transaction nothing observes it, but source that reads in the
		// wrong order invites the reader to infer the wrong dependencies.
		if err := recordAIAgentCreation(ctx, tx, created.ID, params.Owner); err != nil {
			return err
		}

		// The grant comes after the agent exists, because an entity that does
		// not exist cannot be party to an agency relation.
		//
		// Ordering an AI agent into existence is itself the grant: the order
		// confers authority on the agent about to exist, and nothing further is
		// required of the principal. It cannot be perfected at that moment,
		// there being no identity yet to confer authority on, and an AI agent
		// is not identified until it has been embodied. This entry, written
		// after embodiment, is what perfects it. See "How the authorization
		// machine is read" in poc_audit/entity_model.md.
		var err error
		created.AuthorizationID, err = GrantUniversalAuthorization(ctx, tx, GrantParams{
			Principal: params.Owner,
			Agent:     Ref{Type: TypeAIAgent, ID: created.ID},
		})
		if err != nil {
			return xerrors.Errorf("grant authorization: %w", err)
		}

		// The credential comes last. It is a means of exercising authority, so
		// it follows the authority it lets its holder exercise, just as that
		// authority follows the party it was conferred on. A credential issued
		// before either would evidence something that did not yet exist.
		issued, err := IssueCredential(ctx, tx, IssueCredentialParams{
			Holder: Ref{Type: TypeAIAgent, ID: created.ID},
			Actor:  params.Owner,
		})
		if err != nil {
			return xerrors.Errorf("issue credential: %w", err)
		}
		created.CredentialID = issued.ID
		created.Authenticator = issued.Authenticator
		return nil
	}, nil)
	if err != nil {
		return NewAIAgent{}, err
	}

	return created, nil
}

// RetireAIAgent ends an AI agent's life and records how.
//
// event says which way: finish for an embodiment that returned of its own
// accord, kill for one a party ended deliberately. Both reach the same state,
// and the entry is the only thing that says which happened.
//
// effectiveAt is when it happened, which for a finish may be well before
// anybody noticed. Zero means now.
func RetireAIAgent(ctx context.Context, store database.Store, id uuid.UUID, event Event, actor Ref, effectiveAt time.Time) error {
	switch event {
	case EventAIAgentFinish, EventAIAgentKill:
	default:
		return xerrors.Errorf("event %q does not retire an AI agent", event)
	}
	if !actor.Type.Valid() {
		return xerrors.Errorf("actor type %q names no kind of entity", actor.Type)
	}
	if actor.ID == uuid.Nil {
		return xerrors.New("an entry needs an actor, so retirement needs one")
	}

	effective := effectiveAt
	if effective.IsZero() {
		effective = time.Now()
	}

	return store.InTx(func(tx database.Store) error {
		current, err := tx.GetAIAgentLifecycleLedgerRowByID(ctx, id)
		if err != nil {
			return xerrors.Errorf("read the AI agent: %w", err)
		}
		if current.State != AIAgentStateActive {
			return xerrors.Errorf("AI agent %s is already %s", id, current.State)
		}

		entryID, err := tx.NextAIAgentLifecycleJournalEntryID(ctx)
		if err != nil {
			return xerrors.Errorf("take an entry identifier: %w", err)
		}
		_, err = tx.InsertAIAgentLifecycleJournalFirstLine(ctx, database.InsertAIAgentLifecycleJournalFirstLineParams{
			EntryID:       entryID,
			EffectiveDate: sql.NullTime{Time: effective, Valid: true},
			ActorType:     sql.NullString{String: string(actor.Type), Valid: true},
			Actor:         uuid.NullUUID{UUID: actor.ID, Valid: true},
			Event:         string(event),
			Subject:       id,
		})
		if err != nil {
			return xerrors.Errorf("append retirement entry: %w", err)
		}
		if _, err := tx.RetireAIAgent(ctx, database.RetireAIAgentParams{
			ID:                 id,
			PostingReference:   entryID,
			PostingReference_2: current.PostingReference,
		}); err != nil {
			return xerrors.Errorf("post the retirement: %w", err)
		}
		return nil
	}, nil)
}

// recordAIAgentCreation writes the entry and posts it, in that order. The
// journal is the book of original entry and the ledger row is derived from it,
// which is also why the row carries the identifier of the entry that produced
// it.
func recordAIAgentCreation(ctx context.Context, tx database.Store, id uuid.UUID, owner Ref) error {
	entryID, err := tx.NextAIAgentLifecycleJournalEntryID(ctx)
	if err != nil {
		return xerrors.Errorf("take an entry identifier: %w", err)
	}

	_, err = tx.InsertAIAgentLifecycleJournalFirstLine(ctx, database.InsertAIAgentLifecycleJournalFirstLineParams{
		EntryID:       entryID,
		EffectiveDate: sql.NullTime{Time: time.Now(), Valid: true},
		ActorType:     sql.NullString{String: string(owner.Type), Valid: true},
		Actor:         uuid.NullUUID{UUID: owner.ID, Valid: true},
		Event:         string(EventAIAgentCreate),
		Subject:       id,
	})
	if err != nil {
		return xerrors.Errorf("append creation entry: %w", err)
	}

	if _, err := tx.InsertAIAgentLifecycleLedgerRow(ctx, database.InsertAIAgentLifecycleLedgerRowParams{
		ID:               id,
		OwnerType:        string(owner.Type),
		OwnerID:          owner.ID,
		State:            AIAgentStateActive,
		PostingReference: entryID,
	}); err != nil {
		return xerrors.Errorf("post to the ledger: %w", err)
	}
	return nil
}
