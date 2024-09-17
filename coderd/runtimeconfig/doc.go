// Package runtimeconfig contains logic for managing runtime configuration values
// stored in the database. Each coderd should have a Manager singleton instance
// that can create a Resolver for runtime configuration CRUD.
//
// TODO: Implement a caching layer for the Resolver so that we don't hit the
// database on every request. Configuration values are not expected to change
// frequently, so we should use pubsub to notify for updates.
// When implemented, the runtimeconfig will essentially be an in memory lookup
// with a database for persistence.
package runtimeconfig
