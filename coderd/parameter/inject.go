package parameter

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/database"
)

const (
	// Namespace is a banned prefix for user-specifiable parameters.
	Namespace = "coder"

	// Username injects the owner of the workspace at provision.
	Username = "coder_username"
	// WorkspaceTransition represents the moving state of a workspace.
	WorkspaceTransition = "coder_workspace_transition"

	agentTokenPrefix = "coder_agent_token"
)

type InjectOptions struct {
	ParameterSchemas []database.ParameterSchema
	ProvisionJobID   uuid.UUID

	Username   string
	Transition database.WorkspaceTransition
}

// AgentToken returns the name of a token parameter.
func AgentToken(resourceType, resourceName string) string {
	return strings.Join([]string{agentTokenPrefix, resourceType, resourceName}, "_")
}

// HasAgentToken returns whether an agent token is specified in an array of parameters.
func HasAgentToken(parameterSchemas []database.ParameterSchema, resourceType, resourceName string) bool {
	for _, parameterSchema := range parameterSchemas {
		if parameterSchema.Name == AgentToken(resourceType, resourceName) {
			return true
		}
	}
	return false
}

// FindAgentToken returns an agent token from an array of parameter values.
func FindAgentToken(parameterValues []database.ParameterValue, resourceType, resourceName string) (string, bool) {
	for _, parameterValue := range parameterValues {
		if parameterValue.Name == AgentToken(resourceType, resourceName) {
			return parameterValue.SourceValue, true
		}
	}
	return "", false
}

// Inject adds "coder*" parameters to a job.
func Inject(ctx context.Context, db database.Store, options InjectOptions) error {
	insertParameter := func(db database.Store, name, value string) error {
		_, err := db.InsertParameterValue(ctx, database.InsertParameterValueParams{
			ID:                uuid.New(),
			Name:              name,
			CreatedAt:         database.Now(),
			UpdatedAt:         database.Now(),
			Scope:             database.ParameterScopeProvisionerJob,
			ScopeID:           options.ProvisionJobID.String(),
			SourceScheme:      database.ParameterSourceSchemeData,
			SourceValue:       value,
			DestinationScheme: database.ParameterDestinationSchemeProvisionerVariable,
		})
		return err
	}
	return db.InTx(func(db database.Store) error {
		for _, schema := range options.ParameterSchemas {
			if !strings.HasPrefix(schema.Name, Namespace) {
				continue
			}
			var err error
			switch {
			case schema.Name == Username:
				err = insertParameter(db, schema.Name, options.Username)
			case schema.Name == WorkspaceTransition:
				err = insertParameter(db, schema.Name, string(options.Transition))
			case strings.HasPrefix(schema.Name, agentTokenPrefix):
				// Generate an agent token here!
				err = insertParameter(db, schema.Name, uuid.NewString())
			default:
				return xerrors.Errorf("unrecognized namespaced parameter: %q", schema.Name)
			}
			if err != nil {
				return xerrors.Errorf("insert parameter %q: %w", schema.Name, err)
			}
		}
		return nil
	})
}
