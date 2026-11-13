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
	agentRelayParameterSessionID        = "agent_relay_session_id"
	agentRelayParameterPool             = "agent_relay_pool"
	agentRelayParameterAttempt          = "agent_relay_attempt"
	agentRelayParameterClientPlatform   = "agent_relay_client_platform"
	agentRelayParameterClaudeCodePrefix = "agent_relay_claude_code_"
	agentRelayParameterCursorPrefix     = "agent_relay_cursor_"
)

// agentRelayClientPlatformMaxLength caps the reported client platform. The
// value is user-settable, so it is normalized before it leaves the deployment.
const agentRelayClientPlatformMaxLength = 15

// AgentRelayProvider identifies whose provider-specific parameters a build
// carries.
type AgentRelayProvider string

const (
	AgentRelayProviderClaudeCode AgentRelayProvider = "claude_code"
	AgentRelayProviderCursor     AgentRelayProvider = "cursor"
	// AgentRelayProviderUnknown is reported when a build carries no
	// provider-specific parameters.
	AgentRelayProviderUnknown AgentRelayProvider = "unknown"
	// AgentRelayProviderAmbiguous is reported when a build carries
	// parameters of more than one provider.
	AgentRelayProviderAmbiguous AgentRelayProvider = "ambiguous"
)

// WorkspaceBuildAgentRelay is derived from the agent_relay_* parameters.
type WorkspaceBuildAgentRelay struct {
	Provider AgentRelayProvider `json:"provider"`
	// ClientPlatform is the surface that started the session, such as
	// claude_in_slack. Only the Claude Code provider reports it. It is
	// lowercased, characters outside [a-z0-9_] become underscores, and it is
	// cut to 15 characters.
	ClientPlatform string `json:"client_platform,omitempty"`
	// SessionIDHash is the SHA-256 of the relay session ID. Builds of the
	// same session share it.
	SessionIDHash string `json:"session_id_hash"`
	// PoolHash is the unsalted SHA-256 of the relay pool name. It is for
	// counting distinct pools; it does not hide common names.
	PoolHash string `json:"pool_hash,omitempty"`
	// Attempt is the dispatch attempt number, nil when the parameter is
	// missing or not an integer.
	Attempt *int `json:"attempt,omitempty"`
}

// agentRelayBuildParameters only looks up start builds: stop and delete
// builds of a relay workspace carry the same parameters, but they are not
// dispatches and get no AgentRelay field. Only the parameters the converter
// reads are loaded, so other agent_relay_* values, such as credentials,
// never reach the reporter.
func agentRelayBuildParameters(ctx context.Context, db database.Store, builds []database.WorkspaceBuild) (map[uuid.UUID]map[string]string, error) {
	paramsByBuild := make(map[uuid.UUID]map[string]string)
	buildIDs := make([]uuid.UUID, 0, len(builds))
	for _, build := range builds {
		if build.Transition != database.WorkspaceTransitionStart {
			continue
		}
		buildIDs = append(buildIDs, build.ID)
	}
	if len(buildIDs) == 0 {
		return paramsByBuild, nil
	}
	params, err := db.GetWorkspaceBuildParametersByBuildIDsAndNames(ctx, database.GetWorkspaceBuildParametersByBuildIDsAndNamesParams{
		WorkspaceBuildIds: buildIDs,
		Names: []string{
			agentRelayParameterSessionID,
			agentRelayParameterPool,
			agentRelayParameterAttempt,
			agentRelayParameterClientPlatform,
		},
		NamePrefixes: []string{
			agentRelayParameterClaudeCodePrefix,
			agentRelayParameterCursorPrefix,
		},
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

// convertWorkspaceBuildAgentRelay returns nil unless the build has a
// non-empty relay session ID. The pool is hashed because it is user content,
// and the session ID because it only needs to be counted.
func convertWorkspaceBuildAgentRelay(parameters map[string]string) *WorkspaceBuildAgentRelay {
	sessionID, ok := parameters[agentRelayParameterSessionID]
	if !ok || sessionID == "" {
		return nil
	}
	relay := &WorkspaceBuildAgentRelay{
		Provider:       agentRelayProvider(parameters),
		ClientPlatform: normalizeAgentRelayClientPlatform(parameters[agentRelayParameterClientPlatform]),
		SessionIDHash:  fmt.Sprintf("%x", sha256.Sum256([]byte(sessionID))),
	}
	if pool := parameters[agentRelayParameterPool]; pool != "" {
		relay.PoolHash = fmt.Sprintf("%x", sha256.Sum256([]byte(pool)))
	}
	if attempt, err := strconv.Atoi(parameters[agentRelayParameterAttempt]); err == nil {
		relay.Attempt = &attempt
	}
	return relay
}

func normalizeAgentRelayClientPlatform(platform string) string {
	platform = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_':
			return r
		default:
			return '_'
		}
	}, strings.ToLower(platform))
	if len(platform) > agentRelayClientPlatformMaxLength {
		platform = platform[:agentRelayClientPlatformMaxLength]
	}
	return platform
}

// agentRelayProvider only looks at names: coderd stores a row for every
// parameter the template declares, so ephemeral provider parameters stay
// present, with their default value, on later builds.
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
