// Package entity owns the lifecycle of entities, the identities and
// credentials issued to them, and the audit journal accounting for both.
//
// See DIRECTORY.md in this directory for what belongs here and why, and
// poc_audit/audit_approach.md for the approach the journal implements.
package entity

import (
	"context"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
)

// Type names the kind of entity an identifier refers to. It says which table
// holds the primary key, standing in for a foreign key into a union of the
// identity tables that SQL cannot express.
type Type string

const (
	TypeAIAgent        Type = "ai_agent"
	TypeWorkspaceAgent Type = "workspace_agent"
	TypeUser           Type = "user"
)

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
// the proof of concept has not earned. Callers are trusted to hand it well
// formed entries.
//
// Nothing here checks that the actor differs from the subject. An entity
// acting on itself is ordinary, as when a user deletes their own account, and
// the entry records the actor that acted. The rule that an entity may not
// write entries about itself is about authorship rather than about the actor,
// and it is enforced by authorization: appending requires system permission,
// which an entity's own credential does not carry.
func AppendEntry(ctx context.Context, store database.Store, entry Entry) (database.EntityJournal, error) {
	return store.InsertEntityJournalEntry(ctx, database.InsertEntityJournalEntryParams{
		RecordedAt:  dbtime.Now(),
		Event:       string(entry.Event),
		SubjectType: string(entry.Subject.Type),
		Subject:     entry.Subject.ID,
		ActorType:   string(entry.Actor.Type),
		Actor:       entry.Actor.ID,
	})
}
