package wsbuilder_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/files"
	"github.com/coder/coder/v2/coderd/httpapi/httperror"
	"github.com/coder/coder/v2/coderd/provisionerdserver"
	"github.com/coder/coder/v2/coderd/wsbuilder"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisionersdk"
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
	presetID          = uuid.MustParse("12341234-0000-0000-000e-000000000000")
	taskID            = uuid.MustParse("12341234-0000-0000-000f-000000000000")
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
		withTemplateVersionVariables(inactiveVersionID, nil),
		withRichParameters(nil),
		withParameterSchemas(inactiveJobID, nil),
		withWorkspaceTags(inactiveVersionID, nil),
		withProvisionerDaemons([]database.GetEligibleProvisionerDaemonsByProvisionerJobIDsRow{}),

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
		expectFindMatchingPresetID(uuid.Nil, sql.ErrNoRows),
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
		withNoTask,
		expectBuildParameters(func(params database.InsertWorkspaceBuildParametersParams) {
			asrt.Equal(buildID, params.WorkspaceBuildID)
			asrt.Empty(params.Name)
			asrt.Empty(params.Value)
		}),
	)
	fc := files.New(prometheus.NewRegistry(), &coderdtest.FakeAuthorizer{})

	ws := database.Workspace{ID: workspaceID, TemplateID: templateID, OwnerID: userID}
	uut := wsbuilder.New(ws, database.WorkspaceTransitionStart, wsbuilder.NoopUsageChecker{})
	// nolint: dogsled
	_, _, _, err := uut.Build(ctx, mDB, fc, nil, audit.WorkspaceBuildBaggage{})
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
		withTemplateVersionVariables(inactiveVersionID, nil),
		withRichParameters(nil),
		withParameterSchemas(inactiveJobID, nil),
		withWorkspaceTags(inactiveVersionID, nil),
		withProvisionerDaemons([]database.GetEligibleProvisionerDaemonsByProvisionerJobIDsRow{}),

		// Outputs
		expectProvisionerJob(func(job database.InsertProvisionerJobParams) {
			asrt.Equal(otherUserID, job.InitiatorID)
		}),
		withInTx,
		expectFindMatchingPresetID(uuid.Nil, sql.ErrNoRows),
		expectBuild(func(bld database.InsertWorkspaceBuildParams) {
			asrt.Equal(otherUserID, bld.InitiatorID)
		}),
		expectBuildParameters(func(params database.InsertWorkspaceBuildParametersParams) {
		}),
		withBuild,
		withNoTask,
	)
	fc := files.New(prometheus.NewRegistry(), &coderdtest.FakeAuthorizer{})

	ws := database.Workspace{ID: workspaceID, TemplateID: templateID, OwnerID: userID}
	uut := wsbuilder.New(ws, database.WorkspaceTransitionStart, wsbuilder.NoopUsageChecker{}).
		Initiator(otherUserID)
	// nolint: dogsled
	_, _, _, err := uut.Build(ctx, mDB, fc, nil, audit.WorkspaceBuildBaggage{})
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
		withTemplateVersionVariables(inactiveVersionID, nil),
		withRichParameters(nil),
		withParameterSchemas(inactiveJobID, nil),
		withWorkspaceTags(inactiveVersionID, nil),
		withProvisionerDaemons([]database.GetEligibleProvisionerDaemonsByProvisionerJobIDsRow{}),

		// Outputs
		expectProvisionerJob(func(job database.InsertProvisionerJobParams) {
			asrt.Contains(string(job.TraceMetadata.RawMessage), "ip=127.0.0.1")
		}),
		withInTx,
		expectFindMatchingPresetID(uuid.Nil, sql.ErrNoRows),
		expectBuild(func(bld database.InsertWorkspaceBuildParams) {
		}),
		expectBuildParameters(func(params database.InsertWorkspaceBuildParametersParams) {
		}),
		withBuild,
		withNoTask,
	)
	fc := files.New(prometheus.NewRegistry(), &coderdtest.FakeAuthorizer{})

	ws := database.Workspace{ID: workspaceID, TemplateID: templateID, OwnerID: userID}
	uut := wsbuilder.New(ws, database.WorkspaceTransitionStart, wsbuilder.NoopUsageChecker{}).
		Initiator(otherUserID)
	// nolint: dogsled
	_, _, _, err := uut.Build(ctx, mDB, fc, nil, audit.WorkspaceBuildBaggage{IP: "127.0.0.1"})
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
		withTemplateVersionVariables(inactiveVersionID, nil),
		withRichParameters(nil),
		withParameterSchemas(inactiveJobID, nil),
		withWorkspaceTags(inactiveVersionID, nil),
		withProvisionerDaemons([]database.GetEligibleProvisionerDaemonsByProvisionerJobIDsRow{}),

		// Outputs
		expectProvisionerJob(func(_ database.InsertProvisionerJobParams) {
		}),
		withInTx,
		expectFindMatchingPresetID(uuid.Nil, sql.ErrNoRows),
		expectBuild(func(bld database.InsertWorkspaceBuildParams) {
			asrt.Equal(database.BuildReasonAutostart, bld.Reason)
		}),
		expectBuildParameters(func(params database.InsertWorkspaceBuildParametersParams) {
		}),
		withBuild,
		withNoTask,
	)
	fc := files.New(prometheus.NewRegistry(), &coderdtest.FakeAuthorizer{})

	ws := database.Workspace{ID: workspaceID, TemplateID: templateID, OwnerID: userID}
	uut := wsbuilder.New(ws, database.WorkspaceTransitionStart, wsbuilder.NoopUsageChecker{}).
		Reason(database.BuildReasonAutostart)
	// nolint: dogsled
	_, _, _, err := uut.Build(ctx, mDB, fc, nil, audit.WorkspaceBuildBaggage{})
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
		withTemplateVersionVariables(activeVersionID, nil),
		withParameterSchemas(activeJobID, nil),
		withWorkspaceTags(activeVersionID, nil),
		withProvisionerDaemons([]database.GetEligibleProvisionerDaemonsByProvisionerJobIDsRow{}),
		// previous rich parameters are not queried because there is no previous build.

		// Outputs
		expectProvisionerJob(func(job database.InsertProvisionerJobParams) {
			asrt.Equal(activeFileID, job.FileID)
		}),

		withInTx,
		expectFindMatchingPresetID(uuid.Nil, sql.ErrNoRows),
		expectBuild(func(bld database.InsertWorkspaceBuildParams) {
			asrt.Equal(activeVersionID, bld.TemplateVersionID)
			// no previous build...
			asrt.Equal(int32(1), bld.BuildNumber)
			asrt.Len(bld.ProvisionerState, 0)
		}),
		expectBuildParameters(func(params database.InsertWorkspaceBuildParametersParams) {
		}),
		withBuild,
		withNoTask,
	)
	fc := files.New(prometheus.NewRegistry(), &coderdtest.FakeAuthorizer{})

	ws := database.Workspace{ID: workspaceID, TemplateID: templateID, OwnerID: userID}
	uut := wsbuilder.New(ws, database.WorkspaceTransitionStart, wsbuilder.NoopUsageChecker{}).
		ActiveVersion()
	// nolint: dogsled
	_, _, _, err := uut.Build(ctx, mDB, fc, nil, audit.WorkspaceBuildBaggage{})
	req.NoError(err)
}

