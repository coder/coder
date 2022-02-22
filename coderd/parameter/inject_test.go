package parameter_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/parameter"
	"github.com/coder/coder/database"
	"github.com/coder/coder/database/databasefake"
)

func TestInject(t *testing.T) {
	t.Parallel()
	t.Run("Unknown", func(t *testing.T) {
		db := databasefake.New()
		provisionJobID := uuid.New()
		err := parameter.Inject(context.Background(), db, parameter.InjectOptions{
			ParameterSchemas: []database.ParameterSchema{{
				ID:    uuid.New(),
				Name:  fmt.Sprintf("%s_notexist", parameter.Namespace),
				JobID: provisionJobID,
			}},
		})
		require.Error(t, err)
	})

	t.Run("Username", func(t *testing.T) {
		db := databasefake.New()
		provisionJobID := uuid.New()
		err := parameter.Inject(context.Background(), db, parameter.InjectOptions{
			ParameterSchemas: []database.ParameterSchema{{
				ID:    uuid.New(),
				Name:  parameter.Username,
				JobID: provisionJobID,
			}},
			ProvisionJobID: provisionJobID,
			Username:       "kyle",
		})
		require.NoError(t, err)
		values, err := db.GetParameterValuesByScope(context.Background(), database.GetParameterValuesByScopeParams{
			Scope:   database.ParameterScopeProvisionerJob,
			ScopeID: provisionJobID.String(),
		})
		require.NoError(t, err)
		require.Len(t, values, 1)
		require.Equal(t, values[0].SourceValue, "kyle")
	})

	t.Run("AgentToken", func(t *testing.T) {
		db := databasefake.New()
		provisionJobID := uuid.New()
		err := parameter.Inject(context.Background(), db, parameter.InjectOptions{
			ParameterSchemas: []database.ParameterSchema{{
				ID:    uuid.New(),
				Name:  parameter.AgentTokenPrefix,
				JobID: provisionJobID,
			}},
			ProvisionJobID: provisionJobID,
		})
		require.NoError(t, err)
		values, err := db.GetParameterValuesByScope(context.Background(), database.GetParameterValuesByScopeParams{
			Scope:   database.ParameterScopeProvisionerJob,
			ScopeID: provisionJobID.String(),
		})
		require.NoError(t, err)
		require.Len(t, values, 1)
	})

	t.Run("WorkspaceTransition", func(t *testing.T) {
		db := databasefake.New()
		provisionJobID := uuid.New()
		err := parameter.Inject(context.Background(), db, parameter.InjectOptions{
			ParameterSchemas: []database.ParameterSchema{{
				ID:    uuid.New(),
				Name:  parameter.WorkspaceTransition,
				JobID: provisionJobID,
			}},
			ProvisionJobID: provisionJobID,
			Transition:     database.WorkspaceTransitionStop,
		})
		require.NoError(t, err)
		values, err := db.GetParameterValuesByScope(context.Background(), database.GetParameterValuesByScopeParams{
			Scope:   database.ParameterScopeProvisionerJob,
			ScopeID: provisionJobID.String(),
		})
		require.NoError(t, err)
		require.Len(t, values, 1)
		require.Equal(t, string(database.WorkspaceTransitionStop), values[0].SourceValue)
	})
}
