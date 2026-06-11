package aibridged_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/coderd/aibridged"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/testutil"
)

// mintPipelineVersion inserts an additional, unpromoted pipeline version for an
// existing provider's pipeline, with a single pre-req decide member, and returns
// the new pipeline version's id and number. The pipeline's active version is
// left unchanged (the version is a staged draft).
func mintPipelineVersion(ctx context.Context, t *testing.T, db database.Store, providerName, policyName, rego string, versionNumber int32) (uuid.UUID, int32) {
	t.Helper()

	prov, err := db.GetAIProviderByName(ctx, providerName)
	require.NoError(t, err)
	pipe, err := db.GetAIGatewayPipelineByProviderID(ctx, prov.ID)
	require.NoError(t, err)

	pol, err := db.InsertAIGatewayPolicy(ctx, database.InsertAIGatewayPolicyParams{
		ID:   uuid.New(),
		Name: policyName,
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

	pipeVer, err := db.InsertAIGatewayPipelineVersion(ctx, database.InsertAIGatewayPipelineVersionParams{
		ID:            uuid.New(),
		PipelineID:    pipe.ID,
		VersionNumber: versionNumber,
	})
	require.NoError(t, err)
	_, err = db.InsertAIGatewayPipelineVersionPolicy(ctx, database.InsertAIGatewayPipelineVersionPolicyParams{
		ID:                uuid.New(),
		PipelineVersionID: pipeVer.ID,
		PolicyVersionID:   polVer.ID,
		Hook:              database.AIGatewayHookPreReq,
		Kind:              database.AIGatewayPolicyKindDecide,
		FailMode:          database.AIGatewayFailModeFailClosed,
		Enabled:           true,
	})
	require.NoError(t, err)

	return pipeVer.ID, versionNumber
}

func TestPipelineVersionResolver(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, nil)

	// Active version (v1) on openai.
	name := seedPipeline(ctx, t, db, "openai", `default verdict := "ALLOW"`, true)
	// A staged, unpromoted version (v2) on the same pipeline.
	_, stagedNum := mintPipelineVersion(ctx, t, db, name, "openai-staged-block", `default verdict := "BLOCK"`, 2)
	stagedVersion := strconv.Itoa(int(stagedNum))

	resolver := aibridged.NewPipelineVersionResolver(db, logger)

	t.Run("resolves staged version by number", func(t *testing.T) {
		t.Parallel()
		pp, err := resolver.ResolvePipelineVersion(ctx, name, stagedVersion)
		require.NoError(t, err)
		require.NotNil(t, pp.PreReq, "staged version's pre-req pipeline should be built")
		require.EqualValues(t, stagedNum, pp.Version, "resolved snapshot must report the staged version")
	})

	t.Run("resolves staged version by vN label", func(t *testing.T) {
		t.Parallel()
		pp, err := resolver.ResolvePipelineVersion(ctx, name, "v"+stagedVersion)
		require.NoError(t, err)
		require.NotNil(t, pp.PreReq, "the 'vN' label must resolve like the bare number")
		require.EqualValues(t, stagedNum, pp.Version)
	})

	t.Run("caches by version number", func(t *testing.T) {
		t.Parallel()
		first, err := resolver.ResolvePipelineVersion(ctx, name, stagedVersion)
		require.NoError(t, err)
		second, err := resolver.ResolvePipelineVersion(ctx, name, stagedVersion)
		require.NoError(t, err)
		require.Equal(t, first.Version, second.Version)
		// The same compiled pipeline pointer is reused from the cache.
		require.Same(t, first.PreReq, second.PreReq)
	})

	t.Run("unknown version is not found", func(t *testing.T) {
		t.Parallel()
		_, err := resolver.ResolvePipelineVersion(ctx, name, "999")
		require.ErrorIs(t, err, aibridge.ErrPipelineVersionNotFound)
	})

	t.Run("non-numeric version is not found", func(t *testing.T) {
		t.Parallel()
		_, err := resolver.ResolvePipelineVersion(ctx, name, "not-a-number")
		require.ErrorIs(t, err, aibridge.ErrPipelineVersionNotFound)
	})

	t.Run("foreign provider version is not found", func(t *testing.T) {
		t.Parallel()
		// Version number 2 exists for openai's pipeline, but the anthropic
		// pipeline only has v1, so the number must not resolve under it.
		other := seedPipeline(ctx, t, db, "anthropic", `default verdict := "ALLOW"`, true)
		_, err := resolver.ResolvePipelineVersion(ctx, other, stagedVersion)
		require.ErrorIs(t, err, aibridge.ErrPipelineVersionNotFound)
	})
}
