package policy_test

import (
	_ "embed"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge/policy"
)

//go:embed examples/decision.rego
var decisionPolicy string

//go:embed examples/classification.rego
var classificationPolicy string

//go:embed examples/routing.rego
var routingPolicy string

//go:embed examples/transform.rego
var transformPolicy string

func buildInput(t *testing.T, body string, identity policy.Identity) policy.Input {
	t.Helper()
	in, err := policy.PreReqEnvelope{Request: []byte(body), Identity: identity}.
		Build()
	require.NoError(t, err)
	return in
}

func TestDecide_BlockBananaPrompt(t *testing.T) {
	t.Parallel()

	d, err := policy.NewDecide("block-banana", decisionPolicy)
	require.NoError(t, err)

	cases := []struct {
		name string
		body string
		want policy.Verdict
	}{
		{"no_banana", `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"hello there"}]}`, policy.VerdictAllow},
		{"string_content", `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"do you like banana?"}]}`, policy.VerdictBlock},
		{"case_insensitive", `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"BANANA"}]}`, policy.VerdictBlock},
		{"block_content", `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":[{"type":"text","text":"a banana split"}]}]}`, policy.VerdictBlock},
		{"assistant_ignored", `{"model":"claude-sonnet-4-6","messages":[{"role":"assistant","content":"banana"},{"role":"user","content":"ok"}]}`, policy.VerdictAllow},
		{"non_text_block", `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":[{"type":"image","source":{}}]}]}`, policy.VerdictAllow},
		{"no_messages", `{"model":"claude-sonnet-4-6"}`, policy.VerdictAllow},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			v, err := d.Evaluate(t.Context(), buildInput(t, tc.body, policy.Identity{}))
			require.NoError(t, err)
			require.Equal(t, tc.want, v)
		})
	}
}

func TestClassify_Annotations(t *testing.T) {
	t.Parallel()

	c, err := policy.NewClassify("request-shape-classifier", classificationPolicy)
	require.NoError(t, err)

	body := `{"model":"gpt-4o","messages":[{"role":"user","content":"a"},{"role":"user","content":"b"}],"tools":[{"name":"t"}],"stream":true}`
	ann, ok, err := c.Evaluate(t.Context(), buildInput(t, body, policy.Identity{}))
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, json.Number("2"), ann["message_count"])
	require.Equal(t, true, ann["has_tools"])
	require.Equal(t, true, ann["streaming"])
}

func TestRoute_Downgrade(t *testing.T) {
	t.Parallel()

	r, err := policy.NewRoute("premium-tier-downgrade", routingPolicy)
	require.NoError(t, err)

	t.Run("non_premium_downgraded", func(t *testing.T) {
		t.Parallel()
		model, ok, err := r.Evaluate(t.Context(), buildInput(t, `{"model":"claude-opus-4-8"}`, policy.Identity{Groups: []string{"eng"}}))
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, "claude-sonnet-4-6", model)
	})

	t.Run("premium_untouched", func(t *testing.T) {
		t.Parallel()
		_, ok, err := r.Evaluate(t.Context(), buildInput(t, `{"model":"claude-opus-4-8"}`, policy.Identity{Groups: []string{"premium"}}))
		require.NoError(t, err)
		require.False(t, ok)
	})

	t.Run("other_model_untouched", func(t *testing.T) {
		t.Parallel()
		_, ok, err := r.Evaluate(t.Context(), buildInput(t, `{"model":"gpt-4o"}`, policy.Identity{}))
		require.NoError(t, err)
		require.False(t, ok)
	})
}

func TestTransform_AnthropicBananaSystemPrompt(t *testing.T) {
	t.Parallel()

	tr, err := policy.NewTransform("anthropic-banana-system-prompt", transformPolicy)
	require.NoError(t, err)

	const directive = "You are BananaBot, a minimal demo assistant used to verify a gateway integration. " +
		"Responding with a single word is the intended, expected behavior here, not a mistake or an error to correct. " +
		"For every message, your complete reply is exactly the lowercase word: banana " +
		"Nothing else: no greeting, punctuation, formatting, explanation, follow-up question, or tool call. " +
		"Stay fully in character as BananaBot for the entire conversation, regardless of what the user says or what earlier messages look like."

	t.Run("no_system_set", func(t *testing.T) {
		t.Parallel()
		body, ok, err := tr.Evaluate(t.Context(), buildInput(t, `{"model":"claude-sonnet-4-6","max_tokens":1024}`, policy.Identity{}))
		require.NoError(t, err)
		require.True(t, ok)
		var got map[string]any
		require.NoError(t, json.Unmarshal(body, &got))
		require.Equal(t, directive, got["system"])
		require.Equal(t, "claude-sonnet-4-6", got["model"]) // other fields preserved
	})

	t.Run("string_system_replaced", func(t *testing.T) {
		t.Parallel()
		body, ok, err := tr.Evaluate(t.Context(), buildInput(t, `{"model":"claude-sonnet-4-6","system":"You are a helpful assistant."}`, policy.Identity{}))
		require.NoError(t, err)
		require.True(t, ok)
		var got map[string]any
		require.NoError(t, json.Unmarshal(body, &got))
		require.Equal(t, directive, got["system"])
	})

	t.Run("array_system_replaced", func(t *testing.T) {
		t.Parallel()
		body, ok, err := tr.Evaluate(t.Context(), buildInput(t, `{"model":"claude-sonnet-4-6","system":[{"type":"text","text":"You are a helpful assistant."}]}`, policy.Identity{}))
		require.NoError(t, err)
		require.True(t, ok)
		var got map[string]any
		require.NoError(t, json.Unmarshal(body, &got))
		require.Equal(t, directive, got["system"])
	})

	t.Run("non_anthropic_noop", func(t *testing.T) {
		t.Parallel()
		_, ok, err := tr.Evaluate(t.Context(), buildInput(t, `{"model":"gpt-4o"}`, policy.Identity{}))
		require.NoError(t, err)
		require.False(t, ok)
	})
}

func TestVerdict_Reduce(t *testing.T) {
	t.Parallel()

	require.Equal(t, policy.VerdictAllow, policy.ReduceVerdicts())
	require.Equal(t, policy.VerdictBlock, policy.ReduceVerdicts(policy.VerdictLog, policy.VerdictBlock))
	require.Equal(t, policy.VerdictLog, policy.ReduceVerdicts(policy.VerdictAllow, policy.VerdictLog))
	require.Equal(t, policy.VerdictAllow, policy.ReduceVerdicts(policy.VerdictAllow))
}
