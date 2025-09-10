// Package agentconnmock contains a mock implementation of workspacesdk.AgentConn for use in tests.
package agentconnmock

//go:generate mockgen -destination ./agentconnmock.go -package agentconnmock .. AgentConn