func TestWorkspaceBuildWithTags(t *testing.T) {
	t.Parallel()

	asrt := assert.New(t)
	req := require.New(t)

	workspaceTags := []database.TemplateVersionWorkspaceTag{
		{
			Key:   "fruits_tag",
			Value: "data.coder_parameter.number_of_apples.value + data.coder_parameter.number_of_oranges.value",
		},
		{
			Key:   "cluster_tag",
			Value: `"best_developers"`,
		},
		{
			Key:   "project_tag",
			Value: `"${data.coder_parameter.project.value}+12345"`,
		},
		{
			Key:   "team_tag",
			Value: `data.coder_parameter.team.value`,
		},
		{
			Key:   "yes_or_no",
			Value: `data.coder_parameter.is_debug_build.value`,
		},
		{
			Key:   "actually_no",
			Value: `!data.coder_parameter.is_debug_build.value`,
		},
		{
			Key:   "is_debug_build",
			Value: `data.coder_parameter.is_debug_build.value == "true" ? "in-debug-mode" : "no-debug"`,
		},
		{
			Key:   "variable_tag",
			Value: `var.tag`,
		},
		{
			Key:   "another_variable_tag",
			Value: `var.tag2`,
		},
	}

	richParameters := []database.TemplateVersionParameter{
		// Parameters can be mutable although it is discouraged as the workspace can be moved between provisioner nodes.
		{Name: "project", Description: "This is first parameter", Mutable: true, Options: json.RawMessage("[]")},
		{Name: "team", Description: "This is second parameter", Mutable: true, DefaultValue: "godzilla", Options: json.RawMessage("[]")},
		{Name: "is_debug_build", Type: "bool", Description: "This is third parameter", Mutable: false, DefaultValue: "false", Options: json.RawMessage("[]")},
		{Name: "number_of_apples", Type: "number", Description: "This is fourth parameter", Mutable: false, DefaultValue: "4", Options: json.RawMessage("[]")},
		{Name: "number_of_oranges", Type: "number", Description: "This is fifth parameter", Mutable: false, DefaultValue: "6", Options: json.RawMessage("[]")},
	}

	templateVersionVariables := []database.TemplateVersionVariable{
		{Name: "tag", Description: "This is a variable tag", TemplateVersionID: inactiveVersionID, Type: "string", DefaultValue: "default-value", Value: "my-value"},
		{Name: "tag2", Description: "This is another variable tag", TemplateVersionID: inactiveVersionID, Type: "string", DefaultValue: "default-value-2", Value: ""},
	}

	buildParameters := []codersdk.WorkspaceBuildParameter{
		{Name: "project", Value: "foobar-foobaz"},
		{Name: "is_debug_build", Value: "true"},
		// Parameters "team", "number_of_apples", "number_of_oranges" are skipped, so default value is selected
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mDB := expectDB(t,
		// Inputs
		withTemplate,
		withInactiveVersion(richParameters),
		withLastBuildFound,
		withTemplateVersionVariables(inactiveVersionID, templateVersionVariables),
		withRichParameters(nil),
		withParameterSchemas(inactiveJobID, nil),
		withWorkspaceTags(inactiveVersionID, workspaceTags),
		withProvisionerDaemons([]database.GetEligibleProvisionerDaemonsByProvisionerJobIDsRow{}),

		// Outputs
		expectProvisionerJob(func(job database.InsertProvisionerJobParams) {
			asrt.Len(job.Tags, 12)

			expected := database.StringMap{
				"actually_no":          "false",
				"cluster_tag":          "best_developers",
				"fruits_tag":           "10",
				"is_debug_build":       "in-debug-mode",
				"project_tag":          "foobar-foobaz+12345",
				"team_tag":             "godzilla",
				"yes_or_no":            "true",
				"variable_tag":         "my-value",
				"another_variable_tag": "default-value-2",

				"scope":   "user",
				"version": "inactive",
				"owner":   userID.String(),
			}
			asrt.Equal(job.Tags, expected)
		}),
		withInTx,
		expectBuild(func(_ database.InsertWorkspaceBuildParams) {}),
		expectBuildParameters(func(_ database.InsertWorkspaceBuildParametersParams) {
		}),
		withBuild,
		withNoTask,
		expectFindMatchingPresetID(uuid.Nil, sql.ErrNoRows),
	)
	fc := files.New(prometheus.NewRegistry(), &coderdtest.FakeAuthorizer{})

	ws := database.Workspace{ID: workspaceID, TemplateID: templateID, OwnerID: userID}
	uut := wsbuilder.New(ws, database.WorkspaceTransitionStart, wsbuilder.NoopUsageChecker{}).
		RichParameterValues(buildParameters)
	// nolint: dogsled
	_, _, _, err := uut.Build(ctx, mDB, fc, nil, audit.WorkspaceBuildBaggage{})
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
			withTemplateVersionVariables(inactiveVersionID, nil),
			withRichParameters(initialBuildParameters),
			withParameterSchemas(inactiveJobID, nil),
			withWorkspaceTags(inactiveVersionID, nil),
			withProvisionerDaemons([]database.GetEligibleProvisionerDaemonsByProvisionerJobIDsRow{}),

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
			withNoTask,
			expectFindMatchingPresetID(uuid.Nil, sql.ErrNoRows),
		)
		fc := files.New(prometheus.NewRegistry(), &coderdtest.FakeAuthorizer{})

		ws := database.Workspace{ID: workspaceID, TemplateID: templateID, OwnerID: userID}
		uut := wsbuilder.New(ws, database.WorkspaceTransitionStart, wsbuilder.NoopUsageChecker{}).
			RichParameterValues(nextBuildParameters)
		// nolint: dogsled
		_, _, _, err := uut.Build(ctx, mDB, fc, nil, audit.WorkspaceBuildBaggage{})
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
			withTemplateVersionVariables(inactiveVersionID, nil),
			withRichParameters(initialBuildParameters),
			withParameterSchemas(inactiveJobID, nil),
			withWorkspaceTags(inactiveVersionID, nil),
			withProvisionerDaemons([]database.GetEligibleProvisionerDaemonsByProvisionerJobIDsRow{}),

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
			withNoTask,
			expectFindMatchingPresetID(uuid.Nil, sql.ErrNoRows),
		)
		fc := files.New(prometheus.NewRegistry(), &coderdtest.FakeAuthorizer{})

		ws := database.Workspace{ID: workspaceID, TemplateID: templateID, OwnerID: userID}
		uut := wsbuilder.New(ws, database.WorkspaceTransitionStart, wsbuilder.NoopUsageChecker{}).
			RichParameterValues(nextBuildParameters)
		// nolint: dogsled
		_, _, _, err := uut.Build(ctx, mDB, fc, nil, audit.WorkspaceBuildBaggage{})
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
			withNoTask,
			withInactiveVersionNoParams(),
			withLastBuildFound,
			withTemplateVersionVariables(inactiveVersionID, nil),
			withParameterSchemas(inactiveJobID, schemas),
			withWorkspaceTags(inactiveVersionID, nil),
		)
		fc := files.New(prometheus.NewRegistry(), &coderdtest.FakeAuthorizer{})

		ws := database.Workspace{ID: workspaceID, TemplateID: templateID, OwnerID: userID}
		uut := wsbuilder.New(ws, database.WorkspaceTransitionStart, wsbuilder.NoopUsageChecker{})
		// nolint: dogsled
		_, _, _, err := uut.Build(ctx, mDB, fc, nil, audit.WorkspaceBuildBaggage{})
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
			withNoTask,
			withTemplateVersionVariables(inactiveVersionID, nil),
			withRichParameters(initialBuildParameters),
			withParameterSchemas(inactiveJobID, nil),
			withWorkspaceTags(inactiveVersionID, nil),

			// Outputs
			// no transaction, since we failed fast while validation build parameters
		)
		fc := files.New(prometheus.NewRegistry(), &coderdtest.FakeAuthorizer{})

		ws := database.Workspace{ID: workspaceID, TemplateID: templateID, OwnerID: userID}
		uut := wsbuilder.New(ws, database.WorkspaceTransitionStart, wsbuilder.NoopUsageChecker{}).
			RichParameterValues(nextBuildParameters)
		// nolint: dogsled
		_, _, _, err := uut.Build(ctx, mDB, fc, nil, audit.WorkspaceBuildBaggage{})
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
			withTemplateVersionVariables(activeVersionID, nil),
			withRichParameters(initialBuildParameters),
			withParameterSchemas(activeJobID, nil),
			withWorkspaceTags(activeVersionID, nil),
			withProvisionerDaemons([]database.GetEligibleProvisionerDaemonsByProvisionerJobIDsRow{}),

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
			withNoTask,
			expectFindMatchingPresetID(uuid.Nil, sql.ErrNoRows),
		)
		fc := files.New(prometheus.NewRegistry(), &coderdtest.FakeAuthorizer{})

		ws := database.Workspace{ID: workspaceID, TemplateID: templateID, OwnerID: userID}
		uut := wsbuilder.New(ws, database.WorkspaceTransitionStart, wsbuilder.NoopUsageChecker{}).
			RichParameterValues(nextBuildParameters).
			VersionID(activeVersionID)
		_, _, _, err := uut.Build(ctx, mDB, fc, nil, audit.WorkspaceBuildBaggage{})
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
			withTemplateVersionVariables(activeVersionID, nil),
			withRichParameters(initialBuildParameters),
			withParameterSchemas(activeJobID, nil),
			withWorkspaceTags(activeVersionID, nil),
			withProvisionerDaemons([]database.GetEligibleProvisionerDaemonsByProvisionerJobIDsRow{}),

			// Outputs
			expectFindMatchingPresetID(uuid.Nil, sql.ErrNoRows),
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
			withNoTask,
		)
		fc := files.New(prometheus.NewRegistry(), &coderdtest.FakeAuthorizer{})

		ws := database.Workspace{ID: workspaceID, TemplateID: templateID, OwnerID: userID}
		uut := wsbuilder.New(ws, database.WorkspaceTransitionStart, wsbuilder.NoopUsageChecker{}).
			RichParameterValues(nextBuildParameters).
			VersionID(activeVersionID)
		_, _, _, err := uut.Build(ctx, mDB, fc, nil, audit.WorkspaceBuildBaggage{})
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
			withTemplateVersionVariables(activeVersionID, nil),
			withRichParameters(initialBuildParameters),
			withParameterSchemas(activeJobID, nil),
			withWorkspaceTags(activeVersionID, nil),
			withProvisionerDaemons([]database.GetEligibleProvisionerDaemonsByProvisionerJobIDsRow{}),

			// Outputs
			expectFindMatchingPresetID(uuid.Nil, sql.ErrNoRows),
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
			withNoTask,
		)
		fc := files.New(prometheus.NewRegistry(), &coderdtest.FakeAuthorizer{})

		ws := database.Workspace{ID: workspaceID, TemplateID: templateID, OwnerID: userID}
		uut := wsbuilder.New(ws, database.WorkspaceTransitionStart, wsbuilder.NoopUsageChecker{}).
			RichParameterValues(nextBuildParameters).
			VersionID(activeVersionID)
		// nolint: dogsled
		_, _, _, err := uut.Build(ctx, mDB, fc, nil, audit.WorkspaceBuildBaggage{})
		req.NoError(err)
	})
}

