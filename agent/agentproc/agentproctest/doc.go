// Package agentproctest contains utility functions
// for testing process management in the agent.
package agentproctest

//go:generate mockgen -destination ./syscallermock.go -package agentproctest github.com/coder/coder/v2/agent/agentproc Syscaller
