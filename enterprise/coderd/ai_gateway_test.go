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
			Rego: `verdict := "BLOCK" if input.request.body.model == "x"`,
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

func TestAIGatewayPipelineMemberNameCollision(t *testing.T) {
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

	// A policy and a guardrail share the name "pii".
	pol, err := client.CreateAIGatewayPolicy(ctx, codersdk.CreateAIGatewayPolicyRequest{
		Name: "pii",
		Kind: codersdk.AIGatewayPolicyKindDecide,
		Rego: `default verdict := "ALLOW"`,
	})
	require.NoError(t, err)

	gr, err := client.CreateAIGatewayGuardrail(ctx, codersdk.CreateAIGatewayGuardrailRequest{
		Name:        "pii",
		AdapterType: "presidio",
		Config:      []byte(`{"analyzer_url":"http://localhost:1","entity_actions":{"EMAIL_ADDRESS":"BLOCK"}}`),
	})
	require.NoError(t, err)

	// Attaching both to one pipeline must be rejected: names key the
	// annotation namespace and must be unique within a pipeline.
	_, err = client.CreateAIGatewayPipeline(ctx, codersdk.CreateAIGatewayPipelineRequest{
		ProviderID: prov.ID,
		Enabled:    true,
		Policies: []codersdk.AIGatewayPipelinePolicyRequest{{
			PolicyVersionID: *pol.ActiveVersionID,
			Hook:            codersdk.AIGatewayHookPreReq,
			FailMode:        codersdk.AIGatewayFailModeClosed,
		}},
		Guardrails: []codersdk.AIGatewayPipelineGuardrailRequest{{
			GuardrailVersionID: *gr.ActiveVersionID,
			Hook:               codersdk.AIGatewayHookPreReq,
			FailMode:           codersdk.AIGatewayFailModeClosed,
		}},
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "must be unique")
}

// TestAIGatewayPipelineEditPreservesStagedGuardrail covers the regression where
// editing a pipeline dropped a guardrail that was added in an earlier
// unpromoted draft. The fix exposes the tip version's full membership as
// LatestVersion so an edit can be based on the tip (the staged lineage) rather
// than the stale active version.
func TestAIGatewayPipelineEditPreservesStagedGuardrail(t *testing.T) {
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
		Name: "allow",
		Kind: codersdk.AIGatewayPolicyKindDecide,
		Rego: `default verdict := "ALLOW"`,
	})
	require.NoError(t, err)
	polV1 := *pol.ActiveVersionID

	gr, err := client.CreateAIGatewayGuardrail(ctx, codersdk.CreateAIGatewayGuardrailRequest{
		Name:        "pii",
		AdapterType: "presidio",
		Config:      []byte(`{"analyzer_url":"http://localhost:1","entity_actions":{"EMAIL_ADDRESS":"BLOCK"}}`),
	})
	require.NoError(t, err)
	grVersion := *gr.ActiveVersionID

	// Pipeline v1: a policy, no guardrail. v1 is auto-activated (live).
	pipe, err := client.CreateAIGatewayPipeline(ctx, codersdk.CreateAIGatewayPipelineRequest{
		ProviderID: prov.ID,
		Enabled:    true,
		Policies: []codersdk.AIGatewayPipelinePolicyRequest{{
			PolicyVersionID: polV1,
			Hook:            codersdk.AIGatewayHookPreReq,
			FailMode:        codersdk.AIGatewayFailModeClosed,
		}},
	})
	require.NoError(t, err)

	policyReq := []codersdk.AIGatewayPipelinePolicyRequest{{
		PolicyVersionID: polV1,
		Hook:            codersdk.AIGatewayHookPreReq,
		FailMode:        codersdk.AIGatewayFailModeClosed,
	}}
	guardrailReq := []codersdk.AIGatewayPipelineGuardrailRequest{{
		GuardrailVersionID: grVersion,
		Hook:               codersdk.AIGatewayHookPreReq,
		FailMode:           codersdk.AIGatewayFailModeClosed,
	}}

	// Stage a draft v2 that adds the guardrail, without promoting.
	_, err = client.CreateAIGatewayPipelineVersion(ctx, pipe.ID, codersdk.CreateAIGatewayPipelineVersionRequest{
		Policies:   policyReq,
		Guardrails: guardrailReq,
		Activate:   false,
	})
	require.NoError(t, err)

	// The active version is still v1 (no guardrail), but the tip (LatestVersion)
	// carries the staged guardrail with its full membership. The edit UI bases
	// the next version on LatestVersion, so the guardrail is not dropped.
	got, err := client.AIGatewayPipeline(ctx, pipe.ID)
	require.NoError(t, err)
	require.True(t, got.HasUnpromotedChanges())
	require.NotNil(t, got.ActiveVersion)
	require.Empty(t, got.ActiveVersion.Guardrails, "live version has no guardrail yet")
	require.NotNil(t, got.LatestVersion, "tip membership must be exposed for tip-based edits")
	require.Len(t, got.LatestVersion.Guardrails, 1, "tip carries the staged guardrail")
	require.Equal(t, grVersion, got.LatestVersion.Guardrails[0].GuardrailVersionID)

	// A subsequent edit (e.g. a policy toggle) based on the tip must carry the
	// guardrail forward. Re-mint from the tip's membership with a flipped policy.
	tipGuardrails := make([]codersdk.AIGatewayPipelineGuardrailRequest, 0, len(got.LatestVersion.Guardrails))
	for _, g := range got.LatestVersion.Guardrails {
		failMode := g.FailMode
		timeout := g.NetworkTimeoutMS
		enabled := g.Enabled
		tipGuardrails = append(tipGuardrails, codersdk.AIGatewayPipelineGuardrailRequest{
			GuardrailVersionID: g.GuardrailVersionID,
			Hook:               g.Hook,
			FailMode:           failMode,
			NetworkTimeoutMS:   &timeout,
			Enabled:            &enabled,
		})
	}
	_, err = client.CreateAIGatewayPipelineVersion(ctx, pipe.ID, codersdk.CreateAIGatewayPipelineVersionRequest{
		Policies:   policyReq,
		Guardrails: tipGuardrails,
		Activate:   false,
	})
	require.NoError(t, err)

	got2, err := client.AIGatewayPipeline(ctx, pipe.ID)
	require.NoError(t, err)
	require.NotNil(t, got2.LatestVersion)
	require.Len(t, got2.LatestVersion.Guardrails, 1, "editing the tip must preserve the staged guardrail")
	require.Equal(t, grVersion, got2.LatestVersion.Guardrails[0].GuardrailVersionID)
}