func TestWorkspaceBuildWithPreset(t *testing.T) {
	t.Parallel()

	req := require.New(t)
	asrt := assert.New(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var buildID uuid.UUID

	mDB := expectDB(t,
		// Inputs
		withTemplate,
		withActiveVersion(nil),
		// building workspaces using presets with different combinations of parameters
		// is tested at the API layer, in TestWorkspace. Here, it is sufficient to
		// test that the preset is used when provided.
		withTemplateVersionPresetParameters(presetID, nil),
		withLastBuildNotFound,
		withTemplateVersionVariables(activeVersionID, nil),
		withParameterSchemas(activeJobID, nil),
		withWorkspaceTags(activeVersionID, nil),
		withProvisionerDaemons([]database.GetEligibleProvisionerDaemonsByProvisionerJobIDsRow{}),

		// Outputs
		expectProvisionerJob(func(job database.InsertProvisionerJobParams) {
			asrt.Equal(userID, job.InitiatorID)
			asrt.Equal(activeFileID, job.FileID)
			input := provisionerdserver.WorkspaceProvisionJob{}
			err := json.Unmarshal(job.Input, &input)
			req.NoError(err)
			// store build ID for later
			buildID = input.WorkspaceBuildID
		}),

		withInTx,
		expectBuild(func(bld database.InsertWorkspaceBuildParams) {
			asrt.Equal(activeVersionID, bld.TemplateVersionID)
			asrt.Equal(workspaceID, bld.WorkspaceID)
			asrt.Equal(int32(1), bld.BuildNumber)
			asrt.Equal(userID, bld.InitiatorID)
			asrt.Equal(database.WorkspaceTransitionStart, bld.Transition)
			asrt.Equal(database.BuildReasonInitiator, bld.Reason)
			asrt.Equal(buildID, bld.ID)
			asrt.True(bld.TemplateVersionPresetID.Valid)
			asrt.Equal(presetID, bld.TemplateVersionPresetID.UUID)
		}),
		withBuild,
		withNoTask,
		expectBuildParameters(func(params database.InsertWorkspaceBuildParametersParams) {
			asrt.Equal(buildID, params.WorkspaceBuildID)
			asrt.Empty(params.Name)
			asrt.Empty(params.Value)
		}),
	)
	fc := files.New(prometheus.NewRegistry(), &coderdtest.FakeAuthorizer{})

	ws := database.Workspace{ID: workspaceID, TemplateID: templateID, OwnerID: userID}
	uut := wsbuilder.New(ws, database.WorkspaceTransitionStart, wsbuilder.NoopUsageChecker{}).
		ActiveVersion().
		TemplateVersionPresetID(presetID)
	// nolint: dogsled
	_, _, _, err := uut.Build(ctx, mDB, fc, nil, audit.WorkspaceBuildBaggage{})
	req.NoError(err)
}

func TestWorkspaceBuildDeleteOrphan(t *testing.T) {
	t.Parallel()

	t.Run("WithActiveProvisioners", func(t *testing.T) {
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
			withTemplateVersionVariables(inactiveVersionID, nil),
			withRichParameters(nil),
			withWorkspaceTags(inactiveVersionID, nil),
			withProvisionerDaemons([]database.GetEligibleProvisionerDaemonsByProvisionerJobIDsRow{{
				JobID: inactiveJobID,
				ProvisionerDaemon: database.ProvisionerDaemon{
					LastSeenAt: sql.NullTime{Valid: true, Time: dbtime.Now()},
				},
			}}),

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
			expectFindMatchingPresetID(uuid.Nil, sql.ErrNoRows),
			expectBuild(func(bld database.InsertWorkspaceBuildParams) {
				asrt.Equal(inactiveVersionID, bld.TemplateVersionID)
				asrt.Equal(workspaceID, bld.WorkspaceID)
				asrt.Equal(int32(2), bld.BuildNumber)
				asrt.Empty(string(bld.ProvisionerState))
				asrt.Equal(userID, bld.InitiatorID)
				asrt.Equal(database.WorkspaceTransitionDelete, bld.Transition)
				asrt.Equal(database.BuildReasonInitiator, bld.Reason)
				asrt.Equal(buildID, bld.ID)
			}),
			withBuild,
			withNoTask,
			expectBuildParameters(func(params database.InsertWorkspaceBuildParametersParams) {
				asrt.Equal(buildID, params.WorkspaceBuildID)
				asrt.Empty(params.Name)
				asrt.Empty(params.Value)
			}),
		)

		ws := database.Workspace{ID: workspaceID, TemplateID: templateID, OwnerID: userID}
		uut := wsbuilder.New(ws, database.WorkspaceTransitionDelete, wsbuilder.NoopUsageChecker{}).Orphan()
		fc := files.New(prometheus.NewRegistry(), &coderdtest.FakeAuthorizer{})

		// nolint: dogsled
		_, _, _, err := uut.Build(ctx, mDB, fc, nil, audit.WorkspaceBuildBaggage{})
		req.NoError(err)
	})

	t.Run("NoActiveProvisioners", func(t *testing.T) {
		t.Parallel()
		req := require.New(t)
		asrt := assert.New(t)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		var buildID uuid.UUID
		var jobID uuid.UUID

		mDB := expectDB(t,
			// Inputs
			withTemplate,
			withInactiveVersion(nil),
			withLastBuildFound,
			withTemplateVersionVariables(inactiveVersionID, nil),
			withRichParameters(nil),
			withWorkspaceTags(inactiveVersionID, nil),
			withProvisionerDaemons([]database.GetEligibleProvisionerDaemonsByProvisionerJobIDsRow{}),

			// Outputs
			expectProvisionerJob(func(job database.InsertProvisionerJobParams) {
				asrt.Equal(userID, job.InitiatorID)
				asrt.Equal(inactiveFileID, job.FileID)
				input := provisionerdserver.WorkspaceProvisionJob{}
				err := json.Unmarshal(job.Input, &input)
				req.NoError(err)
				// store build ID for later
				buildID = input.WorkspaceBuildID
				// store job ID for later
				jobID = job.ID
			}),

			withInTx,
			expectFindMatchingPresetID(uuid.Nil, sql.ErrNoRows),
			expectBuild(func(bld database.InsertWorkspaceBuildParams) {
				asrt.Equal(inactiveVersionID, bld.TemplateVersionID)
				asrt.Equal(workspaceID, bld.WorkspaceID)
				asrt.Equal(int32(2), bld.BuildNumber)
				asrt.Empty(string(bld.ProvisionerState))
				asrt.Equal(userID, bld.InitiatorID)
				asrt.Equal(database.WorkspaceTransitionDelete, bld.Transition)
				asrt.Equal(database.BuildReasonInitiator, bld.Reason)
				asrt.Equal(buildID, bld.ID)
			}),
			withBuild,
			withNoTask,
			expectBuildParameters(func(params database.InsertWorkspaceBuildParametersParams) {
				asrt.Equal(buildID, params.WorkspaceBuildID)
				asrt.Empty(params.Name)
				asrt.Empty(params.Value)
			}),

			// Because no provisioners were available and the request was to delete --orphan
			expectUpdateProvisionerJobWithCompleteWithStartedAtByID(func(params database.UpdateProvisionerJobWithCompleteWithStartedAtByIDParams) {
				asrt.Equal(jobID, params.ID)
				asrt.False(params.Error.Valid)
				asrt.True(params.CompletedAt.Valid)
				asrt.True(params.StartedAt.Valid)
			}),
			expectUpdateWorkspaceDeletedByID(func(params database.UpdateWorkspaceDeletedByIDParams) {
				asrt.Equal(workspaceID, params.ID)
				asrt.True(params.Deleted)
			}),
			expectGetProvisionerJobByID(func(job database.ProvisionerJob) {
				asrt.Equal(jobID, job.ID)
			}),
		)

		ws := database.Workspace{ID: workspaceID, TemplateID: templateID, OwnerID: userID}
		uut := wsbuilder.New(ws, database.WorkspaceTransitionDelete, wsbuilder.NoopUsageChecker{}).Orphan()
		fc := files.New(prometheus.NewRegistry(), &coderdtest.FakeAuthorizer{})
		// nolint: dogsled
		_, _, _, err := uut.Build(ctx, mDB, fc, nil, audit.WorkspaceBuildBaggage{})
		req.NoError(err)
	})
}

