package dbauthz

import "github.com/coder/coder/coderd/database"

// AuthzStore is the interface for the Authz querier. It will track closely
// to database.Store, but not 1:1 as not all database.Store functions will be
// exposed.
type AuthzStore interface {
	// TODO: @emyrk be selective about which functions are exposed.
	database.Store
}
