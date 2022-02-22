package parameter_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/parameter"
	"github.com/coder/coder/cryptorand"
	"github.com/coder/coder/database"
	"github.com/coder/coder/database/databasefake"
)

func TestCompute(t *testing.T) {
	t.Parallel()
	generateScope := func() parameter.ComputeScope {
		return parameter.ComputeScope{
			ProvisionJobID: uuid.New(),
			OrganizationID: uuid.NewString(),
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
		Name                     string
		AllowOverrideSource      bool
		AllowOverrideDestination bool
		DefaultDestinationScheme database.ParameterDestinationScheme
		ProvisionJobID           uuid.UUID
	}
	generateParameter := func(t *testing.T, db database.Store, opts parameterOptions) database.ParameterSchema {
		if opts.DefaultDestinationScheme == "" {
			opts.DefaultDestinationScheme = database.ParameterDestinationSchemeEnvironmentVariable
		}
		name, err := cryptorand.String(8)
		require.NoError(t, err)
		if opts.Name != "" {
			name = opts.Name
		}
		sourceValue, err := cryptorand.String(8)
		require.NoError(t, err)
		return database.ParameterSchema{
			ID:                       uuid.New(),
			Name:                     name,
			JobID:                    opts.ProvisionJobID,
			DefaultSourceScheme:      database.ParameterSourceSchemeData,
			DefaultSourceValue:       sourceValue,
			AllowOverrideSource:      opts.AllowOverrideSource,
			AllowOverrideDestination: opts.AllowOverrideDestination,
			DefaultDestinationScheme: opts.DefaultDestinationScheme,
		}
	}

	t.Run("NoValue", func(t *testing.T) {
		t.Parallel()
		db := databasefake.New()
		scope := generateScope()
		parameterSchema, err := db.InsertParameterSchema(context.Background(), database.InsertParameterSchemaParams{
			ID:                  uuid.New(),
			JobID:               scope.ProvisionJobID,
			Name:                "hey",
			DefaultSourceScheme: database.ParameterSourceSchemeNone,
		})
		require.NoError(t, err)
		computed, err := parameter.Compute(context.Background(), db, parameter.ComputeOptions{
			ParameterSchemas: []database.ParameterSchema{parameterSchema},
			Scope:            scope,
		})
		require.NoError(t, err)
		require.Len(t, computed, 0)
	})

	t.Run("UseDefaultProjectValue", func(t *testing.T) {
		t.Parallel()
		db := databasefake.New()
		scope := generateScope()
		parameterSchema := generateParameter(t, db, parameterOptions{
			ProvisionJobID:           scope.ProvisionJobID,
			DefaultDestinationScheme: database.ParameterDestinationSchemeProvisionerVariable,
		})
		computed, err := parameter.Compute(context.Background(), db, parameter.ComputeOptions{
			ParameterSchemas: []database.ParameterSchema{parameterSchema},
			Scope:            scope,
		})
		require.NoError(t, err)
		require.Len(t, computed, 1)
		computedValue := computed[0]
		require.True(t, computedValue.DefaultSourceValue)
		require.Equal(t, database.ParameterScopeProvisionerJob, computedValue.Scope)
		require.Equal(t, scope.ProvisionJobID.String(), computedValue.ScopeID)
		require.Equal(t, computedValue.SourceValue, parameterSchema.DefaultSourceValue)
	})

	t.Run("OverrideOrganizationWithImportJob", func(t *testing.T) {
		t.Parallel()
		db := databasefake.New()
		scope := generateScope()
		parameterSchema := generateParameter(t, db, parameterOptions{
			ProvisionJobID: scope.ProvisionJobID,
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
			Scope:             database.ParameterScopeProvisionerJob,
			ScopeID:           scope.ProvisionJobID.String(),
			SourceScheme:      database.ParameterSourceSchemeData,
			SourceValue:       "secondnop",
			DestinationScheme: database.ParameterDestinationSchemeEnvironmentVariable,
		})
		require.NoError(t, err)

		computed, err := parameter.Compute(context.Background(), db, parameter.ComputeOptions{
			ParameterSchemas: []database.ParameterSchema{parameterSchema},
			Scope:            scope,
		})
		require.NoError(t, err)
		require.Len(t, computed, 1)
		require.Equal(t, false, computed[0].DefaultSourceValue)
		require.Equal(t, value.SourceValue, computed[0].SourceValue)
	})

	t.Run("NamespacedProvisionJob", func(t *testing.T) {
		t.Parallel()
		db := databasefake.New()
		scope := generateScope()
		parameterSchema := generateParameter(t, db, parameterOptions{
			Name:           parameter.AgentTokenPrefix,
			ProvisionJobID: scope.ProvisionJobID,
		})
		value, err := db.InsertParameterValue(context.Background(), database.InsertParameterValueParams{
			ID:                uuid.New(),
			Name:              parameterSchema.Name,
			Scope:             database.ParameterScopeProvisionerJob,
			ScopeID:           scope.ProvisionJobID.String(),
			SourceScheme:      database.ParameterSourceSchemeData,
			SourceValue:       "firstnop",
			DestinationScheme: database.ParameterDestinationSchemeProvisionerVariable,
		})
		require.NoError(t, err)
		computed, err := parameter.Compute(context.Background(), db, parameter.ComputeOptions{
			ParameterSchemas: []database.ParameterSchema{parameterSchema},
			Scope:            scope,
		})
		require.NoError(t, err)
		require.Len(t, computed, 1)
		require.Equal(t, value.SourceValue, computed[0].SourceValue)
	})

	t.Run("ProjectOverridesProjectDefault", func(t *testing.T) {
		t.Parallel()
		db := databasefake.New()
		scope := generateScope()
		parameterSchema := generateParameter(t, db, parameterOptions{
			ProvisionJobID: scope.ProvisionJobID,
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

		computed, err := parameter.Compute(context.Background(), db, parameter.ComputeOptions{
			ParameterSchemas: []database.ParameterSchema{parameterSchema},
			Scope:            scope,
		})
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
			ProvisionJobID: scope.ProvisionJobID,
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

		computed, err := parameter.Compute(context.Background(), db, parameter.ComputeOptions{
			ParameterSchemas: []database.ParameterSchema{parameterSchema},
			Scope:            scope,
		})
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
			ProvisionJobID:      scope.ProvisionJobID,
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

		computed, err := parameter.Compute(context.Background(), db, parameter.ComputeOptions{
			ParameterSchemas: []database.ParameterSchema{parameterSchema},
			Scope:            scope,
		})
		require.NoError(t, err)
		require.Len(t, computed, 1)
		require.Equal(t, false, computed[0].DefaultSourceValue)
	})

	t.Run("HideRedisplay", func(t *testing.T) {
		t.Parallel()
		db := databasefake.New()
		scope := generateScope()
		parameterSchema := generateParameter(t, db, parameterOptions{
			ProvisionJobID:           scope.ProvisionJobID,
			DefaultDestinationScheme: database.ParameterDestinationSchemeProvisionerVariable,
		})
		computed, err := parameter.Compute(context.Background(), db, parameter.ComputeOptions{
			ParameterSchemas:    []database.ParameterSchema{parameterSchema},
			Scope:               scope,
			HideRedisplayValues: true,
		})
		require.NoError(t, err)
		require.Len(t, computed, 1)
		computedValue := computed[0]
		require.True(t, computedValue.DefaultSourceValue)
		require.Equal(t, computedValue.SourceValue, "")
	})

	t.Run("BlockReservedNamespace", func(t *testing.T) {
		t.Parallel()
		db := databasefake.New()
		scope := generateScope()
		parameterSchema := generateParameter(t, db, parameterOptions{
			Name:                     parameter.Username,
			ProvisionJobID:           scope.ProvisionJobID,
			DefaultDestinationScheme: database.ParameterDestinationSchemeProvisionerVariable,
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
		_, err = parameter.Compute(context.Background(), db, parameter.ComputeOptions{
			ParameterSchemas: []database.ParameterSchema{parameterSchema},
			Scope:            scope,
		})
		require.Error(t, err)
	})
}
