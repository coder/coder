// package mock contains a mocked implementation of the database.Store interface for use in tests
package mock

//go:generate mockgen -destination ./store.go -package mock github.com/coder/coder/coderd/database Store
