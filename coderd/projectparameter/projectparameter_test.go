package projectparameter_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/projectparameter"
	"github.com/coder/coder/cryptorand"
	"github.com/coder/coder/database"
	"github.com/coder/coder/database/databasefake"
	"github.com/coder/coder/provisionersdk/proto"
)

func TestCompute(t *testing.T) {
	t.Parallel()
	generateScope := func() projectparameter.Scope {
		return projectparameter.Scope{
			OrganizationID:   uuid.New().String(),
			ProjectID:        uuid.New(),
			ProjectHistoryID: uuid.New(),
			UserID:           uuid.NewString(),
		}
	}
	type projectParameterOptions struct {
		AllowOverrideSource      bool
		AllowOverrideDestination bool
		DefaultDestinationScheme database.ParameterDestinationScheme
		ProjectHistoryID         uuid.UUID
	}
	generateProjectParameter := func(t *testing.T, db database.Store, opts projectParameterOptions) database.ProjectParameter {
		if opts.DefaultDestinationScheme == "" {
			opts.DefaultDestinationScheme = database.ParameterDestinationSchemeEnvironmentVariable
		}
		name, err := cryptorand.String(8)
		require.NoError(t, err)
		sourceValue, err := cryptorand.String(8)
		require.NoError(t, err)
		destinationValue, err := cryptorand.String(8)
		require.NoError(t, err)
		param, err := db.InsertProjectParameter(context.Background(), database.InsertProjectParameterParams{
			ID:                  uuid.New(),
			Name:                name,
			ProjectHistoryID:    opts.ProjectHistoryID,
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
		parameter, err := db.InsertProjectParameter(context.Background(), database.InsertProjectParameterParams{
			ID:               uuid.New(),
			ProjectHistoryID: scope.ProjectHistoryID,
			Name:             "hey",
		})
		require.NoError(t, err)

		_, err = projectparameter.Compute(context.Background(), db, scope)
		var noValueErr projectparameter.NoValueError
		require.ErrorAs(t, err, &noValueErr)
		require.Equal(t, parameter.ID.String(), noValueErr.ParameterID.String())
		require.Equal(t, parameter.Name, noValueErr.ParameterName)
	})

	t.Run("UseDefaultProjectValue", func(t *testing.T) {
		t.Parallel()
		db := databasefake.New()
		scope := generateScope()
		parameter := generateProjectParameter(t, db, projectParameterOptions{
			ProjectHistoryID:         scope.ProjectHistoryID,
			DefaultDestinationScheme: database.ParameterDestinationSchemeProvisionerVariable,
		})
		values, err := projectparameter.Compute(context.Background(), db, scope)
		require.NoError(t, err)
		require.Len(t, values, 1)
		value := values[0]
		require.True(t, value.DefaultValue)
		require.Equal(t, database.ParameterScopeProject, value.Scope)
		require.Equal(t, scope.ProjectID.String(), value.ScopeID)
		require.Equal(t, value.Proto.Name, parameter.DefaultDestinationValue.String)
		require.Equal(t, value.Proto.DestinationScheme, proto.ParameterDestination_PROVISIONER_VARIABLE)
		require.Equal(t, value.Proto.Value, parameter.DefaultSourceValue.String)
	})

	t.Run("OverrideOrganizationWithProjectDefault", func(t *testing.T) {
		t.Parallel()
		db := databasefake.New()
		scope := generateScope()
		parameter := generateProjectParameter(t, db, projectParameterOptions{
			ProjectHistoryID: scope.ProjectHistoryID,
		})
		_, err := db.InsertParameterValue(context.Background(), database.InsertParameterValueParams{
			ID:                uuid.New(),
			Name:              parameter.Name,
			Scope:             database.ParameterScopeOrganization,
			ScopeID:           scope.OrganizationID,
			SourceScheme:      database.ParameterSourceSchemeData,
			SourceValue:       "nop",
			DestinationScheme: database.ParameterDestinationSchemeEnvironmentVariable,
			DestinationValue:  "organizationvalue",
		})
		require.NoError(t, err)

		values, err := projectparameter.Compute(context.Background(), db, scope)
		require.NoError(t, err)
		require.Len(t, values, 1)
		require.Equal(t, true, values[0].DefaultValue)
		require.Equal(t, parameter.DefaultSourceValue.String, values[0].Proto.Value)
	})

	t.Run("ProjectOverridesProjectDefault", func(t *testing.T) {
		t.Parallel()
		db := databasefake.New()
		scope := generateScope()
		parameter := generateProjectParameter(t, db, projectParameterOptions{
			ProjectHistoryID: scope.ProjectHistoryID,
		})
		value, err := db.InsertParameterValue(context.Background(), database.InsertParameterValueParams{
			ID:                uuid.New(),
			Name:              parameter.Name,
			Scope:             database.ParameterScopeProject,
			ScopeID:           scope.ProjectID.String(),
			SourceScheme:      database.ParameterSourceSchemeData,
			SourceValue:       "nop",
			DestinationScheme: database.ParameterDestinationSchemeEnvironmentVariable,
			DestinationValue:  "projectvalue",
		})
		require.NoError(t, err)

		values, err := projectparameter.Compute(context.Background(), db, scope)
		require.NoError(t, err)
		require.Len(t, values, 1)
		require.Equal(t, false, values[0].DefaultValue)
		require.Equal(t, value.DestinationValue, values[0].Proto.Value)
	})

	t.Run("WorkspaceCannotOverwriteProjectDefault", func(t *testing.T) {
		t.Parallel()
		db := databasefake.New()
		scope := generateScope()
		parameter := generateProjectParameter(t, db, projectParameterOptions{
			ProjectHistoryID: scope.ProjectHistoryID,
		})
		_, err := db.InsertParameterValue(context.Background(), database.InsertParameterValueParams{
			ID:                uuid.New(),
			Name:              parameter.Name,
			Scope:             database.ParameterScopeWorkspace,
			ScopeID:           scope.WorkspaceID.String(),
			SourceScheme:      database.ParameterSourceSchemeData,
			SourceValue:       "nop",
			DestinationScheme: database.ParameterDestinationSchemeEnvironmentVariable,
			DestinationValue:  "projectvalue",
		})
		require.NoError(t, err)

		values, err := projectparameter.Compute(context.Background(), db, scope)
		require.NoError(t, err)
		require.Len(t, values, 1)
		require.Equal(t, true, values[0].DefaultValue)
	})

	t.Run("WorkspaceOverwriteProjectDefault", func(t *testing.T) {
		t.Parallel()
		db := databasefake.New()
		scope := generateScope()
		parameter := generateProjectParameter(t, db, projectParameterOptions{
			AllowOverrideSource: true,
			ProjectHistoryID:    scope.ProjectHistoryID,
		})
		_, err := db.InsertParameterValue(context.Background(), database.InsertParameterValueParams{
			ID:                uuid.New(),
			Name:              parameter.Name,
			Scope:             database.ParameterScopeWorkspace,
			ScopeID:           scope.WorkspaceID.String(),
			SourceScheme:      database.ParameterSourceSchemeData,
			SourceValue:       "nop",
			DestinationScheme: database.ParameterDestinationSchemeEnvironmentVariable,
			DestinationValue:  "projectvalue",
		})
		require.NoError(t, err)

		values, err := projectparameter.Compute(context.Background(), db, scope)
		require.NoError(t, err)
		require.Len(t, values, 1)
		require.Equal(t, false, values[0].DefaultValue)
	})
}
