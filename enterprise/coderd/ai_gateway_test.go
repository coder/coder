package coderd_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/testutil"
)

func TestAIGatewayPoliciesCRUD(t *testing.T) {
	t.Parallel()

	t.Run("CreateValidatesRego", func(t *testing.T) {
		t.Parallel()
		client, _ := coderdenttest.New(t, aibridgeOpts(t))
		ctx := testutil.Context(t, testutil.WaitLong)

		// A decide policy without a default verdict must be rejected by the gate.
		_, err := client.CreateAIGatewayPolicy(ctx, codersdk.CreateAIGatewayPolicyRequest{
			Name: "bad",
			Kind: codersdk.AIGatewayPolicyKindDecide,
			Rego: `verdict := "BLOCK" if input.request.model == "x"`,
		})
		require.Error(t, err)
	})

	t.Run("CreateAndVersion", func(t *testing.T) {
		t.Parallel()
		client, _ := coderdenttest.New(t, aibridgeOpts(t))
		ctx := testutil.Context(t, testutil.WaitLong)

		pol, err := client.CreateAIGatewayPolicy(ctx, codersdk.CreateAIGatewayPolicyRequest{
			Name: "model-allowlist",
			Kind: codersdk.AIGatewayPolicyKindDecide,
			Rego: `default verdict := "ALLOW"`,
		})
		require.NoError(t, err)
		require.NotNil(t, pol.ActiveVersionID)
		require.Len(t, pol.Versions, 1)

		v2, err := client.CreateAIGatewayPolicyVersion(ctx, pol.ID, codersdk.CreateAIGatewayPolicyVersionRequest{
			Rego:     `default verdict := "LOG"`,
			Activate: true,
		})
		require.NoError(t, err)
		require.EqualValues(t, 2, v2.VersionNumber)

		got, err := client.AIGatewayPolicy(ctx, pol.ID)
		require.NoError(t, err)
		require.NotNil(t, got.ActiveVersionID)
		require.Equal(t, v2.ID, *got.ActiveVersionID)
	})
}

func TestAIGatewayPipelinesCRUD(t *testing.T) {
	t.Parallel()

	client, _ := coderdenttest.New(t, aibridgeOpts(t))
	ctx := testutil.Context(t, testutil.WaitLong)

	prov, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
		Type:    codersdk.AIProviderTypeOpenAI,
		Name:    "openai",
		Enabled: true,
		BaseURL: "https://api.openai.com/v1/",
		APIKeys: []string{"sk-test"},
	})
	require.NoError(t, err)

	pol, err := client.CreateAIGatewayPolicy(ctx, codersdk.CreateAIGatewayPolicyRequest{
		Name: "model-allowlist",
		Kind: codersdk.AIGatewayPolicyKindDecide,
		Rego: `default verdict := "ALLOW"`,
	})
	require.NoError(t, err)
	require.NotNil(t, pol.ActiveVersionID)

	pipe, err := client.CreateAIGatewayPipeline(ctx, codersdk.CreateAIGatewayPipelineRequest{
		ProviderID: prov.ID,
		Enabled:    true,
		Policies: []codersdk.AIGatewayPipelinePolicyRequest{{
			PolicyVersionID: *pol.ActiveVersionID,
			Hook:            codersdk.AIGatewayHookPreReq,
			FailMode:        codersdk.AIGatewayFailModeClosed,
		}},
	})
	require.NoError(t, err)
	require.NotNil(t, pipe.ActiveVersion)
	require.Len(t, pipe.ActiveVersion.Policies, 1)
	require.Equal(t, codersdk.AIGatewayPolicyKindDecide, pipe.ActiveVersion.Policies[0].Kind)

	// Deleting a policy that is live in a pipeline must be blocked.
	err = client.DeleteAIGatewayPolicy(ctx, pol.ID)
	require.Error(t, err)

	// Second pipeline for the same provider must conflict.
	_, err = client.CreateAIGatewayPipeline(ctx, codersdk.CreateAIGatewayPipelineRequest{
		ProviderID: prov.ID,
		Enabled:    true,
		Policies: []codersdk.AIGatewayPipelinePolicyRequest{{
			PolicyVersionID: *pol.ActiveVersionID,
			Hook:            codersdk.AIGatewayHookPreReq,
			FailMode:        codersdk.AIGatewayFailModeClosed,
		}},
	})
	require.Error(t, err)
}

func TestAIGatewayPolicyVersionPropagation(t *testing.T) {
	t.Parallel()

	client, _ := coderdenttest.New(t, aibridgeOpts(t))
	ctx := testutil.Context(t, testutil.WaitLong)

	prov, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
		Type:    codersdk.AIProviderTypeOpenAI,
		Name:    "openai",
		Enabled: true,
		BaseURL: "https://api.openai.com/v1/",
		APIKeys: []string{"sk-test"},
	})
	require.NoError(t, err)

	pol, err := client.CreateAIGatewayPolicy(ctx, codersdk.CreateAIGatewayPolicyRequest{
		Name: "model-allowlist",
		Kind: codersdk.AIGatewayPolicyKindDecide,
		Rego: `default verdict := "BLOCK"`,
	})
	require.NoError(t, err)
	v1 := *pol.ActiveVersionID

	pipe, err := client.CreateAIGatewayPipeline(ctx, codersdk.CreateAIGatewayPipelineRequest{
		ProviderID: prov.ID,
		Enabled:    true,
		Policies: []codersdk.AIGatewayPipelinePolicyRequest{{
			PolicyVersionID: v1,
			Hook:            codersdk.AIGatewayHookPreReq,
			FailMode:        codersdk.AIGatewayFailModeClosed,
		}},
	})
	require.NoError(t, err)
	require.NotNil(t, pipe.ActiveVersion)
	require.Equal(t, v1, pipe.ActiveVersion.Policies[0].PolicyVersionID)

	// Activate a new policy version; the pipeline must be re-pinned to it.
	v2, err := client.CreateAIGatewayPolicyVersion(ctx, pol.ID, codersdk.CreateAIGatewayPolicyVersionRequest{
		Rego:     `default verdict := "ALLOW"`,
		Activate: true,
	})
	require.NoError(t, err)

	got, err := client.AIGatewayPipeline(ctx, pipe.ID)
	require.NoError(t, err)
	require.NotNil(t, got.ActiveVersion)
	require.NotEqual(t, pipe.ActiveVersionID, got.ActiveVersionID, "pipeline should have a new active version")
	require.Len(t, got.ActiveVersion.Policies, 1)
	require.Equal(t, v2.ID, got.ActiveVersion.Policies[0].PolicyVersionID, "pipeline should now pin v2")
}
