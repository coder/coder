package wsbuilder_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/coder/coder/v2/provisionersdk"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/provisionerdserver"
	"github.com/coder/coder/v2/coderd/wsbuilder"
	"github.com/coder/coder/v2/codersdk"
)

var (
	// use fixed IDs so logs are easier to read
	templateID        = uuid.MustParse("12341234-0000-0000-0001-000000000000")
	activeVersionID   = uuid.MustParse("12341234-0000-0000-0002-000000000000")
	inactiveVersionID = uuid.MustParse("12341234-0000-0000-0003-000000000000")
	activeJobID       = uuid.MustParse("12341234-0000-0000-0004-000000000000")
	inactiveJobID     = uuid.MustParse("12341234-0000-0000-0005-000000000000")
	orgID             = uuid.MustParse("12341234-0000-0000-0006-000000000000")
	workspaceID       = uuid.MustParse("12341234-0000-0000-0007-000000000000")
	userID            = uuid.MustParse("12341234-0000-0000-0008-000000000000")
	activeFileID      = uuid.MustParse("12341234-0000-0000-0009-000000000000")
	inactiveFileID    = uuid.MustParse("12341234-0000-0000-000a-000000000000")
	lastBuildID       = uuid.MustParse("12341234-0000-0000-000b-000000000000")
	lastBuildJobID    = uuid.MustParse("12341234-0000-0000-000c-000000000000")
	otherUserID       = uuid.MustParse("12341234-0000-0000-000d-000000000000")
)

