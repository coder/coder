package parameter_test

import (
	"context"
	"database/sql"
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
	generateScope := func() parameter.Scope {
		return parameter.Scope{
			ImportJobID:    uuid.New(),
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
		AllowOverrideSource      bool
		AllowOverrideDestination bool
		DefaultDestinationScheme database.ParameterDestinationScheme
		ImportJobID              uuid.UUID
	}
	generateParameter := func(t *testing.T, db database.Store, opts parameterOptions) database.ParameterSchema {
		if opts.DefaultDestinationScheme == "" {
			opts.DefaultDestinationScheme = database.ParameterDestinationSchemeEnvironmentVariable
		}
		name, err := cryptorand.String(8)
		require.NoError(t, err)
		sourceValue, err := cryptorand.String(8)
		require.NoError(t, err)
		destinationValue, err := cryptorand.String(8)
		require.NoError(t, err)
		param, err := db.InsertParameterSchema(context.Background(), database.InsertParameterSchemaParams{
			ID:                  uuid.New(),
			Name:                name,
			JobID:               opts.ImportJobID,
			DefaultSourceScheme: database.ParameterSourceSchemeData,
			DefaultSourceValue: sql.NullString{
				String: sourceValue,
				Valid:  true,
			},
			DefaultDestinationValue: sql.NullString{
				String: destinationValue,
				Valid:  true,
			},
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
		parameterSchema, err := db.InsertParameterSchema(context.Background(), database.InsertParameterSchemaParams{
			ID:    uuid.New(),
			JobID: scope.ImportJobID,
			Name:  "hey",
		})
		require.NoError(t, err)
		computed, err := parameter.Compute(context.Background(), db, scope)
		require.NoError(t, err)
		require.Equal(t, parameterSchema.ID.String(), computed[0].Schema.ID.String())
		require.Nil(t, computed[0].Value)
	})

	t.Run("UseDefaultProjectValue", func(t *testing.T) {
		t.Parallel()
		db := databasefake.New()
		scope := generateScope()
		parameterSchema := generateParameter(t, db, parameterOptions{
			ImportJobID:              scope.ImportJobID,
			DefaultDestinationScheme: database.ParameterDestinationSchemeProvisionerVariable,
		})
		computed, err := parameter.Compute(context.Background(), db, scope)
		require.NoError(t, err)
		require.Len(t, computed, 1)
		computedValue := computed[0]
		require.True(t, computedValue.Value.Default)
		require.Equal(t, database.ParameterScopeProject, computedValue.Value.Scope)
		require.Equal(t, scope.ProjectID.UUID.String(), computedValue.Value.ScopeID)
		require.Equal(t, computedValue.Value.Name, parameterSchema.DefaultDestinationValue.String)
		require.Equal(t, computedValue.Value.DestinationScheme, database.ParameterDestinationSchemeProvisionerVariable)
		require.Equal(t, computedValue.Value.Value, parameterSchema.DefaultSourceValue.String)
	})

	t.Run("OverrideOrganizationWithProjectDefault", func(t *testing.T) {
		t.Parallel()
		db := databasefake.New()
		scope := generateScope()
		parameterSchema := generateParameter(t, db, parameterOptions{
			ImportJobID: scope.ImportJobID,
		})
		_, err := db.InsertParameterValue(context.Background(), database.InsertParameterValueParams{
			ID:                uuid.New(),
			Name:              parameterSchema.Name,
			Scope:             database.ParameterScopeOrganization,
			ScopeID:           scope.OrganizationID,
			SourceScheme:      database.ParameterSourceSchemeData,
			SourceValue:       "nop",
			DestinationScheme: database.ParameterDestinationSchemeEnvironmentVariable,
			DestinationValue:  "organizationvalue",
		})
		require.NoError(t, err)

		computed, err := parameter.Compute(context.Background(), db, scope)
		require.NoError(t, err)
		require.Len(t, computed, 1)
		require.Equal(t, true, computed[0].Value.Default)
		require.Equal(t, parameterSchema.DefaultSourceValue.String, computed[0].Value.Value)
	})

	t.Run("ProjectOverridesProjectDefault", func(t *testing.T) {
		t.Parallel()
		db := databasefake.New()
		scope := generateScope()
		parameterSchema := generateParameter(t, db, parameterOptions{
			ImportJobID: scope.ImportJobID,
		})
		value, err := db.InsertParameterValue(context.Background(), database.InsertParameterValueParams{
			ID:                uuid.New(),
			Name:              parameterSchema.Name,
			Scope:             database.ParameterScopeProject,
			ScopeID:           scope.ProjectID.UUID.String(),
			SourceScheme:      database.ParameterSourceSchemeData,
			SourceValue:       "nop",
			DestinationScheme: database.ParameterDestinationSchemeEnvironmentVariable,
			DestinationValue:  "projectvalue",
		})
		require.NoError(t, err)

		computed, err := parameter.Compute(context.Background(), db, scope)
		require.NoError(t, err)
		require.Len(t, computed, 1)
		require.Equal(t, false, computed[0].Value.Default)
		require.Equal(t, value.DestinationValue, computed[0].Value.Value)
	})

	t.Run("WorkspaceCannotOverwriteProjectDefault", func(t *testing.T) {
		t.Parallel()
		db := databasefake.New()
		scope := generateScope()
		parameterSchema := generateParameter(t, db, parameterOptions{
			ImportJobID: scope.ImportJobID,
		})
		_, err := db.InsertParameterValue(context.Background(), database.InsertParameterValueParams{
			ID:                uuid.New(),
			Name:              parameterSchema.Name,
			Scope:             database.ParameterScopeWorkspace,
			ScopeID:           scope.WorkspaceID.UUID.String(),
			SourceScheme:      database.ParameterSourceSchemeData,
			SourceValue:       "nop",
			DestinationScheme: database.ParameterDestinationSchemeEnvironmentVariable,
			DestinationValue:  "projectvalue",
		})
		require.NoError(t, err)

		computed, err := parameter.Compute(context.Background(), db, scope)
		require.NoError(t, err)
		require.Len(t, computed, 1)
		require.Equal(t, true, computed[0].Value.Default)
	})

	t.Run("WorkspaceOverwriteProjectDefault", func(t *testing.T) {
		t.Parallel()
		db := databasefake.New()
		scope := generateScope()
		parameterSchema := generateParameter(t, db, parameterOptions{
			AllowOverrideSource: true,
			ImportJobID:         scope.ImportJobID,
		})
		_, err := db.InsertParameterValue(context.Background(), database.InsertParameterValueParams{
			ID:                uuid.New(),
			Name:              parameterSchema.Name,
			Scope:             database.ParameterScopeWorkspace,
			ScopeID:           scope.WorkspaceID.UUID.String(),
			SourceScheme:      database.ParameterSourceSchemeData,
			SourceValue:       "nop",
			DestinationScheme: database.ParameterDestinationSchemeEnvironmentVariable,
			DestinationValue:  "projectvalue",
		})
		require.NoError(t, err)

		computed, err := parameter.Compute(context.Background(), db, scope)
		require.NoError(t, err)
		require.Len(t, computed, 1)
		require.Equal(t, false, computed[0].Value.Default)
	})

	t.Run("AdditionalOverwriteWorkspace", func(t *testing.T) {
		t.Parallel()
		db := databasefake.New()
		scope := generateScope()
		parameterSchema := generateParameter(t, db, parameterOptions{
			AllowOverrideSource: true,
			ImportJobID:         scope.ImportJobID,
		})
		_, err := db.InsertParameterValue(context.Background(), database.InsertParameterValueParams{
			ID:                uuid.New(),
			Name:              parameterSchema.Name,
			Scope:             database.ParameterScopeWorkspace,
			ScopeID:           scope.WorkspaceID.UUID.String(),
			SourceScheme:      database.ParameterSourceSchemeData,
			SourceValue:       "nop",
			DestinationScheme: database.ParameterDestinationSchemeEnvironmentVariable,
			DestinationValue:  "projectvalue",
		})
		require.NoError(t, err)

		computed, err := parameter.Compute(context.Background(), db, scope, database.ParameterValue{
			Name:              parameterSchema.Name,
			Scope:             database.ParameterScopeUser,
			SourceScheme:      database.ParameterSourceSchemeData,
			SourceValue:       "nop",
			DestinationScheme: database.ParameterDestinationSchemeEnvironmentVariable,
			DestinationValue:  "testing",
		})
		require.NoError(t, err)
		require.Len(t, computed, 1)
		require.Equal(t, "testing", computed[0].Value.Value)
	})
}
