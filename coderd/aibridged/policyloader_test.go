package aibridged_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/aibridged"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/testutil"
)

// seedPipeline inserts a provider with a single-decide pre-req pipeline and
// returns the provider name.
func seedPipeline(ctx context.Context, t *testing.T, db database.Store, providerName, rego string, memberEnabled bool) string {
	t.Helper()

	prov, err := db.InsertAIProvider(ctx, database.InsertAIProviderParams{
		ID:      uuid.New(),
		Type:    database.AiProviderTypeOpenai,
		Name:    providerName,
		Enabled: true,
		BaseUrl: "https://api.example.com/",
	})
	require.NoError(t, err)

	pol, err := db.InsertAIGatewayPolicy(ctx, database.InsertAIGatewayPolicyParams{
		ID:   uuid.New(),
		Name: providerName + "-decide",
		Kind: database.AIGatewayPolicyKindDecide,
	})
	require.NoError(t, err)
	polVer, err := db.InsertAIGatewayPolicyVersion(ctx, database.InsertAIGatewayPolicyVersionParams{
		ID:                  uuid.New(),
		PolicyID:            pol.ID,
		VersionNumber:       1,
		Rego:                rego,
		InputSchemaVersion:  1,
		OutputSchemaVersion: 1,
	})
	require.NoError(t, err)
	require.NoError(t, db.UpdateAIGatewayPolicyActiveVersion(ctx, database.UpdateAIGatewayPolicyActiveVersionParams{
		ID:              pol.ID,
		ActiveVersionID: polVer.ID,
	}))

	pipe, err := db.InsertAIGatewayPipeline(ctx, database.InsertAIGatewayPipelineParams{
		ID:         uuid.New(),
		ProviderID: prov.ID,
		Enabled:    true,
	})
	require.NoError(t, err)
	pipeVer, err := db.InsertAIGatewayPipelineVersion(ctx, database.InsertAIGatewayPipelineVersionParams{
		ID:            uuid.New(),
		PipelineID:    pipe.ID,
		VersionNumber: 1,
	})
	require.NoError(t, err)
	_, err = db.InsertAIGatewayPipelineVersionPolicy(ctx, database.InsertAIGatewayPipelineVersionPolicyParams{
		ID:                uuid.New(),
		PipelineVersionID: pipeVer.ID,
		PolicyVersionID:   polVer.ID,
		Hook:              database.AIGatewayHookPreReq,
		Kind:              database.AIGatewayPolicyKindDecide,
		FailMode:          database.AIGatewayFailModeFailClosed,
		Enabled:           memberEnabled,
	})
	require.NoError(t, err)
	require.NoError(t, db.UpdateAIGatewayPipelineActiveVersion(ctx, database.UpdateAIGatewayPipelineActiveVersionParams{
		ID:              pipe.ID,
		ActiveVersionID: pipeVer.ID,
	}))

	return prov.Name
}

func TestBuildProviderPipelines(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, nil)

	name := seedPipeline(ctx, t, db, "openai", `default verdict := "ALLOW"`, true)

	hooks, outcomes, err := aibridged.BuildProviderPipelines(ctx, db, logger)
	require.NoError(t, err)

	pp, ok := hooks[name]
	require.True(t, ok, "expected pipeline for provider %q", name)
	require.NotNil(t, pp.PreReq, "pre-req pipeline should be built")
	require.Nil(t, pp.PreAuth, "no pre-auth policy was configured")

	require.Len(t, outcomes, 1)
	require.Equal(t, name, outcomes[0].Provider)
	require.EqualValues(t, 1, outcomes[0].PipelineVersion)
}

func TestBuildProviderPipelines_DisabledPipelineExcluded(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, nil)

	name := seedPipeline(ctx, t, db, "openai", `default verdict := "ALLOW"`, true)

	// Disable the pipeline; it should drop out of the snapshot.
	pipe, err := db.GetAIGatewayPipelineByProviderID(ctx, mustProviderID(ctx, t, db, name))
	require.NoError(t, err)
	_, err = db.UpdateAIGatewayPipeline(ctx, database.UpdateAIGatewayPipelineParams{ID: pipe.ID, Enabled: false})
	require.NoError(t, err)

	hooks, _, err := aibridged.BuildProviderPipelines(ctx, db, logger)
	require.NoError(t, err)
	_, ok := hooks[name]
	require.False(t, ok, "disabled pipeline must be excluded from the snapshot")
}

