package entity

import (
	"context"
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

	// CreationSite is what the AI agent was created in, as a pair, the thing
	// being of more than one kind.
	//
	// **It is set at creation and is never updated.** An AI agent that moved
	// between a chat and a workspace would keep the site it was created in, and
	// nothing moves one today. That makes it a fact about the creation event
	// rather than a description of where the agent currently runs, which is why
	// it belongs on the creation entry and folds onto the ledger from there.
	CreationSite CreationSite
}

// CreationSite is what an AI agent was created in.
type CreationSite struct {
	Type CreationSiteType
	ID   uuid.UUID
}

// CreationSiteType is the kind of thing an AI agent was created in.
//
// A closed set, held closed by a CHECK rather than an enum, for the reason
// given in "An actor type column on a core table is text with a CHECK" in
// poc_audit/implementation_patterns.md. A database enum of these values exists
// already, created by the AI identity code, and is deliberately not reused: a
// type is not a table, so sharing one would couple this schema to a definition
// this work does not own without buying anything.
type CreationSiteType string

const (
	CreationSiteTypeChat      CreationSiteType = "chat"
	CreationSiteTypeWorkspace CreationSiteType = "workspace"
)

// Valid reports whether t is a member of the closed set.
func (t CreationSiteType) Valid() bool {
	switch t {
	case CreationSiteTypeChat, CreationSiteTypeWorkspace:
		return true
	default:
		return false
	}
}

// abbreviation is what the type contributes to a displayed name. Names are
// read in log lines and want to be short; the stored value is the model's
// vocabulary and wants to be plain.
func (t CreationSiteType) abbreviation() string {
	switch t {
	case CreationSiteTypeChat:
		return "chat"
	case CreationSiteTypeWorkspace:
		return "ws"
	default:
		return "unknown"
	}
}

