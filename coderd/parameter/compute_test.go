package parameter_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbfake"
	"github.com/coder/coder/coderd/database/dbgen"
	"github.com/coder/coder/coderd/parameter"
)

func TestCompute(t *testing.T) {
	t.Parallel()
	generateScope := func() parameter.ComputeScope {
		return parameter.ComputeScope{
			TemplateImportJobID: uuid.New(),
			TemplateID: uuid.NullUUID{
				UUID:  uuid.New(),
				Valid: true,
			},
			WorkspaceID: uuid.NullUUID{
				UUID:  uuid.New(),
				Valid: true,
			},
		}
	}
	type parameterOptions struct {
		AllowOverrideSource      bool
		AllowOverrideDestination bool
		DefaultDestinationScheme database.ParameterDestinationScheme
		TemplateImportJobID      uuid.UUID
	}
	generateParameter := func(t *testing.T, db database.Store, opts parameterOptions) database.ParameterSchema {
		if opts.DefaultDestinationScheme == "" {
			opts.DefaultDestinationScheme = database.ParameterDestinationSchemeEnvironmentVariable
		}

		param := dbgen.ParameterSchema(t, db, database.ParameterSchema{
			JobID:                    opts.TemplateImportJobID,
			DefaultSourceScheme:      database.ParameterSourceSchemeData,
			AllowOverrideSource:      opts.AllowOverrideSource,
			AllowOverrideDestination: opts.AllowOverrideDestination,
			DefaultDestinationScheme: opts.DefaultDestinationScheme,
			ValidationTypeSystem:     database.ParameterTypeSystemNone,
		})

		return param
	}

	t.Run("NoValue", func(t *testing.T) {
		t.Parallel()
		db := dbfake.New()
		scope := generateScope()
		_ = dbgen.ParameterSchema(t, db, database.ParameterSchema{
			JobID:                    scope.TemplateImportJobID,
			DefaultSourceScheme:      database.ParameterSourceSchemeNone,
			DefaultDestinationScheme: database.ParameterDestinationSchemeNone,
			ValidationTypeSystem:     database.ParameterTypeSystemNone,
		})
		computed, err := parameter.Compute(context.Background(), db, scope, nil)
		require.NoError(t, err)
		require.Len(t, computed, 0)
	})

	t.Run("UseDefaultTemplateValue", func(t *testing.T) {
		t.Parallel()
		db := dbfake.New()
		scope := generateScope()
		parameterSchema := generateParameter(t, db, parameterOptions{
			TemplateImportJobID:      scope.TemplateImportJobID,
			DefaultDestinationScheme: database.ParameterDestinationSchemeProvisionerVariable,
		})
		computed, err := parameter.Compute(context.Background(), db, scope, nil)
		require.NoError(t, err)
		require.Len(t, computed, 1)
		computedValue := computed[0]
		require.True(t, computedValue.DefaultSourceValue)
		require.Equal(t, database.ParameterScopeImportJob, computedValue.Scope)
		require.Equal(t, scope.TemplateImportJobID, computedValue.ScopeID)
		require.Equal(t, computedValue.SourceValue, parameterSchema.DefaultSourceValue)
	})

	t.Run("TemplateOverridesTemplateDefault", func(t *testing.T) {
		t.Parallel()
		db := dbfake.New()
		scope := generateScope()
		parameterSchema := generateParameter(t, db, parameterOptions{
			TemplateImportJobID: scope.TemplateImportJobID,
		})
		value := dbgen.ParameterValue(t, db, database.ParameterValue{
			Name:              parameterSchema.Name,
			Scope:             database.ParameterScopeTemplate,
			ScopeID:           scope.TemplateID.UUID,
			SourceScheme:      database.ParameterSourceSchemeData,
			SourceValue:       "nop",
			DestinationScheme: database.ParameterDestinationSchemeEnvironmentVariable,
		})

		computed, err := parameter.Compute(context.Background(), db, scope, nil)
		require.NoError(t, err)
		require.Len(t, computed, 1)
		require.Equal(t, false, computed[0].DefaultSourceValue)
		require.Equal(t, value.SourceValue, computed[0].SourceValue)
	})

	t.Run("WorkspaceCannotOverwriteTemplateDefault", func(t *testing.T) {
		t.Parallel()
		db := dbfake.New()
		scope := generateScope()
		parameterSchema := generateParameter(t, db, parameterOptions{
			TemplateImportJobID: scope.TemplateImportJobID,
		})

		_ = dbgen.ParameterValue(t, db, database.ParameterValue{
			Name:              parameterSchema.Name,
			Scope:             database.ParameterScopeWorkspace,
			ScopeID:           scope.WorkspaceID.UUID,
			SourceScheme:      database.ParameterSourceSchemeData,
			SourceValue:       "nop",
			DestinationScheme: database.ParameterDestinationSchemeEnvironmentVariable,
		})

		computed, err := parameter.Compute(context.Background(), db, scope, nil)
		require.NoError(t, err)
		require.Len(t, computed, 1)
		require.Equal(t, true, computed[0].DefaultSourceValue)
	})

	t.Run("WorkspaceOverwriteTemplateDefault", func(t *testing.T) {
		t.Parallel()
		db := dbfake.New()
		scope := generateScope()
		parameterSchema := generateParameter(t, db, parameterOptions{
			AllowOverrideSource: true,
			TemplateImportJobID: scope.TemplateImportJobID,
		})
		_ = dbgen.ParameterValue(t, db, database.ParameterValue{
			Name:              parameterSchema.Name,
			Scope:             database.ParameterScopeWorkspace,
			ScopeID:           scope.WorkspaceID.UUID,
			SourceScheme:      database.ParameterSourceSchemeData,
			SourceValue:       "nop",
			DestinationScheme: database.ParameterDestinationSchemeEnvironmentVariable,
		})

		computed, err := parameter.Compute(context.Background(), db, scope, nil)
		require.NoError(t, err)
		require.Len(t, computed, 1)
		require.Equal(t, false, computed[0].DefaultSourceValue)
	})

	t.Run("HideRedisplay", func(t *testing.T) {
		t.Parallel()
		db := dbfake.New()
		scope := generateScope()
		_ = generateParameter(t, db, parameterOptions{
			TemplateImportJobID:      scope.TemplateImportJobID,
			DefaultDestinationScheme: database.ParameterDestinationSchemeProvisionerVariable,
		})
		computed, err := parameter.Compute(context.Background(), db, scope, &parameter.ComputeOptions{
			HideRedisplayValues: true,
		})
		require.NoError(t, err)
		require.Len(t, computed, 1)
		computedValue := computed[0]
		require.True(t, computedValue.DefaultSourceValue)
		require.Equal(t, computedValue.SourceValue, "")
	})
}
