package session

import (
	"github.com/coder/coder/coderd/database"
)

// ActorType is an enum of all types of Actors.
type ActorType string

// ActorTypes.
const (
	ActorTypeUser ActorType = "user"
	// TODO: Dean - WorkspaceActor and SatelliteActor
)

// Actor represents an unauthenticated or authenticated client accessing the
// API. To check authorization, callers should call pass the Actor into the
// authz package to assert access.
type Actor interface {
	// Type is the type of actor as an enum. This method exists rather than
	// switching on `actor.(type)` because doing a type switch is ~63% slower
	// according to a benchmark that Dean made. This performance difference adds
	// up over time because we will call this method on most requests.
	Type() ActorType
	// ID is the unique ID of the actor for logging purposes.
	ID() string
	// Name is a friendly, but consistent, name for the actor for logging
	// purposes. E.g. "deansheather"
	Name() string

	// TODO: Steven - RBAC methods
}

// UserActor represents an authenticated user actor. Any consumers that wish to
// check if the actor is a user (and access user fields such as User.ID) can
// do a checked type cast from Actor to UserActor.
type UserActor interface {
	Actor
	User() *database.User
	APIKey() *database.APIKey
}
