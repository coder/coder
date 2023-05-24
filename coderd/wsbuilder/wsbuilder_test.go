package wsbuilder_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbmock"
	"github.com/coder/coder/coderd/database/dbtype"
	"github.com/coder/coder/coderd/provisionerdserver"
	"github.com/coder/coder/coderd/wsbuilder"
	"github.com/coder/coder/codersdk"
)

var (
	// use fixed IDs so logs are easier to read
	templateID         = uuid.MustParse("12341234-0000-0000-0001-000000000000")
	activeVersionID    = uuid.MustParse("12341234-0000-0000-0002-000000000000")
	inactiveVersionID  = uuid.MustParse("12341234-0000-0000-0003-000000000000")
	activeJobID        = uuid.MustParse("12341234-0000-0000-0004-000000000000")
	inactiveJobID      = uuid.MustParse("12341234-0000-0000-0005-000000000000")
	orgID              = uuid.MustParse("12341234-0000-0000-0006-000000000000")
	workspaceID        = uuid.MustParse("12341234-0000-0000-0007-000000000000")
	userID             = uuid.MustParse("12341234-0000-0000-0008-000000000000")
	activeFileID       = uuid.MustParse("12341234-0000-0000-0009-000000000000")
	inactiveFileID     = uuid.MustParse("12341234-0000-0000-000a-000000000000")
	lastBuildID        = uuid.MustParse("12341234-0000-0000-000b-000000000000")
	lastBuildJobID     = uuid.MustParse("12341234-0000-0000-000c-000000000000")
	otherUserID        = uuid.MustParse("12341234-0000-0000-000d-000000000000")
	notReplacedParamID = uuid.MustParse("12341234-0000-0000-000e-000000000000")
	replacedParamID    = uuid.MustParse("12341234-0000-0000-000f-000000000000")
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
		withLegacyParameters(nil), withRichParameters(nil),

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
		expectBuildParameters(func(params database.InsertWorkspaceBuildParametersParams) {
			asrt.Equal(buildID, params.WorkspaceBuildID)
			asrt.Empty(params.Name)
			asrt.Empty(params.Value)
		}),
	)

	ws := database.Workspace{ID: workspaceID, TemplateID: templateID, OwnerID: userID}
	uut := wsbuilder.New(ws, database.WorkspaceTransitionStart)
	_, _, err := uut.Build(ctx, mDB, nil)
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
		withLegacyParameters(nil), withRichParameters(nil),

		// Outputs
		expectProvisionerJob(func(job database.InsertProvisionerJobParams) {
			asrt.Equal(otherUserID, job.InitiatorID)
		}),
		expectBuild(func(bld database.InsertWorkspaceBuildParams) {
			asrt.Equal(otherUserID, bld.InitiatorID)
		}),
		expectBuildParameters(func(params database.InsertWorkspaceBuildParametersParams) {
		}),
	)

	ws := database.Workspace{ID: workspaceID, TemplateID: templateID, OwnerID: userID}
	uut := wsbuilder.New(ws, database.WorkspaceTransitionStart).Initiator(otherUserID)
	_, _, err := uut.Build(ctx, mDB, nil)
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
		withLegacyParameters(nil), withRichParameters(nil),

		// Outputs
		expectProvisionerJob(func(job database.InsertProvisionerJobParams) {
		}),
		expectBuild(func(bld database.InsertWorkspaceBuildParams) {
			asrt.Equal(database.BuildReasonAutostart, bld.Reason)
		}),
		expectBuildParameters(func(params database.InsertWorkspaceBuildParametersParams) {
		}),
	)

	ws := database.Workspace{ID: workspaceID, TemplateID: templateID, OwnerID: userID}
	uut := wsbuilder.New(ws, database.WorkspaceTransitionStart).Reason(database.BuildReasonAutostart)
	_, _, err := uut.Build(ctx, mDB, nil)
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
		withLegacyParameters(nil),
		// previous rich parameters are not queried because there is no previous build.

		// Outputs
		expectProvisionerJob(func(job database.InsertProvisionerJobParams) {
			asrt.Equal(activeFileID, job.FileID)
		}),
		expectBuild(func(bld database.InsertWorkspaceBuildParams) {
			asrt.Equal(activeVersionID, bld.TemplateVersionID)
			// no previous build...
			asrt.Equal(int32(1), bld.BuildNumber)
			asrt.Len(bld.ProvisionerState, 0)
		}),
		expectBuildParameters(func(params database.InsertWorkspaceBuildParametersParams) {
		}),
	)

	ws := database.Workspace{ID: workspaceID, TemplateID: templateID, OwnerID: userID}
	uut := wsbuilder.New(ws, database.WorkspaceTransitionStart).ActiveVersion()
	_, _, err := uut.Build(ctx, mDB, nil)
	req.NoError(err)
}

