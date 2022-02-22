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
	type parameterOptions struct {
		Name                     string
		AllowOverrideSource      bool
		AllowOverrideDestination bool
		DefaultDestinationScheme database.ParameterDestinationScheme
		ProvisionJobID           uuid.UUID
	}
	generateOptions := func(hideRedisplay bool) parameter.ComputeOptions {
		return parameter.ComputeOptions{
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

			HideRedisplayValues: hideRedisplay,
		}
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
		options := generateOptions(false)
		parameterSchema, err := db.InsertParameterSchema(context.Background(), database.InsertParameterSchemaParams{
			ID:                  uuid.New(),
			JobID:               options.ProvisionJobID,
			Name:                "hey",
			DefaultSourceScheme: database.ParameterSourceSchemeNone,
		})
		require.NoError(t, err)
		options.Schemas = []database.ParameterSchema{parameterSchema}
		computed, err := parameter.Compute(context.Background(), db, options)
		require.NoError(t, err)
		require.Len(t, computed, 0)
	})

	t.Run("UseDefaultProjectValue", func(t *testing.T) {
		t.Parallel()
		db := databasefake.New()
		options := generateOptions(false)
		parameterSchema := generateParameter(t, db, parameterOptions{
			ProvisionJobID:           options.ProvisionJobID,
			DefaultDestinationScheme: database.ParameterDestinationSchemeProvisionerVariable,
		})
		options.Schemas = []database.ParameterSchema{parameterSchema}
		computed, err := parameter.Compute(context.Background(), db, options)
		require.NoError(t, err)
		require.Len(t, computed, 1)
		computedValue := computed[0]
		require.True(t, computedValue.DefaultSourceValue)
		require.Equal(t, database.ParameterScopeProvisionerJob, computedValue.Scope)
		require.Equal(t, options.ProvisionJobID.String(), computedValue.ScopeID)
		require.Equal(t, computedValue.SourceValue, parameterSchema.DefaultSourceValue)
	})

	t.Run("OverrideOrganizationWithImportJob", func(t *testing.T) {
		t.Parallel()
		db := databasefake.New()
		options := generateOptions(false)
		parameterSchema := generateParameter(t, db, parameterOptions{
			ProvisionJobID: options.ProvisionJobID,
		})
		_, err := db.InsertParameterValue(context.Background(), database.InsertParameterValueParams{
			ID:                uuid.New(),
			Name:              parameterSchema.Name,
			Scope:             database.ParameterScopeOrganization,
			ScopeID:           options.OrganizationID,
			SourceScheme:      database.ParameterSourceSchemeData,
			SourceValue:       "firstnop",
			DestinationScheme: database.ParameterDestinationSchemeEnvironmentVariable,
		})
		require.NoError(t, err)

		value, err := db.InsertParameterValue(context.Background(), database.InsertParameterValueParams{
			ID:                uuid.New(),
			Name:              parameterSchema.Name,
			Scope:             database.ParameterScopeProvisionerJob,
			ScopeID:           options.ProvisionJobID.String(),
			SourceScheme:      database.ParameterSourceSchemeData,
			SourceValue:       "secondnop",
			DestinationScheme: database.ParameterDestinationSchemeEnvironmentVariable,
		})
		require.NoError(t, err)
		options.Schemas = []database.ParameterSchema{parameterSchema}

		computed, err := parameter.Compute(context.Background(), db, options)
		require.NoError(t, err)
		require.Len(t, computed, 1)
		require.Equal(t, false, computed[0].DefaultSourceValue)
		require.Equal(t, value.SourceValue, computed[0].SourceValue)
	})

	t.Run("NamespacedProvisionJob", func(t *testing.T) {
		t.Parallel()
		db := databasefake.New()
		options := generateOptions(false)
		parameterSchema := generateParameter(t, db, parameterOptions{
			Name:           parameter.Username,
			ProvisionJobID: options.ProvisionJobID,
		})
		value, err := db.InsertParameterValue(context.Background(), database.InsertParameterValueParams{
			ID:                uuid.New(),
			Name:              parameterSchema.Name,
			Scope:             database.ParameterScopeProvisionerJob,
			ScopeID:           options.ProvisionJobID.String(),
			SourceScheme:      database.ParameterSourceSchemeData,
			SourceValue:       "firstnop",
			DestinationScheme: database.ParameterDestinationSchemeProvisionerVariable,
		})
		require.NoError(t, err)
		options.Schemas = []database.ParameterSchema{parameterSchema}
		computed, err := parameter.Compute(context.Background(), db, options)
		require.NoError(t, err)
		require.Len(t, computed, 1)
		require.Equal(t, value.SourceValue, computed[0].SourceValue)
	})

	t.Run("ProjectOverridesProjectDefault", func(t *testing.T) {
		t.Parallel()
		db := databasefake.New()
		options := generateOptions(false)
		parameterSchema := generateParameter(t, db, parameterOptions{
			ProvisionJobID: options.ProvisionJobID,
		})
		value, err := db.InsertParameterValue(context.Background(), database.InsertParameterValueParams{
			ID:                uuid.New(),
			Name:              parameterSchema.Name,
			Scope:             database.ParameterScopeProject,
			ScopeID:           options.ProjectID.UUID.String(),
			SourceScheme:      database.ParameterSourceSchemeData,
			SourceValue:       "nop",
			DestinationScheme: database.ParameterDestinationSchemeEnvironmentVariable,
		})
		require.NoError(t, err)
		options.Schemas = []database.ParameterSchema{parameterSchema}
		computed, err := parameter.Compute(context.Background(), db, options)
		require.NoError(t, err)
		require.Len(t, computed, 1)
		require.Equal(t, false, computed[0].DefaultSourceValue)
		require.Equal(t, value.SourceValue, computed[0].SourceValue)
	})

	t.Run("WorkspaceCannotOverwriteProjectDefault", func(t *testing.T) {
		t.Parallel()
		db := databasefake.New()
		options := generateOptions(false)
		parameterSchema := generateParameter(t, db, parameterOptions{
			ProvisionJobID: options.ProvisionJobID,
		})
		_, err := db.InsertParameterValue(context.Background(), database.InsertParameterValueParams{
			ID:                uuid.New(),
			Name:              parameterSchema.Name,
			Scope:             database.ParameterScopeWorkspace,
			ScopeID:           options.WorkspaceID.UUID.String(),
			SourceScheme:      database.ParameterSourceSchemeData,
			SourceValue:       "nop",
			DestinationScheme: database.ParameterDestinationSchemeEnvironmentVariable,
		})
		require.NoError(t, err)
		options.Schemas = []database.ParameterSchema{parameterSchema}
		computed, err := parameter.Compute(context.Background(), db, options)
		require.NoError(t, err)
		require.Len(t, computed, 1)
		require.Equal(t, true, computed[0].DefaultSourceValue)
	})

	t.Run("WorkspaceOverwriteProjectDefault", func(t *testing.T) {
		t.Parallel()
		db := databasefake.New()
		options := generateOptions(false)
		parameterSchema := generateParameter(t, db, parameterOptions{
			AllowOverrideSource: true,
			ProvisionJobID:      options.ProvisionJobID,
		})
		_, err := db.InsertParameterValue(context.Background(), database.InsertParameterValueParams{
			ID:                uuid.New(),
			Name:              parameterSchema.Name,
			Scope:             database.ParameterScopeWorkspace,
			ScopeID:           options.WorkspaceID.UUID.String(),
			SourceScheme:      database.ParameterSourceSchemeData,
			SourceValue:       "nop",
			DestinationScheme: database.ParameterDestinationSchemeEnvironmentVariable,
		})
		require.NoError(t, err)
		options.Schemas = []database.ParameterSchema{parameterSchema}
		computed, err := parameter.Compute(context.Background(), db, options)
		require.NoError(t, err)
		require.Len(t, computed, 1)
		require.Equal(t, false, computed[0].DefaultSourceValue)
	})

	t.Run("HideRedisplay", func(t *testing.T) {
		t.Parallel()
		db := databasefake.New()
		options := generateOptions(true)
		parameterSchema := generateParameter(t, db, parameterOptions{
			ProvisionJobID:           options.ProvisionJobID,
			DefaultDestinationScheme: database.ParameterDestinationSchemeProvisionerVariable,
		})
		options.Schemas = []database.ParameterSchema{parameterSchema}
		computed, err := parameter.Compute(context.Background(), db, options)
		require.NoError(t, err)
		require.Len(t, computed, 1)
		computedValue := computed[0]
		require.True(t, computedValue.DefaultSourceValue)
		require.Equal(t, computedValue.SourceValue, "")
	})

	t.Run("BlockReservedNamespace", func(t *testing.T) {
		t.Parallel()
		db := databasefake.New()
		options := generateOptions(false)
		parameterSchema := generateParameter(t, db, parameterOptions{
			Name:                     parameter.Username,
			ProvisionJobID:           options.ProvisionJobID,
			DefaultDestinationScheme: database.ParameterDestinationSchemeProvisionerVariable,
		})
		_, err := db.InsertParameterValue(context.Background(), database.InsertParameterValueParams{
			ID:                uuid.New(),
			Name:              parameterSchema.Name,
			Scope:             database.ParameterScopeWorkspace,
			ScopeID:           options.WorkspaceID.UUID.String(),
			SourceScheme:      database.ParameterSourceSchemeData,
			SourceValue:       "nop",
			DestinationScheme: database.ParameterDestinationSchemeEnvironmentVariable,
		})
		require.NoError(t, err)
		options.Schemas = []database.ParameterSchema{parameterSchema}
		_, err = parameter.Compute(context.Background(), db, options)
		require.Error(t, err)
	})
}
