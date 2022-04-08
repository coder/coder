package session

import (
	"github.com/coder/coder/coderd/database"
)

// ActorType is an enum of all types of Actors.
type ActorType string

// ActorTypes.
const (
	ActorTypeAnonymous ActorType = "anonymous"
	ActorTypeUser      ActorType = "user"
)

// Actor represents an unauthenticated or authenticated client accessing the
// API. To check authorization, callers should call pass the Actor into the
// authz package to assert access.
type Actor interface {
	Type() ActorType
	// ID is the unique ID of the actor for logging purposes.
	ID() string
	// Name is a friendly, but consistent, name for the actor for logging
	// purposes. E.g. "deansheather"
	Name() string

	// TODO: Steven - RBAC methods
}

// AnonymousActor represents an unauthenticated API client.
type AnonymousActor interface {
	Actor
	Anonymous()
}

// UserActor represents an authenticated user actor. Any consumers that wish to
// check if the actor is a user (and access user fields such as User.ID) can
// do a checked type cast from Actor to UserActor.
type UserActor interface {
	Actor
	User() *database.User
}