// TestAIGatewayPipelineMemberToggleInPlace verifies that enabling/disabling a
// member (policy or guardrail) flips the membership flag on the live version in
// place and does NOT mint a new pipeline version.
func TestAIGatewayPipelineMemberToggleInPlace(t *testing.T) {
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
		Name: "allow",
		Kind: codersdk.AIGatewayPolicyKindDecide,
		Rego: `default verdict := "ALLOW"`,
	})
	require.NoError(t, err)
	gr, err := client.CreateAIGatewayGuardrail(ctx, codersdk.CreateAIGatewayGuardrailRequest{
		Name:        "pii",
		AdapterType: "presidio",
		Config:      []byte(`{"analyzer_url":"http://localhost:1","entity_actions":{"EMAIL_ADDRESS":"BLOCK"}}`),
	})
	require.NoError(t, err)

	pipe, err := client.CreateAIGatewayPipeline(ctx, codersdk.CreateAIGatewayPipelineRequest{
		ProviderID: prov.ID,
		Enabled:    true,
		Policies: []codersdk.AIGatewayPipelinePolicyRequest{{
			PolicyVersionID: *pol.ActiveVersionID,
			Hook:            codersdk.AIGatewayHookPreReq,
			FailMode:        codersdk.AIGatewayFailModeClosed,
		}},
		Guardrails: []codersdk.AIGatewayPipelineGuardrailRequest{{
			GuardrailVersionID: *gr.ActiveVersionID,
			Hook:               codersdk.AIGatewayHookPreReq,
			FailMode:           codersdk.AIGatewayFailModeClosed,
		}},
	})
	require.NoError(t, err)

	// Disable the guardrail member in place.
	got, err := client.UpdateAIGatewayPipelineMember(ctx, pipe.ID, codersdk.UpdateAIGatewayPipelineMemberRequest{
		GuardrailVersionID: gr.ActiveVersionID,
		Hook:               codersdk.AIGatewayHookPreReq,
		Enabled:            false,
	})
	require.NoError(t, err)
	require.Equal(t, *pipe.ActiveVersionID, *got.ActiveVersionID, "no new version is activated")
	require.False(t, got.HasUnpromotedChanges(), "enable/disable must not mint a draft")
	require.NotNil(t, got.ActiveVersion)
	require.Len(t, got.ActiveVersion.Guardrails, 1)
	require.False(t, got.ActiveVersion.Guardrails[0].Enabled, "guardrail member disabled live, in place")

	// Disable the policy member too; still no version churn.
	got, err = client.UpdateAIGatewayPipelineMember(ctx, pipe.ID, codersdk.UpdateAIGatewayPipelineMemberRequest{
		PolicyVersionID: pol.ActiveVersionID,
		Hook:            codersdk.AIGatewayHookPreReq,
		Enabled:         false,
	})
	require.NoError(t, err)
	require.False(t, got.ActiveVersion.Policies[0].Enabled, "policy member disabled live, in place")

	versions, err := client.AIGatewayPipelineVersions(ctx, pipe.ID)
	require.NoError(t, err)
	require.Len(t, versions, 1, "enable/disable must never mint a pipeline version")
}

