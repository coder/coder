// Package agentproc contains logic for interfacing with local
// processes running in the same context as the agent.
package agentproc

//go:generate mockgen -destination ./syscallermock_test.go -package agentproc_test github.com/coder/coder/v2/agent/agentproc Syscaller