func TestWorkspaceBuildUsageChecker(t *testing.T) {
	t.Parallel()

	t.Run("Permitted", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		var calls int64
		fakeUsageChecker := &fakeUsageChecker{
			checkBuildUsageFunc: func(_ context.Context, _ database.Store, _ *database.TemplateVersion, _ *database.Task, _ database.WorkspaceTransition) (wsbuilder.UsageCheckResponse, error) {
				atomic.AddInt64(&calls, 1)
				return wsbuilder.UsageCheckResponse{Permitted: true}, nil
			},
		}

		mDB := expectDB(t,
			// Inputs
			withTemplate,
			withInactiveVersion(nil),
			withLastBuildFound,
			withTemplateVersionVariables(inactiveVersionID, nil),
			withRichParameters(nil),
			withParameterSchemas(inactiveJobID, nil),
			withWorkspaceTags(inactiveVersionID, nil),
			withProvisionerDaemons([]database.GetEligibleProvisionerDaemonsByProvisionerJobIDsRow{}),

			// Outputs
			expectProvisionerJob(func(job database.InsertProvisionerJobParams) {}),
			withInTx,
			expectFindMatchingPresetID(uuid.Nil, sql.ErrNoRows),
			expectBuild(func(bld database.InsertWorkspaceBuildParams) {}),
			withBuild,
			withNoTask,
			expectBuildParameters(func(params database.InsertWorkspaceBuildParametersParams) {}),
		)
		fc := files.New(prometheus.NewRegistry(), &coderdtest.FakeAuthorizer{})

		ws := database.Workspace{ID: workspaceID, TemplateID: templateID, OwnerID: userID}
		uut := wsbuilder.New(ws, database.WorkspaceTransitionStart, fakeUsageChecker)
		// nolint: dogsled
		_, _, _, err := uut.Build(ctx, mDB, fc, nil, audit.WorkspaceBuildBaggage{})
		require.NoError(t, err)
		require.EqualValues(t, 1, calls)
	})

	// The failure cases are mostly identical from a test perspective.
	const message = "fake test message"
	cases := []struct {
		name        string
		response    wsbuilder.UsageCheckResponse
		responseErr error
		assertions  func(t *testing.T, err error)
	}{
		{
			name: "NotPermitted",
			response: wsbuilder.UsageCheckResponse{
				Permitted: false,
				Message:   message,
			},
			assertions: func(t *testing.T, err error) {
				require.ErrorContains(t, err, message)
				var buildErr wsbuilder.BuildError
				require.ErrorAs(t, err, &buildErr)
				require.Equal(t, http.StatusForbidden, buildErr.Status)
			},
		},
		{
			name:        "Error",
			responseErr: xerrors.New("fake error"),
			assertions: func(t *testing.T, err error) {
				require.ErrorContains(t, err, "fake error")
				require.ErrorAs(t, err, &wsbuilder.BuildError{})
			},
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			var calls int64
			fakeUsageChecker := &fakeUsageChecker{
				checkBuildUsageFunc: func(_ context.Context, _ database.Store, _ *database.TemplateVersion, _ *database.Task, _ database.WorkspaceTransition) (wsbuilder.UsageCheckResponse, error) {
					atomic.AddInt64(&calls, 1)
					return c.response, c.responseErr
				},
			}

			mDB := expectDB(t,
				withTemplate,
				withNoTask,
				withInactiveVersionNoParams(),
			)
			fc := files.New(prometheus.NewRegistry(), &coderdtest.FakeAuthorizer{})

			ws := database.Workspace{ID: workspaceID, TemplateID: templateID, OwnerID: userID}
			uut := wsbuilder.New(ws, database.WorkspaceTransitionStart, fakeUsageChecker).
				VersionID(inactiveVersionID)
			// nolint: dogsled
			_, _, _, err := uut.Build(ctx, mDB, fc, nil, audit.WorkspaceBuildBaggage{})
			c.assertions(t, err)
			require.EqualValues(t, 1, calls)
		})
	}
}

