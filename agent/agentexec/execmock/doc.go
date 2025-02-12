// Package execmock contains a mock implementation of agentexec.Execer for user in tests.
package execmock

//go:generate mockgen -destination ./execmock.go -package execmock .. Execer