func TestBuilder_NoOptions(t *testing.T) {
	t.Parallel()
	req := require.New(t)
	asrt := assert.New(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var buildID uuid.UUID

	mDB := expectDB(t,
		// Inputs
		withTemplate,
		withInactiveVersion(nil),
		withLastBuildFound,
		withRichParameters(nil),
		withParameterSchemas(inactiveJobID, nil),

		// Outputs
		expectProvisionerJob(func(job database.InsertProvisionerJobParams) {
			asrt.Equal(userID, job.InitiatorID)
			asrt.Equal(inactiveFileID, job.FileID)
			input := provisionerdserver.WorkspaceProvisionJob{}
			err := json.Unmarshal(job.Input, &input)
			req.NoError(err)
			// store build ID for later
			buildID = input.WorkspaceBuildID
		}),

		withInTx,
		expectBuild(func(bld database.InsertWorkspaceBuildParams) {
			asrt.Equal(inactiveVersionID, bld.TemplateVersionID)
			asrt.Equal(workspaceID, bld.WorkspaceID)
			asrt.Equal(int32(2), bld.BuildNumber)
			asrt.Equal("last build state", string(bld.ProvisionerState))
			asrt.Equal(userID, bld.InitiatorID)
			asrt.Equal(database.WorkspaceTransitionStart, bld.Transition)
			asrt.Equal(database.BuildReasonInitiator, bld.Reason)
			asrt.Equal(buildID, bld.ID)
		}),
		withBuild,
		expectBuildParameters(func(params database.InsertWorkspaceBuildParametersParams) {
			asrt.Equal(buildID, params.WorkspaceBuildID)
			asrt.Empty(params.Name)
			asrt.Empty(params.Value)
		}),
	)

	ws := database.Workspace{ID: workspaceID, TemplateID: templateID, OwnerID: userID}
	uut := wsbuilder.New(ws, database.WorkspaceTransitionStart)
	_, _, err := uut.Build(ctx, mDB, nil, audit.WorkspaceBuildBaggage{})
	req.NoError(err)
}

func TestBuilder_Initiator(t *testing.T) {
	t.Parallel()
	req := require.New(t)
	asrt := assert.New(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mDB := expectDB(t,
		// Inputs
		withTemplate,
		withInactiveVersion(nil),
		withLastBuildFound,
		withRichParameters(nil),
		withParameterSchemas(inactiveJobID, nil),

		// Outputs
		expectProvisionerJob(func(job database.InsertProvisionerJobParams) {
			asrt.Equal(otherUserID, job.InitiatorID)
		}),
		withInTx,
		expectBuild(func(bld database.InsertWorkspaceBuildParams) {
			asrt.Equal(otherUserID, bld.InitiatorID)
		}),
		expectBuildParameters(func(params database.InsertWorkspaceBuildParametersParams) {
		}),
		withBuild,
	)

	ws := database.Workspace{ID: workspaceID, TemplateID: templateID, OwnerID: userID}
	uut := wsbuilder.New(ws, database.WorkspaceTransitionStart).Initiator(otherUserID)
	_, _, err := uut.Build(ctx, mDB, nil, audit.WorkspaceBuildBaggage{})
	req.NoError(err)
}

func TestBuilder_Baggage(t *testing.T) {
	t.Parallel()
	req := require.New(t)
	asrt := assert.New(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		),
	)

	mDB := expectDB(t,
		// Inputs
		withTemplate,
		withInactiveVersion(nil),
		withLastBuildFound,
		withRichParameters(nil),
		withParameterSchemas(inactiveJobID, nil),

		// Outputs
		expectProvisionerJob(func(job database.InsertProvisionerJobParams) {
			asrt.Contains(string(job.TraceMetadata.RawMessage), "ip=127.0.0.1")
		}),
		withInTx,
		expectBuild(func(bld database.InsertWorkspaceBuildParams) {
		}),
		expectBuildParameters(func(params database.InsertWorkspaceBuildParametersParams) {
		}),
		withBuild,
	)

	ws := database.Workspace{ID: workspaceID, TemplateID: templateID, OwnerID: userID}
	uut := wsbuilder.New(ws, database.WorkspaceTransitionStart).Initiator(otherUserID)
	_, _, err := uut.Build(ctx, mDB, nil, audit.WorkspaceBuildBaggage{IP: "127.0.0.1"})
	req.NoError(err)
}

func TestBuilder_Reason(t *testing.T) {
	t.Parallel()
	req := require.New(t)
	asrt := assert.New(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mDB := expectDB(t,
		// Inputs
		withTemplate,
		withInactiveVersion(nil),
		withLastBuildFound,
		withRichParameters(nil),
		withParameterSchemas(inactiveJobID, nil),

		// Outputs
		expectProvisionerJob(func(job database.InsertProvisionerJobParams) {
		}),
		withInTx,
		expectBuild(func(bld database.InsertWorkspaceBuildParams) {
			asrt.Equal(database.BuildReasonAutostart, bld.Reason)
		}),
		expectBuildParameters(func(params database.InsertWorkspaceBuildParametersParams) {
		}),
		withBuild,
	)

	ws := database.Workspace{ID: workspaceID, TemplateID: templateID, OwnerID: userID}
	uut := wsbuilder.New(ws, database.WorkspaceTransitionStart).Reason(database.BuildReasonAutostart)
	_, _, err := uut.Build(ctx, mDB, nil, audit.WorkspaceBuildBaggage{})
	req.NoError(err)
}

func TestBuilder_ActiveVersion(t *testing.T) {
	t.Parallel()
	req := require.New(t)
	asrt := assert.New(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mDB := expectDB(t,
		// Inputs
		withTemplate,
		withActiveVersion(nil),
		withLastBuildNotFound,
		withParameterSchemas(activeJobID, nil),
		// previous rich parameters are not queried because there is no previous build.

		// Outputs
		expectProvisionerJob(func(job database.InsertProvisionerJobParams) {
			asrt.Equal(activeFileID, job.FileID)
		}),

		withInTx,
		expectBuild(func(bld database.InsertWorkspaceBuildParams) {
			asrt.Equal(activeVersionID, bld.TemplateVersionID)
			// no previous build...
			asrt.Equal(int32(1), bld.BuildNumber)
			asrt.Len(bld.ProvisionerState, 0)
		}),
		expectBuildParameters(func(params database.InsertWorkspaceBuildParametersParams) {
		}),
		withBuild,
	)

	ws := database.Workspace{ID: workspaceID, TemplateID: templateID, OwnerID: userID}
	uut := wsbuilder.New(ws, database.WorkspaceTransitionStart).ActiveVersion()
	_, _, err := uut.Build(ctx, mDB, nil, audit.WorkspaceBuildBaggage{})
	req.NoError(err)
}

func TestWorkspaceBuildWithRichParameters(t *testing.T) {
	t.Parallel()

	const (
		firstParameterName        = "first_parameter"
		firstParameterDescription = "This is first parameter"
		firstParameterValue       = "1"

		secondParameterName        = "second_parameter"
		secondParameterDescription = "This is second parameter"
		secondParameterValue       = "2"

		immutableParameterName        = "immutable_parameter"
		immutableParameterDescription = "This is immutable parameter"
		immutableParameterValue       = "3"
	)

	initialBuildParameters := []database.WorkspaceBuildParameter{
		{Name: firstParameterName, Value: firstParameterValue},
		{Name: secondParameterName, Value: secondParameterValue},
		{Name: immutableParameterName, Value: immutableParameterValue},
	}

	richParameters := []database.TemplateVersionParameter{
		{Name: firstParameterName, Description: firstParameterDescription, Mutable: true, Options: json.RawMessage("[]")},
		{Name: secondParameterName, Description: secondParameterDescription, Mutable: true, Options: json.RawMessage("[]")},
		{Name: immutableParameterName, Description: immutableParameterDescription, Mutable: false, Options: json.RawMessage("[]")},
	}

	t.Run("UpdateParameterValues", func(t *testing.T) {
		t.Parallel()

		req := require.New(t)
		asrt := assert.New(t)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		const updatedParameterValue = "3"
		nextBuildParameters := []codersdk.WorkspaceBuildParameter{
			{Name: firstParameterName, Value: firstParameterValue},
			{Name: secondParameterName, Value: updatedParameterValue},
		}
		expectedParams := map[string]string{
			firstParameterName:     firstParameterValue,
			secondParameterName:    updatedParameterValue,
			immutableParameterName: immutableParameterValue,
		}

		mDB := expectDB(t,
			// Inputs
			withTemplate,
			withInactiveVersion(richParameters),
			withLastBuildFound,
			withRichParameters(initialBuildParameters),
			withParameterSchemas(inactiveJobID, nil),

			// Outputs
			expectProvisionerJob(func(job database.InsertProvisionerJobParams) {}),
			withInTx,
			expectBuild(func(bld database.InsertWorkspaceBuildParams) {}),
			expectBuildParameters(func(params database.InsertWorkspaceBuildParametersParams) {
				asrt.Len(params.Name, len(expectedParams))
				for i := range params.Name {
					value, ok := expectedParams[params.Name[i]]
					asrt.True(ok, "unexpected name %s", params.Name[i])
					asrt.Equal(value, params.Value[i])
				}
			}),
			withBuild,
		)

		ws := database.Workspace{ID: workspaceID, TemplateID: templateID, OwnerID: userID}
		uut := wsbuilder.New(ws, database.WorkspaceTransitionStart).RichParameterValues(nextBuildParameters)
		_, _, err := uut.Build(ctx, mDB, nil, audit.WorkspaceBuildBaggage{})
		req.NoError(err)
	})
	t.Run("UsePreviousParameterValues", func(t *testing.T) {
		t.Parallel()

		req := require.New(t)
		asrt := assert.New(t)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		nextBuildParameters := []codersdk.WorkspaceBuildParameter{}
		expectedParams := map[string]string{}
		for _, p := range initialBuildParameters {
			expectedParams[p.Name] = p.Value
		}

		mDB := expectDB(t,
			// Inputs
			withTemplate,
			withInactiveVersion(richParameters),
			withLastBuildFound,
			withRichParameters(initialBuildParameters),
			withParameterSchemas(inactiveJobID, nil),

			// Outputs
			expectProvisionerJob(func(job database.InsertProvisionerJobParams) {}),
			withInTx,
			expectBuild(func(bld database.InsertWorkspaceBuildParams) {}),
			expectBuildParameters(func(params database.InsertWorkspaceBuildParametersParams) {
				asrt.Len(params.Name, len(expectedParams))
				for i := range params.Name {
					value, ok := expectedParams[params.Name[i]]
					asrt.True(ok, "unexpected name %s", params.Name[i])
					asrt.Equal(value, params.Value[i])
				}
			}),
			withBuild,
		)

		ws := database.Workspace{ID: workspaceID, TemplateID: templateID, OwnerID: userID}
		uut := wsbuilder.New(ws, database.WorkspaceTransitionStart).RichParameterValues(nextBuildParameters)
		_, _, err := uut.Build(ctx, mDB, nil, audit.WorkspaceBuildBaggage{})
		req.NoError(err)
	})

	t.Run("StartWorkspaceWithLegacyParameterValues", func(t *testing.T) {
		t.Parallel()

		req := require.New(t)
		asrt := assert.New(t)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		schemas := []database.ParameterSchema{
			{
				Name:                     "not-replaced",
				DefaultDestinationScheme: database.ParameterDestinationSchemeEnvironmentVariable,
			},
			{
				Name:                     "replaced",
				DefaultDestinationScheme: database.ParameterDestinationSchemeEnvironmentVariable,
			},
		}

		mDB := expectDB(t,
			// Inputs
			withTemplate,
			withInactiveVersion(richParameters),
			withLastBuildFound,
			withRichParameters(nil),
			withParameterSchemas(inactiveJobID, schemas),

			// Outputs
			expectProvisionerJob(func(job database.InsertProvisionerJobParams) {}),
			withInTx,
			expectBuild(func(bld database.InsertWorkspaceBuildParams) {}),
		)

		ws := database.Workspace{ID: workspaceID, TemplateID: templateID, OwnerID: userID}
		uut := wsbuilder.New(ws, database.WorkspaceTransitionStart)
		_, _, err := uut.Build(ctx, mDB, nil, audit.WorkspaceBuildBaggage{})
		bldErr := wsbuilder.BuildError{}
		req.ErrorAs(err, &bldErr)
		asrt.Equal(http.StatusBadRequest, bldErr.Status)
	})

	t.Run("DoNotModifyImmutables", func(t *testing.T) {
		t.Parallel()

		req := require.New(t)
		asrt := assert.New(t)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		nextBuildParameters := []codersdk.WorkspaceBuildParameter{
			{Name: immutableParameterName, Value: "BAD"},
		}

		mDB := expectDB(t,
			// Inputs
			withTemplate,
			withInactiveVersion(richParameters),
			withLastBuildFound,
			withRichParameters(initialBuildParameters),
			withParameterSchemas(inactiveJobID, nil),

			// Outputs
			expectProvisionerJob(func(job database.InsertProvisionerJobParams) {}),
			withInTx,
			expectBuild(func(bld database.InsertWorkspaceBuildParams) {}),
			// no build parameters, since we hit an error validating.
			// expectBuildParameters(func(params database.InsertWorkspaceBuildParametersParams) {}),
		)

		ws := database.Workspace{ID: workspaceID, TemplateID: templateID, OwnerID: userID}
		uut := wsbuilder.New(ws, database.WorkspaceTransitionStart).RichParameterValues(nextBuildParameters)
		_, _, err := uut.Build(ctx, mDB, nil, audit.WorkspaceBuildBaggage{})
		bldErr := wsbuilder.BuildError{}
		req.ErrorAs(err, &bldErr)
		asrt.Equal(http.StatusBadRequest, bldErr.Status)
	})

	t.Run("NewImmutableRequiredParameterAdded", func(t *testing.T) {
		t.Parallel()

		req := require.New(t)
		asrt := assert.New(t)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// new template revision
		const newImmutableParameterName = "new_immutable_parameter"
		const newImmutableParameterDescription = "This is also an immutable parameter"
		version2params := []database.TemplateVersionParameter{
			{Name: firstParameterName, Description: firstParameterDescription, Mutable: true, Options: json.RawMessage("[]")},
			{Name: secondParameterName, Description: secondParameterDescription, Mutable: true, Options: json.RawMessage("[]")},
			{Name: immutableParameterName, Description: immutableParameterDescription, Mutable: false, Options: json.RawMessage("[]")},
			{Name: newImmutableParameterName, Description: newImmutableParameterDescription, Mutable: false, Required: true, Options: json.RawMessage("[]")},
		}

		nextBuildParameters := []codersdk.WorkspaceBuildParameter{
			{Name: newImmutableParameterName, Value: "good"},
		}
		expectedParams := map[string]string{
			firstParameterName:        firstParameterValue,
			secondParameterName:       secondParameterValue,
			immutableParameterName:    immutableParameterValue,
			newImmutableParameterName: "good",
		}

		mDB := expectDB(t,
			// Inputs
			withTemplate,
			withActiveVersion(version2params),
			withLastBuildFound,
			withRichParameters(initialBuildParameters),
			withParameterSchemas(activeJobID, nil),

			// Outputs
			expectProvisionerJob(func(job database.InsertProvisionerJobParams) {}),
			withInTx,
			expectBuild(func(bld database.InsertWorkspaceBuildParams) {}),
			expectBuildParameters(func(params database.InsertWorkspaceBuildParametersParams) {
				asrt.Len(params.Name, len(expectedParams))
				for i := range params.Name {
					value, ok := expectedParams[params.Name[i]]
					asrt.True(ok, "unexpected name %s", params.Name[i])
					asrt.Equal(value, params.Value[i])
				}
			}),
			withBuild,
		)

		ws := database.Workspace{ID: workspaceID, TemplateID: templateID, OwnerID: userID}
		uut := wsbuilder.New(ws, database.WorkspaceTransitionStart).
			RichParameterValues(nextBuildParameters).
			VersionID(activeVersionID)
		_, _, err := uut.Build(ctx, mDB, nil, audit.WorkspaceBuildBaggage{})
		req.NoError(err)
	})

	t.Run("NewImmutableOptionalParameterAdded", func(t *testing.T) {
		t.Parallel()

		req := require.New(t)
		asrt := assert.New(t)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// new template revision
		const newImmutableParameterName = "new_immutable_parameter"
		const newImmutableParameterDescription = "This is also an immutable parameter"
		version2params := []database.TemplateVersionParameter{
			{Name: firstParameterName, Description: firstParameterDescription, Mutable: true, Options: json.RawMessage("[]")},
			{Name: secondParameterName, Description: secondParameterDescription, Mutable: true, Options: json.RawMessage("[]")},
			{Name: immutableParameterName, Description: immutableParameterDescription, Mutable: false, Options: json.RawMessage("[]")},
			{Name: newImmutableParameterName, Description: newImmutableParameterDescription, Mutable: false, DefaultValue: "12345", Options: json.RawMessage("[]")},
		}

		nextBuildParameters := []codersdk.WorkspaceBuildParameter{
			{Name: newImmutableParameterName, Value: "good"},
		}
		expectedParams := map[string]string{
			firstParameterName:        firstParameterValue,
			secondParameterName:       secondParameterValue,
			immutableParameterName:    immutableParameterValue,
			newImmutableParameterName: "good",
		}

		mDB := expectDB(t,
			// Inputs
			withTemplate,
			withActiveVersion(version2params),
			withLastBuildFound,
			withRichParameters(initialBuildParameters),
			withParameterSchemas(activeJobID, nil),

			// Outputs
			expectProvisionerJob(func(job database.InsertProvisionerJobParams) {}),
			withInTx,
			expectBuild(func(bld database.InsertWorkspaceBuildParams) {}),
			expectBuildParameters(func(params database.InsertWorkspaceBuildParametersParams) {
				asrt.Len(params.Name, len(expectedParams))
				for i := range params.Name {
					value, ok := expectedParams[params.Name[i]]
					asrt.True(ok, "unexpected name %s", params.Name[i])
					asrt.Equal(value, params.Value[i])
				}
			}),
			withBuild,
		)

		ws := database.Workspace{ID: workspaceID, TemplateID: templateID, OwnerID: userID}
		uut := wsbuilder.New(ws, database.WorkspaceTransitionStart).
			RichParameterValues(nextBuildParameters).
			VersionID(activeVersionID)
		_, _, err := uut.Build(ctx, mDB, nil, audit.WorkspaceBuildBaggage{})
		req.NoError(err)
	})

	t.Run("NewImmutableOptionalParameterUsesDefault", func(t *testing.T) {
		t.Parallel()

		req := require.New(t)
		asrt := assert.New(t)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// new template revision
		const newImmutableParameterName = "new_immutable_parameter"
		const newImmutableParameterDescription = "This is also an immutable parameter"
		version2params := []database.TemplateVersionParameter{
			{Name: firstParameterName, Description: firstParameterDescription, Mutable: true, Options: json.RawMessage("[]")},
			{Name: secondParameterName, Description: secondParameterDescription, Mutable: true, Options: json.RawMessage("[]")},
			{Name: immutableParameterName, Description: immutableParameterDescription, Mutable: false, Options: json.RawMessage("[]")},
			{Name: newImmutableParameterName, Description: newImmutableParameterDescription, Mutable: false, DefaultValue: "12345", Options: json.RawMessage("[]")},
		}

		nextBuildParameters := []codersdk.WorkspaceBuildParameter{}
		expectedParams := map[string]string{
			firstParameterName:        firstParameterValue,
			secondParameterName:       secondParameterValue,
			immutableParameterName:    immutableParameterValue,
			newImmutableParameterName: "12345",
		}

		mDB := expectDB(t,
			// Inputs
			withTemplate,
			withActiveVersion(version2params),
			withLastBuildFound,
			withRichParameters(initialBuildParameters),
			withParameterSchemas(activeJobID, nil),

			// Outputs
			expectProvisionerJob(func(job database.InsertProvisionerJobParams) {}),
			withInTx,
			expectBuild(func(bld database.InsertWorkspaceBuildParams) {}),
			expectBuildParameters(func(params database.InsertWorkspaceBuildParametersParams) {
				asrt.Len(params.Name, len(expectedParams))
				for i := range params.Name {
					value, ok := expectedParams[params.Name[i]]
					asrt.True(ok, "unexpected name %s", params.Name[i])
					asrt.Equal(value, params.Value[i])
				}
			}),
			withBuild,
		)

		ws := database.Workspace{ID: workspaceID, TemplateID: templateID, OwnerID: userID}
		uut := wsbuilder.New(ws, database.WorkspaceTransitionStart).
			RichParameterValues(nextBuildParameters).
			VersionID(activeVersionID)
		_, _, err := uut.Build(ctx, mDB, nil, audit.WorkspaceBuildBaggage{})
		req.NoError(err)
	})
}

type txExpect func(mTx *dbmock.MockStore)

func expectDB(t *testing.T, opts ...txExpect) *dbmock.MockStore {
	t.Helper()
	ctrl := gomock.NewController(t)
	mDB := dbmock.NewMockStore(ctrl)
	mTx := dbmock.NewMockStore(ctrl)

	// we expect to be run in a transaction; we use mTx to record the
	// "in transaction" calls.
	mDB.EXPECT().InTx(
		gomock.Any(), gomock.Eq(&sql.TxOptions{Isolation: sql.LevelRepeatableRead}),
	).
		DoAndReturn(func(f func(database.Store) error, _ *sql.TxOptions) error {
			err := f(mTx)
			return err
		})

	// txExpect args set up the expectations for what happens in the transaction.
	for _, o := range opts {
		o(mTx)
	}
	return mDB
}

func withTemplate(mTx *dbmock.MockStore) {
	mTx.EXPECT().GetTemplateByID(gomock.Any(), templateID).
		Times(1).
		Return(database.Template{
			ID:              templateID,
			OrganizationID:  orgID,
			Provisioner:     database.ProvisionerTypeTerraform,
			ActiveVersionID: activeVersionID,
		}, nil)
}

// withInTx runs the given functions on the same db mock.
func withInTx(mTx *dbmock.MockStore) {
	mTx.EXPECT().InTx(gomock.Any(), gomock.Any()).Times(1).DoAndReturn(
		func(f func(store database.Store) error, _ *sql.TxOptions) error {
			return f(mTx)
		},
	)
}

func withActiveVersion(params []database.TemplateVersionParameter) func(mTx *dbmock.MockStore) {
	return func(mTx *dbmock.MockStore) {
		mTx.EXPECT().GetTemplateVersionByID(gomock.Any(), activeVersionID).
			Times(1).
			Return(database.TemplateVersion{
				ID:             activeVersionID,
				TemplateID:     uuid.NullUUID{UUID: templateID, Valid: true},
				OrganizationID: orgID,
				Name:           "active",
				JobID:          activeJobID,
			}, nil)

		mTx.EXPECT().GetProvisionerJobByID(gomock.Any(), activeJobID).
			Times(1).Return(database.ProvisionerJob{
			ID:             activeJobID,
			OrganizationID: orgID,
			InitiatorID:    userID,
			Provisioner:    database.ProvisionerTypeTerraform,
			StorageMethod:  database.ProvisionerStorageMethodFile,
			Type:           database.ProvisionerJobTypeTemplateVersionImport,
			Input:          nil,
			Tags: database.StringMap{
				"version":               "active",
				provisionersdk.TagScope: provisionersdk.ScopeUser,
			},
			FileID:      activeFileID,
			StartedAt:   sql.NullTime{Time: dbtime.Now(), Valid: true},
			UpdatedAt:   time.Now(),
			CompletedAt: sql.NullTime{Time: dbtime.Now(), Valid: true},
		}, nil)
		paramsCall := mTx.EXPECT().GetTemplateVersionParameters(gomock.Any(), activeVersionID).
			Times(1)
		if len(params) > 0 {
			paramsCall.Return(params, nil)
		} else {
			paramsCall.Return(nil, sql.ErrNoRows)
		}
	}
}

func withInactiveVersion(params []database.TemplateVersionParameter) func(mTx *dbmock.MockStore) {
	return func(mTx *dbmock.MockStore) {
		mTx.EXPECT().GetTemplateVersionByID(gomock.Any(), inactiveVersionID).
			Times(1).
			Return(database.TemplateVersion{
				ID:             inactiveVersionID,
				TemplateID:     uuid.NullUUID{UUID: templateID, Valid: true},
				OrganizationID: orgID,
				Name:           "inactive",
				JobID:          inactiveJobID,
			}, nil)

		mTx.EXPECT().GetProvisionerJobByID(gomock.Any(), inactiveJobID).
			Times(1).Return(database.ProvisionerJob{
			ID:             inactiveJobID,
			OrganizationID: orgID,
			InitiatorID:    userID,
			Provisioner:    database.ProvisionerTypeTerraform,
			StorageMethod:  database.ProvisionerStorageMethodFile,
			Type:           database.ProvisionerJobTypeTemplateVersionImport,
			Input:          nil,
			Tags: database.StringMap{
				"version":               "inactive",
				provisionersdk.TagScope: provisionersdk.ScopeUser,
			},
			FileID:      inactiveFileID,
			StartedAt:   sql.NullTime{Time: dbtime.Now(), Valid: true},
			UpdatedAt:   time.Now(),
			CompletedAt: sql.NullTime{Time: dbtime.Now(), Valid: true},
		}, nil)
		paramsCall := mTx.EXPECT().GetTemplateVersionParameters(gomock.Any(), inactiveVersionID).
			Times(1)
		if len(params) > 0 {
			paramsCall.Return(params, nil)
		} else {
			paramsCall.Return(nil, sql.ErrNoRows)
		}
	}
}

func withLastBuildFound(mTx *dbmock.MockStore) {
	mTx.EXPECT().GetLatestWorkspaceBuildByWorkspaceID(gomock.Any(), workspaceID).
		Times(1).
		Return(database.WorkspaceBuild{
			ID:                lastBuildID,
			WorkspaceID:       workspaceID,
			TemplateVersionID: inactiveVersionID,
			BuildNumber:       1,
			Transition:        database.WorkspaceTransitionStart,
			InitiatorID:       userID,
			JobID:             lastBuildJobID,
			ProvisionerState:  []byte("last build state"),
			Reason:            database.BuildReasonInitiator,
		}, nil)

	mTx.EXPECT().GetProvisionerJobByID(gomock.Any(), lastBuildJobID).
		Times(1).
		Return(database.ProvisionerJob{
			ID:             lastBuildJobID,
			OrganizationID: orgID,
			InitiatorID:    userID,
			Provisioner:    database.ProvisionerTypeTerraform,
			StorageMethod:  database.ProvisionerStorageMethodFile,
			FileID:         inactiveFileID,
			Type:           database.ProvisionerJobTypeWorkspaceBuild,
			StartedAt:      sql.NullTime{Time: dbtime.Now(), Valid: true},
			UpdatedAt:      time.Now(),
			CompletedAt:    sql.NullTime{Time: dbtime.Now(), Valid: true},
		}, nil)
}

func withLastBuildNotFound(mTx *dbmock.MockStore) {
	mTx.EXPECT().GetLatestWorkspaceBuildByWorkspaceID(gomock.Any(), workspaceID).
		Times(1).
		Return(database.WorkspaceBuild{}, sql.ErrNoRows)
}

func withParameterSchemas(jobID uuid.UUID, schemas []database.ParameterSchema) func(mTx *dbmock.MockStore) {
	return func(mTx *dbmock.MockStore) {
		c := mTx.EXPECT().GetParameterSchemasByJobID(
			gomock.Any(),
			jobID).
			Times(1)
		if len(schemas) > 0 {
			c.Return(schemas, nil)
		} else {
			c.Return(nil, sql.ErrNoRows)
		}
	}
}

func withRichParameters(params []database.WorkspaceBuildParameter) func(mTx *dbmock.MockStore) {
	return func(mTx *dbmock.MockStore) {
		c := mTx.EXPECT().GetWorkspaceBuildParameters(gomock.Any(), lastBuildID).
			Times(1)
		if len(params) > 0 {
			c.Return(params, nil)
		} else {
			c.Return(nil, sql.ErrNoRows)
		}
	}
}

// Since there is expected to be only one each of job, build, and build-parameters inserted, instead
// of building matchers, we match any call and then assert its parameters.  This will feel
// more familiar to the way we write other tests.

// expectProvisionerJob captures a call to InsertProvisionerJob and runs the provided assertions
// against it.
func expectProvisionerJob(
	assertions func(job database.InsertProvisionerJobParams),
) func(mTx *dbmock.MockStore) {
	return func(mTx *dbmock.MockStore) {
		mTx.EXPECT().InsertProvisionerJob(gomock.Any(), gomock.Any()).
			Times(1).
			DoAndReturn(
				func(ctx context.Context, params database.InsertProvisionerJobParams) (database.ProvisionerJob, error) {
					assertions(params)
					// there is no point copying anything other than the ID, since this object is just
					// returned to our test code, and we've already asserted what we care about.
					return database.ProvisionerJob{ID: params.ID}, nil
				},
			)
	}
}

func withBuild(mTx *dbmock.MockStore) {
	mTx.EXPECT().GetWorkspaceBuildByID(gomock.Any(), gomock.Any()).Times(1).
		DoAndReturn(func(ctx context.Context, id uuid.UUID) (database.WorkspaceBuild, error) {
			return database.WorkspaceBuild{ID: id}, nil
		})
}

// expectBuild captures a call to InsertWorkspaceBuild and runs the provided assertions
// against it.
func expectBuild(
	assertions func(job database.InsertWorkspaceBuildParams),
) func(mTx *dbmock.MockStore) {
	return func(mTx *dbmock.MockStore) {
		mTx.EXPECT().InsertWorkspaceBuild(gomock.Any(), gomock.Any()).
			Times(1).
			DoAndReturn(
				func(ctx context.Context, params database.InsertWorkspaceBuildParams) error {
					assertions(params)
					return nil
				},
			)
	}
}

// expectBuildParameters captures a call to InsertWorkspaceBuildParameters and runs the provided assertions
// against it.
func expectBuildParameters(
	assertions func(database.InsertWorkspaceBuildParametersParams),
) func(mTx *dbmock.MockStore) {
	return func(mTx *dbmock.MockStore) {
		mTx.EXPECT().InsertWorkspaceBuildParameters(gomock.Any(), gomock.Any()).
			Times(1).
			DoAndReturn(
				func(ctx context.Context, params database.InsertWorkspaceBuildParametersParams) error {
					assertions(params)
					return nil
				},
			)
	}
}
