// Package agentexecmock contains a mock implementation of agentexec.Execer for use in tests.
package agentexecmock

//go:generate mockgen -destination ./agentexecmock.go -package agentexecmock .. Execer
