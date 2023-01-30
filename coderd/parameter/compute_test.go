package parameter_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/databasefake"
	"github.com/coder/coder/coderd/parameter"
	"github.com/coder/coder/cryptorand"
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
		name, err := cryptorand.String(8)
		require.NoError(t, err)
		sourceValue, err := cryptorand.String(8)
		require.NoError(t, err)
		param, err := db.InsertParameterSchema(context.Background(), database.InsertParameterSchemaParams{
			ID:                       uuid.New(),
			Name:                     name,
			JobID:                    opts.TemplateImportJobID,
			DefaultSourceScheme:      database.ParameterSourceSchemeData,
			DefaultSourceValue:       sourceValue,
			AllowOverrideSource:      opts.AllowOverrideSource,
			AllowOverrideDestination: opts.AllowOverrideDestination,
			DefaultDestinationScheme: opts.DefaultDestinationScheme,
			ValidationTypeSystem:     database.ParameterTypeSystemNone,
		})
		require.NoError(t, err)
		return param
	}

	t.Run("NoValue", func(t *testing.T) {
		t.Parallel()
		db := databasefake.New()
		scope := generateScope()
		_, err := db.InsertParameterSchema(context.Background(), database.InsertParameterSchemaParams{
			ID:                       uuid.New(),
			JobID:                    scope.TemplateImportJobID,
			Name:                     "hey",
			DefaultSourceScheme:      database.ParameterSourceSchemeNone,
			DefaultDestinationScheme: database.ParameterDestinationSchemeNone,
			ValidationTypeSystem:     database.ParameterTypeSystemNone,
		})
		require.NoError(t, err)
		computed, err := parameter.Compute(context.Background(), db, scope, nil)
		require.NoError(t, err)
		require.Len(t, computed, 0)
	})

	t.Run("UseDefaultTemplateValue", func(t *testing.T) {
		t.Parallel()
		db := databasefake.New()
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
		db := databasefake.New()
		scope := generateScope()
		parameterSchema := generateParameter(t, db, parameterOptions{
			TemplateImportJobID: scope.TemplateImportJobID,
		})
		value, err := db.InsertParameterValue(context.Background(), database.InsertParameterValueParams{
			ID:                uuid.New(),
			Name:              parameterSchema.Name,
			Scope:             database.ParameterScopeTemplate,
			ScopeID:           scope.TemplateID.UUID,
			SourceScheme:      database.ParameterSourceSchemeData,
			SourceValue:       "nop",
			DestinationScheme: database.ParameterDestinationSchemeEnvironmentVariable,
		})
		require.NoError(t, err)

		computed, err := parameter.Compute(context.Background(), db, scope, nil)
		require.NoError(t, err)
		require.Len(t, computed, 1)
		require.Equal(t, false, computed[0].DefaultSourceValue)
		require.Equal(t, value.SourceValue, computed[0].SourceValue)
	})

	t.Run("WorkspaceCannotOverwriteTemplateDefault", func(t *testing.T) {
		t.Parallel()
		db := databasefake.New()
		scope := generateScope()
		parameterSchema := generateParameter(t, db, parameterOptions{
			TemplateImportJobID: scope.TemplateImportJobID,
		})

		_, err := db.InsertParameterValue(context.Background(), database.InsertParameterValueParams{
			ID:                uuid.New(),
			Name:              parameterSchema.Name,
			Scope:             database.ParameterScopeWorkspace,
			ScopeID:           scope.WorkspaceID.UUID,
			SourceScheme:      database.ParameterSourceSchemeData,
			SourceValue:       "nop",
			DestinationScheme: database.ParameterDestinationSchemeEnvironmentVariable,
		})
		require.NoError(t, err)

		computed, err := parameter.Compute(context.Background(), db, scope, nil)
		require.NoError(t, err)
		require.Len(t, computed, 1)
		require.Equal(t, true, computed[0].DefaultSourceValue)
	})

	t.Run("WorkspaceOverwriteTemplateDefault", func(t *testing.T) {
		t.Parallel()
		db := databasefake.New()
		scope := generateScope()
		parameterSchema := generateParameter(t, db, parameterOptions{
			AllowOverrideSource: true,
			TemplateImportJobID: scope.TemplateImportJobID,
		})
		_, err := db.InsertParameterValue(context.Background(), database.InsertParameterValueParams{
			ID:                uuid.New(),
			Name:              parameterSchema.Name,
			Scope:             database.ParameterScopeWorkspace,
			ScopeID:           scope.WorkspaceID.UUID,
			SourceScheme:      database.ParameterSourceSchemeData,
			SourceValue:       "nop",
			DestinationScheme: database.ParameterDestinationSchemeEnvironmentVariable,
		})
		require.NoError(t, err)

		computed, err := parameter.Compute(context.Background(), db, scope, nil)
		require.NoError(t, err)
		require.Len(t, computed, 1)
		require.Equal(t, false, computed[0].DefaultSourceValue)
	})

	t.Run("HideRedisplay", func(t *testing.T) {
		t.Parallel()
		db := databasefake.New()
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
