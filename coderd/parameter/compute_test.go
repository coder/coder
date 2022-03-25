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
			ProjectImportJobID: uuid.New(),
			OrganizationID:     uuid.NewString(),
			ProjectID: uuid.NullUUID{
				UUID:  uuid.New(),
				Valid: true,
			},
			WorkspaceID: uuid.NullUUID{
				UUID:  uuid.New(),
				Valid: true,
			},
			UserID: uuid.NewString(),
		}
	}
	type parameterOptions struct {
		AllowOverrideSource      bool
		AllowOverrideDestination bool
		DefaultDestinationScheme database.ParameterDestinationScheme
		ProjectImportJobID       uuid.UUID
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
			JobID:                    opts.ProjectImportJobID,
			DefaultSourceScheme:      database.ParameterSourceSchemeData,
			DefaultSourceValue:       sourceValue,
			AllowOverrideSource:      opts.AllowOverrideSource,
			AllowOverrideDestination: opts.AllowOverrideDestination,
			DefaultDestinationScheme: opts.DefaultDestinationScheme,
		})
		require.NoError(t, err)
		return param
	}

	t.Run("NoValue", func(t *testing.T) {
		t.Parallel()
		db := databasefake.New()
		scope := generateScope()
		_, err := db.InsertParameterSchema(context.Background(), database.InsertParameterSchemaParams{
			ID:                  uuid.New(),
			JobID:               scope.ProjectImportJobID,
			Name:                "hey",
			DefaultSourceScheme: database.ParameterSourceSchemeNone,
		})
		require.NoError(t, err)
		computed, err := parameter.Compute(context.Background(), db, scope, nil)
		require.NoError(t, err)
		require.Len(t, computed, 0)
	})

	t.Run("UseDefaultProjectValue", func(t *testing.T) {
		t.Parallel()
		db := databasefake.New()
		scope := generateScope()
		parameterSchema := generateParameter(t, db, parameterOptions{
			ProjectImportJobID:       scope.ProjectImportJobID,
			DefaultDestinationScheme: database.ParameterDestinationSchemeProvisionerVariable,
		})
		computed, err := parameter.Compute(context.Background(), db, scope, nil)
		require.NoError(t, err)
		require.Len(t, computed, 1)
		computedValue := computed[0]
		require.True(t, computedValue.DefaultSourceValue)
		require.Equal(t, database.ParameterScopeImportJob, computedValue.Scope)
		require.Equal(t, scope.ProjectImportJobID.String(), computedValue.ScopeID)
		require.Equal(t, computedValue.SourceValue, parameterSchema.DefaultSourceValue)
	})

	t.Run("OverrideOrganizationWithImportJob", func(t *testing.T) {
		t.Parallel()
		db := databasefake.New()
		scope := generateScope()
		parameterSchema := generateParameter(t, db, parameterOptions{
			ProjectImportJobID: scope.ProjectImportJobID,
		})
		_, err := db.InsertParameterValue(context.Background(), database.InsertParameterValueParams{
			ID:                uuid.New(),
			Name:              parameterSchema.Name,
			Scope:             database.ParameterScopeOrganization,
			ScopeID:           scope.OrganizationID,
			SourceScheme:      database.ParameterSourceSchemeData,
			SourceValue:       "firstnop",
			DestinationScheme: database.ParameterDestinationSchemeEnvironmentVariable,
		})
		require.NoError(t, err)

		value, err := db.InsertParameterValue(context.Background(), database.InsertParameterValueParams{
			ID:                uuid.New(),
			Name:              parameterSchema.Name,
			Scope:             database.ParameterScopeImportJob,
			ScopeID:           scope.ProjectImportJobID.String(),
			SourceScheme:      database.ParameterSourceSchemeData,
			SourceValue:       "secondnop",
			DestinationScheme: database.ParameterDestinationSchemeEnvironmentVariable,
		})
		require.NoError(t, err)

		computed, err := parameter.Compute(context.Background(), db, scope, nil)
		require.NoError(t, err)
		require.Len(t, computed, 1)
		require.Equal(t, false, computed[0].DefaultSourceValue)
		require.Equal(t, value.SourceValue, computed[0].SourceValue)
	})

	t.Run("ProjectOverridesProjectDefault", func(t *testing.T) {
		t.Parallel()
		db := databasefake.New()
		scope := generateScope()
		parameterSchema := generateParameter(t, db, parameterOptions{
			ProjectImportJobID: scope.ProjectImportJobID,
		})
		value, err := db.InsertParameterValue(context.Background(), database.InsertParameterValueParams{
			ID:                uuid.New(),
			Name:              parameterSchema.Name,
			Scope:             database.ParameterScopeProject,
			ScopeID:           scope.ProjectID.UUID.String(),
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

	t.Run("WorkspaceCannotOverwriteProjectDefault", func(t *testing.T) {
		t.Parallel()
		db := databasefake.New()
		scope := generateScope()
		parameterSchema := generateParameter(t, db, parameterOptions{
			ProjectImportJobID: scope.ProjectImportJobID,
		})
		_, err := db.InsertParameterValue(context.Background(), database.InsertParameterValueParams{
			ID:                uuid.New(),
			Name:              parameterSchema.Name,
			Scope:             database.ParameterScopeWorkspace,
			ScopeID:           scope.WorkspaceID.UUID.String(),
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

	t.Run("WorkspaceOverwriteProjectDefault", func(t *testing.T) {
		t.Parallel()
		db := databasefake.New()
		scope := generateScope()
		parameterSchema := generateParameter(t, db, parameterOptions{
			AllowOverrideSource: true,
			ProjectImportJobID:  scope.ProjectImportJobID,
		})
		_, err := db.InsertParameterValue(context.Background(), database.InsertParameterValueParams{
			ID:                uuid.New(),
			Name:              parameterSchema.Name,
			Scope:             database.ParameterScopeWorkspace,
			ScopeID:           scope.WorkspaceID.UUID.String(),
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
			ProjectImportJobID:       scope.ProjectImportJobID,
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
