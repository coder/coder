// Package entity owns the lifecycle of entities, the identities and
// credentials issued to them, and the journal accounting for both.
//
// See DIRECTORY.md in this directory for what belongs here and why, and
// poc_audit/audit_approach.md for the approach the journal implements.
package entity

import "github.com/google/uuid"

// Type names a kind of entity that can act, and the values together are the
// actor type set. What defines the set is the capacity to act; that it is
// closed is a property of it rather than its name.
//
// An actor is named by a (type, identifier) pair, because an identifier alone
// cannot say which identity table holds it and SQL cannot declare a key into a
// union of those tables. The type carries what the schema cannot. A subject
// needs no such pair, one journal per entity meaning the table already says
// what kind its subjects are.
//
// A value outside the set names no table, so an entry carrying one names an
// actor nothing can resolve, which severs the journal's link to the party
// responsible. The lifecycle functions refuse them.
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

// Event names a persistent state change. Each machine qualifies its own
// constants, since revoke and lapse each name a transition in two of them and a
// shared constant would assert that two transitions are the same kind of event.
type Event string