func TestBuilder_LegacyParams(t *testing.T) {
	t.Parallel()
	req := require.New(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	oldParams := []database.ParameterValue{
		{Name: "not-replaced", SourceValue: "nr", ID: notReplacedParamID},
		{Name: "replaced", SourceValue: "r", ID: replacedParamID},
	}
	newParams := []codersdk.CreateParameterRequest{
		{Name: "replaced", SourceValue: "s"},
		{Name: "new", SourceValue: "n"},
	}

	mDB := expectDB(t,
		// Inputs
		withTemplate,
		withActiveVersion(nil),
		withLastBuildFound,
		withLegacyParameters(oldParams),
		withRichParameters(nil),

		// Outputs
		expectProvisionerJob(func(job database.InsertProvisionerJobParams) {
		}),
		expectBuild(func(bld database.InsertWorkspaceBuildParams) {
		}),
		expectBuildParameters(func(params database.InsertWorkspaceBuildParametersParams) {
		}),
		expectReplacedParam(replacedParamID, "replaced", "s"),
		expectInsertedParam("new", "n"),
	)

	ws := database.Workspace{ID: workspaceID, TemplateID: templateID, OwnerID: userID}
	uut := wsbuilder.New(ws, database.WorkspaceTransitionStart).ActiveVersion().LegacyParameterValues(newParams)
	_, _, err := uut.Build(ctx, mDB, nil)
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
			withLegacyParameters(nil),
			withRichParameters(initialBuildParameters),

			// Outputs
			expectProvisionerJob(func(job database.InsertProvisionerJobParams) {}),
			expectBuild(func(bld database.InsertWorkspaceBuildParams) {}),
			expectBuildParameters(func(params database.InsertWorkspaceBuildParametersParams) {
				asrt.Len(params.Name, len(expectedParams))
				for i := range params.Name {
					value, ok := expectedParams[params.Name[i]]
					asrt.True(ok, "unexpected name %s", params.Name[i])
					asrt.Equal(value, params.Value[i])
				}
			}),
		)

		ws := database.Workspace{ID: workspaceID, TemplateID: templateID, OwnerID: userID}
		uut := wsbuilder.New(ws, database.WorkspaceTransitionStart).RichParameterValues(nextBuildParameters)
		_, _, err := uut.Build(ctx, mDB, nil)
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
			withLegacyParameters(nil),
			withRichParameters(initialBuildParameters),

			// Outputs
			expectProvisionerJob(func(job database.InsertProvisionerJobParams) {}),
			expectBuild(func(bld database.InsertWorkspaceBuildParams) {}),
			expectBuildParameters(func(params database.InsertWorkspaceBuildParametersParams) {
				asrt.Len(params.Name, len(expectedParams))
				for i := range params.Name {
					value, ok := expectedParams[params.Name[i]]
					asrt.True(ok, "unexpected name %s", params.Name[i])
					asrt.Equal(value, params.Value[i])
				}
			}),
		)

		ws := database.Workspace{ID: workspaceID, TemplateID: templateID, OwnerID: userID}
		uut := wsbuilder.New(ws, database.WorkspaceTransitionStart).RichParameterValues(nextBuildParameters)
		_, _, err := uut.Build(ctx, mDB, nil)
		req.NoError(err)
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
			withLegacyParameters(nil),
			withRichParameters(initialBuildParameters),

			// Outputs
			expectProvisionerJob(func(job database.InsertProvisionerJobParams) {}),
			expectBuild(func(bld database.InsertWorkspaceBuildParams) {}),
			// no build parameters, since we hit an error validating.
			// expectBuildParameters(func(params database.InsertWorkspaceBuildParametersParams) {}),
		)

		ws := database.Workspace{ID: workspaceID, TemplateID: templateID, OwnerID: userID}
		uut := wsbuilder.New(ws, database.WorkspaceTransitionStart).RichParameterValues(nextBuildParameters)
		_, _, err := uut.Build(ctx, mDB, nil)
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
			withLegacyParameters(nil),
			withRichParameters(initialBuildParameters),

			// Outputs
			expectProvisionerJob(func(job database.InsertProvisionerJobParams) {}),
			expectBuild(func(bld database.InsertWorkspaceBuildParams) {}),
			expectBuildParameters(func(params database.InsertWorkspaceBuildParametersParams) {
				asrt.Len(params.Name, len(expectedParams))
				for i := range params.Name {
					value, ok := expectedParams[params.Name[i]]
					asrt.True(ok, "unexpected name %s", params.Name[i])
					asrt.Equal(value, params.Value[i])
				}
			}),
		)

		ws := database.Workspace{ID: workspaceID, TemplateID: templateID, OwnerID: userID}
		uut := wsbuilder.New(ws, database.WorkspaceTransitionStart).
			RichParameterValues(nextBuildParameters).
			VersionID(activeVersionID)
		_, _, err := uut.Build(ctx, mDB, nil)
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
			withLegacyParameters(nil),
			withRichParameters(initialBuildParameters),

			// Outputs
			expectProvisionerJob(func(job database.InsertProvisionerJobParams) {}),
			expectBuild(func(bld database.InsertWorkspaceBuildParams) {}),
			expectBuildParameters(func(params database.InsertWorkspaceBuildParametersParams) {
				asrt.Len(params.Name, len(expectedParams))
				for i := range params.Name {
					value, ok := expectedParams[params.Name[i]]
					asrt.True(ok, "unexpected name %s", params.Name[i])
					asrt.Equal(value, params.Value[i])
				}
			}),
		)

		ws := database.Workspace{ID: workspaceID, TemplateID: templateID, OwnerID: userID}
		uut := wsbuilder.New(ws, database.WorkspaceTransitionStart).
			RichParameterValues(nextBuildParameters).
			VersionID(activeVersionID)
		_, _, err := uut.Build(ctx, mDB, nil)
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
			withLegacyParameters(nil),
			withRichParameters(initialBuildParameters),

			// Outputs
			expectProvisionerJob(func(job database.InsertProvisionerJobParams) {}),
			expectBuild(func(bld database.InsertWorkspaceBuildParams) {}),
			expectBuildParameters(func(params database.InsertWorkspaceBuildParametersParams) {
				asrt.Len(params.Name, len(expectedParams))
				for i := range params.Name {
					value, ok := expectedParams[params.Name[i]]
					asrt.True(ok, "unexpected name %s", params.Name[i])
					asrt.Equal(value, params.Value[i])
				}
			}),
		)

		ws := database.Workspace{ID: workspaceID, TemplateID: templateID, OwnerID: userID}
		uut := wsbuilder.New(ws, database.WorkspaceTransitionStart).
			RichParameterValues(nextBuildParameters).
			VersionID(activeVersionID)
		_, _, err := uut.Build(ctx, mDB, nil)
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
			Tags: dbtype.StringMap{
				"version":                   "active",
				provisionerdserver.TagScope: provisionerdserver.ScopeUser,
			},
			FileID:      activeFileID,
			StartedAt:   sql.NullTime{Time: database.Now(), Valid: true},
			UpdatedAt:   time.Now(),
			CompletedAt: sql.NullTime{Time: database.Now(), Valid: true},
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
			Tags: dbtype.StringMap{
				"version":                   "inactive",
				provisionerdserver.TagScope: provisionerdserver.ScopeUser,
			},
			FileID:      inactiveFileID,
			StartedAt:   sql.NullTime{Time: database.Now(), Valid: true},
			UpdatedAt:   time.Now(),
			CompletedAt: sql.NullTime{Time: database.Now(), Valid: true},
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
			StartedAt:      sql.NullTime{Time: database.Now(), Valid: true},
			UpdatedAt:      time.Now(),
			CompletedAt:    sql.NullTime{Time: database.Now(), Valid: true},
		}, nil)
}