func TestWorkspaceBuildWithTask(t *testing.T) {
	t.Parallel()
	req := require.New(t)
	asrt := assert.New(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testTask := database.Task{
		ID:                taskID,
		OrganizationID:    orgID,
		OwnerID:           userID,
		Name:              "test-task",
		WorkspaceID:       uuid.NullUUID{UUID: workspaceID, Valid: true},
		TemplateVersionID: activeVersionID,
		CreatedAt:         dbtime.Now(),
	}

	mDB := expectDB(t,
		// Inputs
		withTemplate,
		withInactiveVersion(nil),
		withLastBuildFound,
		withTemplateVersionVariables(inactiveVersionID, nil),
		withRichParameters(nil),
		withParameterSchemas(inactiveJobID, nil),
		withWorkspaceTags(inactiveVersionID, nil),
		withProvisionerDaemons([]database.GetEligibleProvisionerDaemonsByProvisionerJobIDsRow{}),

		// Outputs
		expectProvisionerJob(func(job database.InsertProvisionerJobParams) {}),
		withInTx,
		expectFindMatchingPresetID(uuid.Nil, sql.ErrNoRows),
		expectBuild(func(bld database.InsertWorkspaceBuildParams) {}),
		withBuild,
		withTask(testTask),
		expectUpsertTaskWorkspaceApp(func(params database.UpsertTaskWorkspaceAppParams) {
			asrt.Equal(taskID, params.TaskID)
			asrt.Equal(int32(2), params.WorkspaceBuildNumber)
			asrt.False(params.WorkspaceAgentID.Valid, "workspace_agent_id should be NULL initially")
			asrt.False(params.WorkspaceAppID.Valid, "workspace_app_id should be NULL initially")
		}),
		expectBuildParameters(func(params database.InsertWorkspaceBuildParametersParams) {}),
	)
	fc := files.New(prometheus.NewRegistry(), &coderdtest.FakeAuthorizer{})

	ws := database.Workspace{ID: workspaceID, TemplateID: templateID, OwnerID: userID}
	uut := wsbuilder.New(ws, database.WorkspaceTransitionStart, wsbuilder.NoopUsageChecker{})
	// nolint: dogsled
	_, _, _, err := uut.Build(ctx, mDB, fc, nil, audit.WorkspaceBuildBaggage{})
	req.NoError(err)
}

func TestWsbuildError(t *testing.T) {
	t.Parallel()

	const msg = "test error"
	var buildErr error = wsbuilder.BuildError{
		Status:  http.StatusBadRequest,
		Message: msg,
	}

	respErr, ok := httperror.IsResponder(buildErr)
	require.True(t, ok, "should be a Coder SDK error")

	code, resp := respErr.Response()
	require.Equal(t, http.StatusBadRequest, code)
	require.Equal(t, msg, resp.Message)
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
		gomock.Any(), gomock.Eq(&database.TxOptions{Isolation: sql.LevelRepeatableRead}),
	).
		DoAndReturn(func(f func(database.Store) error, _ *database.TxOptions) error {
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
			ID:                      templateID,
			OrganizationID:          orgID,
			Provisioner:             database.ProvisionerTypeTerraform,
			ActiveVersionID:         activeVersionID,
			UseClassicParameterFlow: true,
		}, nil)
}