// DisplayName is what a human reads where an AI agent is named.
//
// **Computed, never stored.** A stored name is a rendering that has to be kept
// in step with what it renders, and nothing here needs one: the name is used
// for logging and display, and `rbac.Subject` says of its own friendly name
// that it "is entirely optional". Computing it also removes the uniqueness the
// name inherited from being a username, and the retry loop that uniqueness
// forced.
//
// The creation site makes the name say what kind of agent this is, which is the
// whole of what the previous generated name said beyond identifying it.
func DisplayName(site CreationSiteType, id uuid.UUID) string {
	return "ai-" + site.abbreviation() + "-" + id.String()
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
// **The three are together because WP1 needed to exercise end to end flow of
// control, not because anything argues they belong together.** Two of them are
// expected to leave.
//
// Issuing a credential has no long term reason to be here at all, and the work
// package that moves it is not yet written.
//
// **The grant may stay only while every grant is universal.** A universal grant
// takes no parameters, so creating an AI agent can imply one without deciding
// anything. A scoped grant is a decision, and a decision cannot be a side
// effect of creating the thing it is about. So whoever introduces a scoped
// grant has to take the grant out of here in the same pass.
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
	if !params.CreationSite.Type.Valid() {
		return NewAIAgent{}, xerrors.Errorf("creation site type %q names no kind of thing", params.CreationSite.Type)
	}
	if params.CreationSite.ID == uuid.Nil {
		return NewAIAgent{}, xerrors.New("an AI agent is embodied in something, so creation needs to say what")
	}

	created := NewAIAgent{ID: uuid.New()}
	err := store.InTx(func(tx database.Store) error {
		// The order here is the order of dependency, not of necessity. Inside
		// one transaction nothing observes it, but source that reads in the
		// wrong order invites the reader to infer the wrong dependencies.
		if err := recordAIAgentCreation(ctx, tx, created.ID, params.Owner, params.CreationSite); err != nil {
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
//
// **Retirement ends more than the agent.** An authorization naming a party that
// has ceased to exist cannot go on holding, and a credential authenticating one
// authenticates nobody, so both lapse here. All of it commits together, the
// three endings arising together in the sense the audit approach gives that
// term, and each lapse takes the retirement's effective date because that is
// when it happened rather than when it was written.
//
// The retirement is commanded and the two lapses are observed, so they do not
// share an actor the way the three entries of a creation do. See SystemActor.
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
		current, err := tx.GetAIAgentLedgerRowByID(ctx, id)
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
		_, err = tx.InsertAIAgentLifecycleJournalEntry(ctx, database.InsertAIAgentLifecycleJournalEntryParams{
			EntryID:       entryID,
			EffectiveDate: effective,
			ActorType:     string(actor.Type),
			Actor:         actor.ID,
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

		// Reading down the chain of dependency, as creation writes down it. An
		// authorization rests on the parties to it and a credential rests on
		// the authority it lets its holder exercise, so each is ended after the
		// thing it rested on.
		if err := lapseAuthorizationsOf(ctx, tx, id, effective); err != nil {
			return err
		}
		return lapseCredentialsOf(ctx, tx, id, effective)
	}, nil)
}

// lapseAuthorizationsOf ends every authorization naming a retired AI agent as
// agent.
//
// Rows already terminated are passed over rather than refused. A retirement is
// one event and the authorizations under it are several, so one of them having
// been revoked earlier says nothing about the others and is not a failure of
// this one.
func lapseAuthorizationsOf(ctx context.Context, tx database.Store, agentID uuid.UUID, effective time.Time) error {
	rows, err := tx.GetAuthorizationLedgerRowsByAgent(ctx, database.GetAuthorizationLedgerRowsByAgentParams{
		AgentType: string(TypeAIAgent),
		AgentID:   agentID,
	})
	if err != nil {
		return xerrors.Errorf("read the agent's authorizations: %w", err)
	}
	// The read returns every authorization whatever its state, so the ones
	// already ended are dropped rather than skipped inside the loop: they are
	// not part of this event and must not occupy a line of it.
	active := make([]database.AuthorizationLedger, 0, len(rows))
	for _, row := range rows {
		if row.State == StateActive {
			active = append(active, row)
		}
	}
	if err := LapseAuthorizations(ctx, tx, active, SystemActor, effective); err != nil {
		return xerrors.Errorf("lapse the agent's authorizations: %w", err)
	}
	return nil
}

// lapseCredentialsOf invalidates every credential a retired AI agent holds.
//
// The read returns only the valid ones, so unlike the authorizations there is
// nothing to drop. More than one may be valid at once, a rotation being allowed
// to overlap, so this is a set rather than a lookup, and the whole set ends as
// one event.
func lapseCredentialsOf(ctx context.Context, tx database.Store, agentID uuid.UUID, effective time.Time) error {
	rows, err := tx.GetValidCredentialsByHolder(ctx, database.GetValidCredentialsByHolderParams{
		HolderType: string(TypeAIAgent),
		HolderID:   agentID,
	})
	if err != nil {
		return xerrors.Errorf("read the agent's credentials: %w", err)
	}
	if err := LapseCredentials(ctx, tx, rows, SystemActor, effective); err != nil {
		return xerrors.Errorf("lapse the agent's credentials: %w", err)
	}
	return nil
}

// recordAIAgentCreation writes the entry and posts it, in that order. The
// journal is the book of original entry and the ledger row is derived from it,
// which is also why the row carries the identifier of the entry that produced
// it.
func recordAIAgentCreation(ctx context.Context, tx database.Store, id uuid.UUID, owner Ref, site CreationSite) error {
	entryID, err := tx.NextAIAgentLifecycleJournalEntryID(ctx)
	if err != nil {
		return xerrors.Errorf("take an entry identifier: %w", err)
	}

	// One reading of the clock, carried by the entry and by the ledger row
	// folded from it. Two readings could disagree, and the creation time is
	// the entry's effective date rather than a fact of its own.
	effective := time.Now()

	_, err = tx.InsertAIAgentLifecycleJournalEntry(ctx, database.InsertAIAgentLifecycleJournalEntryParams{
		EntryID:       entryID,
		EffectiveDate: effective,
		ActorType:     string(owner.Type),
		Actor:         owner.ID,
		Event:         string(EventAIAgentCreate),
		Subject:       id,
	})
	if err != nil {
		return xerrors.Errorf("append creation entry: %w", err)
	}

	// The line before the row it posts to, the journal being the book of
	// original entry. Line zero, this being the only line.
	if _, err := tx.InsertAIAgentLifecycleJournalCreateLine(ctx, database.InsertAIAgentLifecycleJournalCreateLineParams{
		EntryID:          entryID,
		Line:             0,
		CreationSiteType: string(site.Type),
		CreationSiteID:   site.ID,
	}); err != nil {
		return xerrors.Errorf("append the creation line: %w", err)
	}

	if _, err := tx.InsertAIAgentLedgerRow(ctx, database.InsertAIAgentLedgerRowParams{
		ID:               id,
		OwnerType:        string(owner.Type),
		OwnerID:          owner.ID,
		CreationSiteType: string(site.Type),
		CreationSiteID:   site.ID,
		State:            AIAgentStateActive,
		PostingReference: entryID,
		CreationTime:     effective,
	}); err != nil {
		return xerrors.Errorf("post to the ledger: %w", err)
	}
	return nil
}