func TestBuildProviderPipelines_DisabledMemberExcluded(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, nil)

	// The pipeline is enabled but its only member is disabled within it.
	name := seedPipeline(ctx, t, db, "openai", `default verdict := "ALLOW"`, false)

	hooks, _, err := aibridged.BuildProviderPipelines(ctx, db, logger)
	require.NoError(t, err)
	_, ok := hooks[name]
	require.False(t, ok, "a pipeline whose only member is disabled must not appear")
}

func TestBuildProviderPipelines_PreToolHook(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, nil)

	prov, err := db.InsertAIProvider(ctx, database.InsertAIProviderParams{
		ID:      uuid.New(),
		Type:    database.AiProviderTypeOpenai,
		Name:    "openai",
		Enabled: true,
		BaseUrl: "https://api.example.com/",
	})
	require.NoError(t, err)

	pol, err := db.InsertAIGatewayPolicy(ctx, database.InsertAIGatewayPolicyParams{
		ID:   uuid.New(),
		Name: "tool-gate",
		Kind: database.AIGatewayPolicyKindDecide,
	})
	require.NoError(t, err)
	polVer, err := db.InsertAIGatewayPolicyVersion(ctx, database.InsertAIGatewayPolicyVersionParams{
		ID:                  uuid.New(),
		PolicyID:            pol.ID,
		VersionNumber:       1,
		Rego:                `default verdict := "ALLOW"`,
		InputSchemaVersion:  1,
		OutputSchemaVersion: 1,
	})
	require.NoError(t, err)
	require.NoError(t, db.UpdateAIGatewayPolicyActiveVersion(ctx, database.UpdateAIGatewayPolicyActiveVersionParams{
		ID:              pol.ID,
		ActiveVersionID: polVer.ID,
	}))

	pipe, err := db.InsertAIGatewayPipeline(ctx, database.InsertAIGatewayPipelineParams{
		ID:         uuid.New(),
		ProviderID: prov.ID,
		Enabled:    true,
	})
	require.NoError(t, err)
	pipeVer, err := db.InsertAIGatewayPipelineVersion(ctx, database.InsertAIGatewayPipelineVersionParams{
		ID:            uuid.New(),
		PipelineID:    pipe.ID,
		VersionNumber: 1,
	})
	require.NoError(t, err)
	_, err = db.InsertAIGatewayPipelineVersionPolicy(ctx, database.InsertAIGatewayPipelineVersionPolicyParams{
		ID:                uuid.New(),
		PipelineVersionID: pipeVer.ID,
		PolicyVersionID:   polVer.ID,
		Hook:              database.AIGatewayHookPreTool,
		Kind:              database.AIGatewayPolicyKindDecide,
		FailMode:          database.AIGatewayFailModeFailClosed,
		Enabled:           true,
	})
	require.NoError(t, err)
	require.NoError(t, db.UpdateAIGatewayPipelineActiveVersion(ctx, database.UpdateAIGatewayPipelineActiveVersionParams{
		ID:              pipe.ID,
		ActiveVersionID: pipeVer.ID,
	}))

	hooks, _, err := aibridged.BuildProviderPipelines(ctx, db, logger)
	require.NoError(t, err)
	pp, ok := hooks["openai"]
	require.True(t, ok)
	require.NotNil(t, pp.PreTool, "pre-tool pipeline should be built")
	require.Nil(t, pp.PreReq)
	// A single fail-closed member yields an aggregate fail-closed gate.
	require.False(t, pp.PreToolFailOpen)
}

func mustProviderID(ctx context.Context, t *testing.T, db database.Store, name string) uuid.UUID {
	t.Helper()
	prov, err := db.GetAIProviderByName(ctx, name)
	require.NoError(t, err)
	return prov.ID
}