// withInTx runs the given functions on the same db mock.
func withInTx(mTx *dbmock.MockStore) {
	mTx.EXPECT().InTx(gomock.Any(), gomock.Any()).Times(1).DoAndReturn(
		func(f func(store database.Store) error, _ *database.TxOptions) error {
			return f(mTx)
		},
	)
}

func withActiveVersionNoParams() func(mTx *dbmock.MockStore) {
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
	}
}

func withActiveVersion(params []database.TemplateVersionParameter) func(mTx *dbmock.MockStore) {
	return func(mTx *dbmock.MockStore) {
		withActiveVersionNoParams()(mTx)
		paramsCall := mTx.EXPECT().GetTemplateVersionParameters(gomock.Any(), activeVersionID).
			Times(1)
		if len(params) > 0 {
			paramsCall.Return(params, nil)
		} else {
			paramsCall.Return(nil, sql.ErrNoRows)
		}
	}
}

func withInactiveVersionNoParams() func(mTx *dbmock.MockStore) {
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
	}
}

func withInactiveVersion(params []database.TemplateVersionParameter) func(mTx *dbmock.MockStore) {
	return func(mTx *dbmock.MockStore) {
		withInactiveVersionNoParams()(mTx)

		paramsCall := mTx.EXPECT().GetTemplateVersionParameters(gomock.Any(), inactiveVersionID).
			Times(1)
		if len(params) > 0 {
			paramsCall.Return(params, nil)
		} else {
			paramsCall.Return(nil, sql.ErrNoRows)
		}
	}
}

