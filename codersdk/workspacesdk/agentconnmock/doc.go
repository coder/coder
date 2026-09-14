// Package agentconnmock contains a mock implementation of workspacesdk.AgentConn for use in tests.
package agentconnmock

//go:generate go tool mockgen -destination ./agentconnmock.go -package agentconnmock .. AgentConn
