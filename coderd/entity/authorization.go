package entity

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
)

// StateActive and StateTerminated are the states of an authorization. See the
// authorization lifecycle in poc_audit/entity_model.md. There is no state
// between them: a grant is complete once perfected, so the relation either
// holds or it does not.
const (
	StateActive     = "active"
	StateTerminated = "terminated"
)

// EventGrant names the transition that brings an authorization into being.
const EventGrant Event = "grant"

// UniversalScope is the only scope the proof of concept issues. Everything the
// principal may do, the agent may do, which is the grant that restricts nothing
// and so has the shortest description. It is deliberately empty rather than a
// word meaning "everything": a later authorization language can define its own
// syntax without having to reserve one.
const UniversalScope = ""

// GrantParams are the inputs to granting authorization.
type GrantParams struct {
	// Principal is the party conferring authority. Only a principal can grant,
	// so this is also the actor on the entry.
	Principal Ref

	// Agent is the party receiving it. It must already exist: there is no
	// granting anything to a party that is not there.
	Agent Ref

	// EffectiveAt is when the grant was made. Zero means now, which is correct
	// where the grant is being perfected by this recording rather than
	// reported after the fact.
	EffectiveAt time.Time
}

// GrantUniversalAuthorization confers universal authority from a principal on
// an agent and records it. Universal means the grant restricts nothing:
// everything the principal may do, the agent may do.
//
// # Precondition: the agent exists
//
// An entity that does not exist cannot be party to an agency relation, so the
// agent must exist before this is called. Nothing here enforces that.
//
// Enforcement is not available cheaply. This is ordinarily called in the same
// transaction that creates the agent, so there is no prior perfected record of
// the agent's existence to consult, and a foreign key would demand the ledger
// row before the entry that accounts for it. Taking the agent as an argument
// encourages a caller to have one in hand; it does not oblige them to have a
// real one. The gap is a consequence of keeping these two lifecycles in
// separate modules, which is worth more than closing it here would be.
//
// The gap is therefore left open deliberately and handed to reconciliation
// rather than to a check. A grant naming an agent that was never created is
// detectable after the fact by reading the two journals together, and
// poc_audit/entity_model.md describes that under "A grant may name an agent
// that does not exist".
//
// The entry and the ledger row are written together, so no moment exists at
// which authority is held without being accounted for, or accounted for
// without being held. The entry is written first: it is the book of original
// entry, and the ledger row is derived from it, which is also why the row
// carries the identifier of the entry that produced it.
//
// Recording the grant is what perfects it. Nothing here reports a grant that
// happened somewhere else.
//
// store may be a transaction handle, in which case this joins it and commits
// nothing itself.
func GrantUniversalAuthorization(ctx context.Context, store database.Store, params GrantParams) (uuid.UUID, error) {
	if !params.Principal.Type.Valid() || params.Principal.ID == uuid.Nil {
		return uuid.Nil, xerrors.New("a grant is an act of a principal, so it needs one")
	}
	if !params.Agent.Type.Valid() || params.Agent.ID == uuid.Nil {
		return uuid.Nil, xerrors.New("a grant confers authority on an agent, so it needs one")
	}

	effective := params.EffectiveAt
	if effective.IsZero() {
		effective = time.Now()
	}

	id := uuid.New()
	err := store.InTx(func(tx database.Store) error {
		entryID, err := tx.NextAuthorizationLifecycleJournalEntryID(ctx)
		if err != nil {
			return xerrors.Errorf("take an entry identifier: %w", err)
		}

		_, err = tx.InsertAuthorizationLifecycleJournalFirstLine(ctx, database.InsertAuthorizationLifecycleJournalFirstLineParams{
			EntryID:       entryID,
			EffectiveDate: sql.NullTime{Time: effective, Valid: true},
			ActorType:     sql.NullString{String: string(params.Principal.Type), Valid: true},
			Actor:         uuid.NullUUID{UUID: params.Principal.ID, Valid: true},
			Event:         string(EventGrant),
			Subject:       id,
		})
		if err != nil {
			return xerrors.Errorf("append grant entry: %w", err)
		}

		_, err = tx.InsertAuthorizationLifecycleLedgerRow(ctx, database.InsertAuthorizationLifecycleLedgerRowParams{
			ID:               id,
			PrincipalType:    string(params.Principal.Type),
			PrincipalID:      params.Principal.ID,
			AgentType:        string(params.Agent.Type),
			AgentID:          params.Agent.ID,
			State:            StateActive,
			PostingReference: entryID,
		})
		if err != nil {
			return xerrors.Errorf("post to the ledger: %w", err)
		}
		return nil
	}, nil)
	if err != nil {
		return uuid.Nil, err
	}
	return id, nil
}
