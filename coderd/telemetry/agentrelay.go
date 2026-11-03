package telemetry

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
)

// Rich parameter names stamped by Agent Relay (github.com/coder/agent-relay)
// on every build it dispatches. The names are a contract owned by
// agent-relay's template contract and the agent-relay registry modules.
// Renaming one there silently drops it from telemetry here.
const (
	agentRelayParameterPrefix           = "agent_relay_"
	agentRelayParameterSessionID        = "agent_relay_session_id"
	agentRelayParameterPool             = "agent_relay_pool"
	agentRelayParameterAttempt          = "agent_relay_attempt"
	agentRelayParameterClientPlatform   = "agent_relay_client_platform"
	agentRelayParameterClaudeCodePrefix = "agent_relay_claude_code_"
	agentRelayParameterCursorPrefix     = "agent_relay_cursor_"
)

// AgentRelayProvider is the Agent Relay provider that dispatched a build.
type AgentRelayProvider string

// Agent Relay providers reported in WorkspaceBuildAgentRelay.Provider.
const (
	AgentRelayProviderClaudeCode AgentRelayProvider = "claude_code"
	AgentRelayProviderCursor     AgentRelayProvider = "cursor"
	AgentRelayProviderUnknown    AgentRelayProvider = "unknown"
	// AgentRelayProviderAmbiguous is reported when a build carries
	// parameters of more than one provider.
	AgentRelayProviderAmbiguous AgentRelayProvider = "ambiguous"
)

// WorkspaceBuildAgentRelay describes a build dispatched by Agent Relay,
// derived from the agent_relay_* rich parameters it stamps on the build.
type WorkspaceBuildAgentRelay struct {
	// Provider is claude_code, cursor, ambiguous, or unknown.
	Provider AgentRelayProvider `json:"provider"`
	// ClientPlatform is the surface that started the session, such as
	// claude_in_slack. Only the Claude Code provider reports it.
	ClientPlatform string `json:"client_platform,omitempty"`
	// SessionIDHash is the SHA-256 of the relay session ID. Builds of the
	// same session share it.
	SessionIDHash string `json:"session_id_hash"`
	// PoolHash is the SHA-256 of the relay pool name.
	PoolHash string `json:"pool_hash,omitempty"`
	// Attempt is the dispatch attempt number, 0 when not reported.
	Attempt int `json:"attempt,omitempty"`
}

// agentRelayBuildParameters returns the Agent Relay parameters of the given
// builds, keyed by build ID and then parameter name, in a single query.
func agentRelayBuildParameters(ctx context.Context, db database.Store, builds []database.WorkspaceBuild) (map[uuid.UUID]map[string]string, error) {
	paramsByBuild := make(map[uuid.UUID]map[string]string)
	if len(builds) == 0 {
		return paramsByBuild, nil
	}
	buildIDs := make([]uuid.UUID, 0, len(builds))
	for _, build := range builds {
		buildIDs = append(buildIDs, build.ID)
	}
	params, err := db.GetWorkspaceBuildParametersByBuildIDsAndNamePrefix(ctx, database.GetWorkspaceBuildParametersByBuildIDsAndNamePrefixParams{
		WorkspaceBuildIds: buildIDs,
		NamePrefix:        agentRelayParameterPrefix,
	})
	if err != nil {
		return nil, xerrors.Errorf("get agent relay workspace build parameters: %w", err)
	}
	for _, param := range params {
		if paramsByBuild[param.WorkspaceBuildID] == nil {
			paramsByBuild[param.WorkspaceBuildID] = make(map[string]string)
		}
		paramsByBuild[param.WorkspaceBuildID][param.Name] = param.Value
	}
	return paramsByBuild, nil
}

// convertWorkspaceBuildAgentRelay returns nil unless the parameters mark the
// build as dispatched by Agent Relay. Session ID and pool are hashed because
// the pool name is user content and the session ID only needs to be counted.
func convertWorkspaceBuildAgentRelay(parameters map[string]string) *WorkspaceBuildAgentRelay {
	sessionID, ok := parameters[agentRelayParameterSessionID]
	if !ok || sessionID == "" {
		return nil
	}
	relay := &WorkspaceBuildAgentRelay{
		Provider:       agentRelayProvider(parameters),
		ClientPlatform: parameters[agentRelayParameterClientPlatform],
		SessionIDHash:  fmt.Sprintf("%x", sha256.Sum256([]byte(sessionID))),
	}
	if pool := parameters[agentRelayParameterPool]; pool != "" {
		relay.PoolHash = fmt.Sprintf("%x", sha256.Sum256([]byte(pool)))
	}
	if attempt, err := strconv.Atoi(parameters[agentRelayParameterAttempt]); err == nil {
		relay.Attempt = attempt
	}
	return relay
}

// agentRelayProvider reports which provider's parameters the build carries.
// Builds without provider parameters are unknown: the Claude Code provider's
// required parameter is ephemeral, so its stop and restart builds lack it.
func agentRelayProvider(parameters map[string]string) AgentRelayProvider {
	var claudeCode, cursor bool
	for name := range parameters {
		switch {
		case strings.HasPrefix(name, agentRelayParameterClaudeCodePrefix):
			claudeCode = true
		case strings.HasPrefix(name, agentRelayParameterCursorPrefix):
			cursor = true
		}
	}
	switch {
	case claudeCode && cursor:
		return AgentRelayProviderAmbiguous
	case claudeCode:
		return AgentRelayProviderClaudeCode
	case cursor:
		return AgentRelayProviderCursor
	default:
		return AgentRelayProviderUnknown
	}
}