func withTemplateVersionPresetParameters(presetID uuid.UUID, params []database.TemplateVersionPresetParameter) func(mTx *dbmock.MockStore) {
	return func(mTx *dbmock.MockStore) {
		mTx.EXPECT().GetPresetParametersByPresetID(gomock.Any(), presetID).Return(params, nil)
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

func withTemplateVersionVariables(versionID uuid.UUID, params []database.TemplateVersionVariable) func(mTx *dbmock.MockStore) {
	return func(mTx *dbmock.MockStore) {
		c := mTx.EXPECT().GetTemplateVersionVariables(gomock.Any(), versionID).
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

func withWorkspaceTags(versionID uuid.UUID, tags []database.TemplateVersionWorkspaceTag) func(mTx *dbmock.MockStore) {
	return func(mTx *dbmock.MockStore) {
		c := mTx.EXPECT().GetTemplateVersionWorkspaceTags(gomock.Any(), versionID).
			Times(1)
		if len(tags) > 0 {
			c.Return(tags, nil)
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

// expectUpdateProvisionerJobWithCompleteWithStartedAtByID asserts a call to
// expectUpdateProvisionerJobWithCompleteWithStartedAtByID and runs the provided
// assertions against it.
func expectUpdateProvisionerJobWithCompleteWithStartedAtByID(assertions func(params database.UpdateProvisionerJobWithCompleteWithStartedAtByIDParams)) func(mTx *dbmock.MockStore) {
	return func(mTx *dbmock.MockStore) {
		mTx.EXPECT().UpdateProvisionerJobWithCompleteWithStartedAtByID(gomock.Any(), gomock.Any()).
			Times(1).
			DoAndReturn(
				func(ctx context.Context, params database.UpdateProvisionerJobWithCompleteWithStartedAtByIDParams) error {
					assertions(params)
					return nil
				},
			)
	}
}

// expectUpdateWorkspaceDeletedByID asserts a call to UpdateWorkspaceDeletedByID
// and runs the provided assertions against it.
func expectUpdateWorkspaceDeletedByID(assertions func(params database.UpdateWorkspaceDeletedByIDParams)) func(mTx *dbmock.MockStore) {
	return func(mTx *dbmock.MockStore) {
		mTx.EXPECT().UpdateWorkspaceDeletedByID(gomock.Any(), gomock.Any()).
			Times(1).
			DoAndReturn(
				func(ctx context.Context, params database.UpdateWorkspaceDeletedByIDParams) error {
					assertions(params)
					return nil
				},
			)
	}
}

// expectGetProvisionerJobByID asserts a call to GetProvisionerJobByID
// and runs the provided assertions against it.
func expectGetProvisionerJobByID(assertions func(job database.ProvisionerJob)) func(mTx *dbmock.MockStore) {
	return func(mTx *dbmock.MockStore) {
		mTx.EXPECT().GetProvisionerJobByID(gomock.Any(), gomock.Any()).
			Times(1).
			DoAndReturn(
				func(ctx context.Context, id uuid.UUID) (database.ProvisionerJob, error) {
					job := database.ProvisionerJob{ID: id}
					assertions(job)
					return job, nil
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

func withProvisionerDaemons(provisionerDaemons []database.GetEligibleProvisionerDaemonsByProvisionerJobIDsRow) func(mTx *dbmock.MockStore) {
	return func(mTx *dbmock.MockStore) {
		mTx.EXPECT().GetEligibleProvisionerDaemonsByProvisionerJobIDs(gomock.Any(), gomock.Any()).Return(provisionerDaemons, nil)
	}
}

func expectFindMatchingPresetID(id uuid.UUID, err error) func(mTx *dbmock.MockStore) {
	return func(mTx *dbmock.MockStore) {
		mTx.EXPECT().FindMatchingPresetID(gomock.Any(), gomock.Any()).
			Times(1).
			Return(id, err)
	}
}

type fakeUsageChecker struct {
	checkBuildUsageFunc func(ctx context.Context, store database.Store, templateVersion *database.TemplateVersion, task *database.Task, transition database.WorkspaceTransition) (wsbuilder.UsageCheckResponse, error)
}

func (f *fakeUsageChecker) CheckBuildUsage(ctx context.Context, store database.Store, templateVersion *database.TemplateVersion, task *database.Task, transition database.WorkspaceTransition) (wsbuilder.UsageCheckResponse, error) {
	return f.checkBuildUsageFunc(ctx, store, templateVersion, task, transition)
}

func withNoTask(mTx *dbmock.MockStore) {
	mTx.EXPECT().GetTaskByWorkspaceID(gomock.Any(), gomock.Any()).Times(1).
		DoAndReturn(func(ctx context.Context, id uuid.UUID) (database.Task, error) {
			return database.Task{}, sql.ErrNoRows
		})
}

func withTask(task database.Task) func(mTx *dbmock.MockStore) {
	return func(mTx *dbmock.MockStore) {
		mTx.EXPECT().GetTaskByWorkspaceID(gomock.Any(), gomock.Any()).Times(1).
			DoAndReturn(func(ctx context.Context, id uuid.UUID) (database.Task, error) {
				return task, nil
			})
	}
}

func expectUpsertTaskWorkspaceApp(
	assertions func(database.UpsertTaskWorkspaceAppParams),
) func(mTx *dbmock.MockStore) {
	return func(mTx *dbmock.MockStore) {
		mTx.EXPECT().UpsertTaskWorkspaceApp(gomock.Any(), gomock.Any()).
			Times(1).
			DoAndReturn(
				func(ctx context.Context, params database.UpsertTaskWorkspaceAppParams) (database.TaskWorkspaceApp, error) {
					assertions(params)
					return database.TaskWorkspaceApp{
						TaskID:               params.TaskID,
						WorkspaceBuildNumber: params.WorkspaceBuildNumber,
						WorkspaceAgentID:     params.WorkspaceAgentID,
						WorkspaceAppID:       params.WorkspaceAppID,
					}, nil
				},
			)
	}
}