func withLastBuildNotFound(mTx *dbmock.MockStore) {
	mTx.EXPECT().GetLatestWorkspaceBuildByWorkspaceID(gomock.Any(), workspaceID).
		Times(1).
		Return(database.WorkspaceBuild{}, sql.ErrNoRows)
}

func withLegacyParameters(params []database.ParameterValue) func(mTx *dbmock.MockStore) {
	return func(mTx *dbmock.MockStore) {
		c := mTx.EXPECT().ParameterValues(
			gomock.Any(),
			database.ParameterValuesParams{
				Scopes:   []database.ParameterScope{database.ParameterScopeWorkspace},
				ScopeIds: []uuid.UUID{workspaceID},
			}).
			Times(1)
		if len(params) > 0 {
			c.Return(params, nil)
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

// expectBuild captures a call to InsertWorkspaceBuild and runs the provided assertions
// against it.
func expectBuild(
	assertions func(job database.InsertWorkspaceBuildParams),
) func(mTx *dbmock.MockStore) {
	return func(mTx *dbmock.MockStore) {
		mTx.EXPECT().InsertWorkspaceBuild(gomock.Any(), gomock.Any()).
			Times(1).
			DoAndReturn(
				func(ctx context.Context, params database.InsertWorkspaceBuildParams) (database.WorkspaceBuild, error) {
					assertions(params)
					// there is no point copying anything other than the ID, since this object is just
					// returned to our test code, and we've already asserted what we care about.
					return database.WorkspaceBuild{ID: params.ID}, nil
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

type insertParameterMatcher struct {
	name  string
	value string
}

func (m insertParameterMatcher) Matches(x interface{}) bool {
	p, ok := x.(database.InsertParameterValueParams)
	if !ok {
		return false
	}
	if p.Name != m.name {
		return false
	}
	return p.SourceValue == m.value
}

func (m insertParameterMatcher) String() string {
	return fmt.Sprintf("ParameterValue %s=%s", m.name, m.value)
}

func expectReplacedParam(oldID uuid.UUID, name, newValue string) func(store *dbmock.MockStore) {
	return func(mTx *dbmock.MockStore) {
		del := mTx.EXPECT().DeleteParameterValueByID(gomock.Any(), oldID).
			Times(1).
			Return(nil)
		mTx.EXPECT().InsertParameterValue(gomock.Any(), insertParameterMatcher{name, newValue}).
			Times(1).
			After(del).
			Return(database.ParameterValue{}, nil)
	}
}

func expectInsertedParam(name, newValue string) func(store *dbmock.MockStore) {
	return func(mTx *dbmock.MockStore) {
		mTx.EXPECT().InsertParameterValue(gomock.Any(), insertParameterMatcher{name, newValue}).
			Times(1).
			Return(database.ParameterValue{}, nil)
	}
}
