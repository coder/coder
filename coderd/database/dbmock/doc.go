// package dbmock contains a mocked implementation of the database.Store interface for use in tests
package dbmock

//go:generate mockgen -destination ./store.go -package dbmock github.com/coder/coder/coderd/database Store
