package runtimeconfig

import (
	"github.com/google/uuid"
)

// Manager is the singleton that produces resolvers for runtime configuration.
// TODO: Implement caching layer.
type Manager struct{}

func NewManager() *Manager {
	return &Manager{}
}

// Resolver is the deployment wide namespace for runtime configuration.
// If you are trying to namespace a configuration, orgs for example, use
// OrganizationResolver.
func (*Manager) Resolver(db Store) Resolver {
	return NewStoreResolver(db)
}

// OrganizationResolver will namespace all runtime configuration to the provided
// organization ID. Configuration values stored with a given organization ID require
// that the organization ID be provided to retrieve the value.
// No values set here will ever be returned by the call to 'Resolver()'.
func (*Manager) OrganizationResolver(db Store, orgID uuid.UUID) Resolver {
	return OrganizationResolver(orgID, NewStoreResolver(db))
}