// TestAIGatewayGuardrailEditMintsPipelineVersion verifies that editing a
// guardrail mints a pipeline version on a pipeline that uses it, even when the
// guardrail is only in the pipeline's unpromoted tip (matching how policy edits
// behave). This exercises tip-based propagation referencing.
func TestAIGatewayGuardrailEditMintsPipelineVersion(t *testing.T) {
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
		Name: "allow",
		Kind: codersdk.AIGatewayPolicyKindDecide,
		Rego: `default verdict := "ALLOW"`,
	})
	require.NoError(t, err)
	gr, err := client.CreateAIGatewayGuardrail(ctx, codersdk.CreateAIGatewayGuardrailRequest{
		Name:        "pii",
		AdapterType: "presidio",
		Config:      []byte(`{"analyzer_url":"http://localhost:1","entity_actions":{"EMAIL_ADDRESS":"BLOCK"}}`),
	})
	require.NoError(t, err)
	grV1 := *gr.ActiveVersionID

	// Pipeline v1: policy only, no guardrail. v1 is live.
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

	// Stage v2 adding the guardrail, unpromoted (so the active version v1 does
	// NOT reference the guardrail; only the tip does).
	_, err = client.CreateAIGatewayPipelineVersion(ctx, pipe.ID, codersdk.CreateAIGatewayPipelineVersionRequest{
		Policies: []codersdk.AIGatewayPipelinePolicyRequest{{
			PolicyVersionID: *pol.ActiveVersionID,
			Hook:            codersdk.AIGatewayHookPreReq,
			FailMode:        codersdk.AIGatewayFailModeClosed,
		}},
		Guardrails: []codersdk.AIGatewayPipelineGuardrailRequest{{
			GuardrailVersionID: grV1,
			Hook:               codersdk.AIGatewayHookPreReq,
			FailMode:           codersdk.AIGatewayFailModeClosed,
		}},
		Activate: false,
	})
	require.NoError(t, err)

	before, err := client.AIGatewayPipeline(ctx, pipe.ID)
	require.NoError(t, err)
	require.EqualValues(t, 2, before.LatestVersionNumber)

	// Edit the guardrail config: new guardrail version, activated (not promoted).
	grV2, err := client.CreateAIGatewayGuardrailVersion(ctx, gr.ID, codersdk.CreateAIGatewayGuardrailVersionRequest{
		Config:   []byte(`{"analyzer_url":"http://localhost:1","entity_actions":{"US_SSN":"BLOCK"}}`),
		Activate: true,
	})
	require.NoError(t, err)

	// The guardrail edit must mint a new pipeline version (v3) on the pipeline
	// whose tip uses the guardrail, re-pinned to the new guardrail version, even
	// though the active version (v1) does not reference the guardrail.
	after, err := client.AIGatewayPipeline(ctx, pipe.ID)
	require.NoError(t, err)
	require.EqualValues(t, 3, after.LatestVersionNumber, "guardrail edit must mint a pipeline version")
	require.NotNil(t, after.LatestVersion)
	require.Len(t, after.LatestVersion.Guardrails, 1)
	require.Equal(t, grV2.ID, after.LatestVersion.Guardrails[0].GuardrailVersionID, "tip re-pinned to the new guardrail version")
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

	// Activate a new policy version WITHOUT promoting. Under explicit two-stage
	// rollout, activation propagates by minting an unpromoted pipeline draft on
	// the tip; the live pipeline must still pin v1.
	v2, err := client.CreateAIGatewayPolicyVersion(ctx, pol.ID, codersdk.CreateAIGatewayPolicyVersionRequest{
		Rego:     `default verdict := "ALLOW"`,
		Activate: true,
	})
	require.NoError(t, err)

	got, err := client.AIGatewayPipeline(ctx, pipe.ID)
	require.NoError(t, err)
	require.NotNil(t, got.ActiveVersion)
	require.Equal(t, pipe.ActiveVersionID, got.ActiveVersionID, "live pipeline version must be unchanged without promote")
	require.Len(t, got.ActiveVersion.Policies, 1)
	require.Equal(t, v1, got.ActiveVersion.Policies[0].PolicyVersionID, "live pipeline must still pin v1")

	// The mint created an unpromoted tip ahead of the active version: the
	// pipeline now reports drift, which the UI surfaces as a "promote" workqueue.
	require.NotNil(t, got.LatestVersionID)
	require.NotEqual(t, *got.ActiveVersionID, *got.LatestVersionID, "tip must be ahead of active after an unpromoted mint")
	require.True(t, got.HasUnpromotedChanges())

	// The versions endpoint lists both the live and the minted tip, newest first.
	versions, err := client.AIGatewayPipelineVersions(ctx, pipe.ID)
	require.NoError(t, err)
	require.Len(t, versions, 2)
	require.Equal(t, *got.LatestVersionID, versions[0].ID, "newest version first")

	// Promote the minted tip directly (the per-pipeline promote action the UI
	// uses): activate the pipeline's latest version.
	promoted, err := client.UpdateAIGatewayPipeline(ctx, pipe.ID, codersdk.UpdateAIGatewayPipelineRequest{
		ActiveVersionID: got.LatestVersionID,
	})
	require.NoError(t, err)
	require.Equal(t, *got.LatestVersionID, *promoted.ActiveVersionID)
	require.False(t, promoted.HasUnpromotedChanges(), "promoting the tip clears drift")
	require.Len(t, promoted.ActiveVersion.Policies, 1)
	require.Equal(t, v2.ID, promoted.ActiveVersion.Policies[0].PolicyVersionID, "promoted pipeline pins v2")
}
