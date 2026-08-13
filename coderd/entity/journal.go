// Package entity owns the lifecycle of entities, the identities and
// credentials issued to them, and the audit journal accounting for both.
//
// See DIRECTORY.md in this directory for what belongs here and why, and
// poc_audit/audit_approach.md for the approach the journal implements.
package entity

import (
	"context"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
)

// Type names the kind of entity an identifier refers to. It says which table
// holds the primary key, standing in for a foreign key into a union of the
// identity tables that SQL cannot express.
//
// The set is closed and AppendEntry refuses anything outside it. A type naming
// no table produces an entry whose subject can never be resolved, which breaks
// the journal's only link to the world.
//
// One set serves both roles. Every type below can be an actor or a subject: a
// workspace_agent acts, and its own creation is something that happens to it.
//
// Post proof of concept: the column has no CHECK constraint holding the set
// closed. That is deliberate, since every new type would then need a
// migration. The intended replacement is a periodic query sweeping for values
// outside the set, which also catches rows that never came through this
// package.
type Type string

const (
	TypeAIAgent        Type = "ai_agent"
	TypeWorkspaceAgent Type = "workspace_agent"
	// TypeUser also covers system actors, such as the account that creates
	// prebuilt workspaces. See the finding in poc_audit/entity_model.md: those
	// are users only because there was nowhere else to put them.
	TypeUser Type = "user"
)

// Valid reports whether t is a member of the closed set.
func (t Type) Valid() bool {
	switch t {
	case TypeAIAgent, TypeWorkspaceAgent, TypeUser:
		return true
	default:
		return false
	}
}

// Ref identifies one entity: what kind it is, and which one.
type Ref struct {
	Type Type
	ID   uuid.UUID
}

// Event names a persistent state change. The vocabulary is open text for now.
type Event string

const EventCreated Event = "created"

// Entry is an element of the journal. It records that an event happened to a
// subject, and which actor brought it about.
//
// It carries one actor rather than a principal and an agent. Delegation is
// authorized in advance and recorded separately, so an entry needs only the
// actor behind the action. That also keeps a single shape for the case where
// the actor is a user acting for themselves.
type Entry struct {
	Event   Event
	Subject Ref
	Actor   Ref
}

// AppendEntry writes one entry to the journal. Lifecycle functions call it
// rather than inserting inline, so that an entry and the state change it
// accounts for can commit together. That they do so is a convention recorded
// in DIRECTORY.md, which this function does not enforce and cannot.
//
// store may be a transaction handle, in which case the entry commits with
// whatever else that transaction carries.
//
// This is not production quality and is not trying to be. The validation a
// real implementation would need is deliberately absent, and deliberately not
// specified here either: writing that specification now would fix decisions
// the proof of concept has not earned. Callers are otherwise trusted to hand
// it well formed entries.
//
// The types are the one exception, and are checked because they are not
// really input validation. A type is the stand in for a foreign key, so a bad
// one severs the entry from the thing it is about, and nothing downstream can
// detect that or recover from it.
//
// Nothing here checks that the actor differs from the subject. An entity
// acting on itself is ordinary, as when a user deletes their own account, and
// the entry records the actor that acted. The rule that an entity may not
// write entries about itself is about authorship rather than about the actor,
// and it is enforced by authorization: appending requires system permission,
// which an entity's own credential does not carry.
func AppendEntry(ctx context.Context, store database.Store, entry Entry) (database.EntityJournal, error) {
	if !entry.Subject.Type.Valid() {
		return database.EntityJournal{}, xerrors.Errorf("subject type %q names no kind of entity", entry.Subject.Type)
	}
	if !entry.Actor.Type.Valid() {
		return database.EntityJournal{}, xerrors.Errorf("actor type %q names no kind of entity", entry.Actor.Type)
	}

	return store.InsertEntityJournalEntry(ctx, database.InsertEntityJournalEntryParams{
		RecordedAt:  dbtime.Now(),
		Event:       string(entry.Event),
		SubjectType: string(entry.Subject.Type),
		Subject:     entry.Subject.ID,
		ActorType:   string(entry.Actor.Type),
		Actor:       entry.Actor.ID,
	})
}
