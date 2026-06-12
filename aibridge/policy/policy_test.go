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

	const bananaMsg = "This request was blocked because it mentioned bananas."
	cases := []struct {
		name    string
		body    string
		want    policy.Verdict
		wantMsg string
	}{
		{"no_banana", `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"hello there"}]}`, policy.VerdictAllow, ""},
		{"string_content", `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"do you like banana?"}]}`, policy.VerdictBlock, bananaMsg},
		{"case_insensitive", `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"BANANA"}]}`, policy.VerdictBlock, bananaMsg},
		{"block_content", `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":[{"type":"text","text":"a banana split"}]}]}`, policy.VerdictBlock, bananaMsg},
		{"assistant_ignored", `{"model":"claude-sonnet-4-6","messages":[{"role":"assistant","content":"banana"},{"role":"user","content":"ok"}]}`, policy.VerdictAllow, ""},
		{"non_text_block", `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":[{"type":"image","source":{}}]}]}`, policy.VerdictAllow, ""},
		{"no_messages", `{"model":"claude-sonnet-4-6"}`, policy.VerdictAllow, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			res := d.Evaluate(t.Context(), buildInput(t, tc.body, policy.Identity{}))
			require.Nil(t, res.Err)
			require.Equal(t, tc.want, res.Verdict)
			require.Equal(t, tc.wantMsg, res.Message)
		})
	}
}

func TestDecide_Message(t *testing.T) {
	t.Parallel()

	t.Run("absent_when_no_message_rule", func(t *testing.T) {
		t.Parallel()
		d, err := policy.NewDecide("block-no-msg", `default verdict := "BLOCK"`)
		require.NoError(t, err)
		res := d.Evaluate(t.Context(), buildInput(t, `{"model":"x"}`, policy.Identity{}))
		require.Nil(t, res.Err)
		require.Equal(t, policy.VerdictBlock, res.Verdict)
		require.Empty(t, res.Message)
	})

	t.Run("absent_when_not_blocking", func(t *testing.T) {
		t.Parallel()
		// A message rule that is defined regardless of verdict is still not
		// surfaced unless the verdict blocks.
		d, err := policy.NewDecide("allow-with-msg", `
default verdict := "ALLOW"
message := "should not surface"
`)
		require.NoError(t, err)
		res := d.Evaluate(t.Context(), buildInput(t, `{"model":"x"}`, policy.Identity{}))
		require.Nil(t, res.Err)
		require.Equal(t, policy.VerdictAllow, res.Verdict)
		require.Empty(t, res.Message)
	})

	t.Run("malformed_message_ignored", func(t *testing.T) {
		t.Parallel()
		// A non-string message must not error or alter the verdict.
		d, err := policy.NewDecide("block-bad-msg", `
default verdict := "BLOCK"
message := 42
`)
		require.NoError(t, err)
		res := d.Evaluate(t.Context(), buildInput(t, `{"model":"x"}`, policy.Identity{}))
		require.Nil(t, res.Err)
		require.Equal(t, policy.VerdictBlock, res.Verdict)
		require.Empty(t, res.Message)
	})
}

func TestAnnotate_Annotations(t *testing.T) {
	t.Parallel()

	c, err := policy.NewAnnotate("request-shape-annotator", classificationPolicy)
	require.NoError(t, err)

	body := `{"model":"gpt-4o","messages":[{"role":"user","content":"a"},{"role":"user","content":"b"}],"tools":[{"name":"t"}],"stream":true}`
	res := c.Evaluate(t.Context(), buildInput(t, body, policy.Identity{}))
	require.Nil(t, res.Err)
	// Annotations are namespaced under the producing stage's name.
	ann, ok := res.Annotations["request-shape-annotator"].(map[string]any)
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
		res := r.Evaluate(t.Context(), buildInput(t, `{"model":"claude-opus-4-8"}`, policy.Identity{Groups: []string{"eng"}}))
		require.Nil(t, res.Err)
		require.Equal(t, "claude-sonnet-4-6", res.Route)
	})

	t.Run("premium_untouched", func(t *testing.T) {
		t.Parallel()
		res := r.Evaluate(t.Context(), buildInput(t, `{"model":"claude-opus-4-8"}`, policy.Identity{Groups: []string{"premium"}}))
		require.Nil(t, res.Err)
		require.Empty(t, res.Route)
	})

	t.Run("other_model_untouched", func(t *testing.T) {
		t.Parallel()
		res := r.Evaluate(t.Context(), buildInput(t, `{"model":"gpt-4o"}`, policy.Identity{}))
		require.Nil(t, res.Err)
		require.Empty(t, res.Route)
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

	// transformBody returns the JSON body a transform's root edit rewrites to.
	transformBody := func(t *testing.T, res policy.StageResult) map[string]any {
		t.Helper()
		require.Nil(t, res.Err)
		require.Len(t, res.Edits, 1)
		require.Empty(t, res.Edits[0].Pointer) // whole-body rewrite is the root edit
		b, err := json.Marshal(res.Edits[0].Value)
		require.NoError(t, err)
		var got map[string]any
		require.NoError(t, json.Unmarshal(b, &got))
		return got
	}

	t.Run("no_system_set", func(t *testing.T) {
		t.Parallel()
		res := tr.Evaluate(t.Context(), buildInput(t, `{"model":"claude-sonnet-4-6","max_tokens":1024}`, policy.Identity{}))
		got := transformBody(t, res)
		require.Equal(t, directive, got["system"])
		require.Equal(t, "claude-sonnet-4-6", got["model"]) // other fields preserved
	})

	t.Run("string_system_replaced", func(t *testing.T) {
		t.Parallel()
		res := tr.Evaluate(t.Context(), buildInput(t, `{"model":"claude-sonnet-4-6","system":"You are a helpful assistant."}`, policy.Identity{}))
		require.Equal(t, directive, transformBody(t, res)["system"])
	})

	t.Run("array_system_replaced", func(t *testing.T) {
		t.Parallel()
		res := tr.Evaluate(t.Context(), buildInput(t, `{"model":"claude-sonnet-4-6","system":[{"type":"text","text":"You are a helpful assistant."}]}`, policy.Identity{}))
		require.Equal(t, directive, transformBody(t, res)["system"])
	})

	t.Run("non_anthropic_noop", func(t *testing.T) {
		t.Parallel()
		res := tr.Evaluate(t.Context(), buildInput(t, `{"model":"gpt-4o"}`, policy.Identity{}))
		require.Nil(t, res.Err)
		require.Empty(t, res.Edits)
		require.Nil(t, res.Headers)
	})

	t.Run("headers_override", func(t *testing.T) {
		t.Parallel()
		hdr, err := policy.NewTransform("header-injector", `
headers := {"x-coder-policy": "applied"} if startswith(input.request.body.model, "claude")
`)
		require.NoError(t, err)
		res := hdr.Evaluate(t.Context(), buildInput(t, `{"model":"claude-sonnet-4-6"}`, policy.Identity{}))
		require.Nil(t, res.Err)
		require.Empty(t, res.Edits) // body rule undefined, only headers set
		require.Equal(t, map[string]string{"x-coder-policy": "applied"}, res.Headers)
	})

	t.Run("malformed_header_value_fails_closed", func(t *testing.T) {
		t.Parallel()
		// A malformed headers rule is a decode failure, synthesized through the
		// default fail mode (fail-closed) into a BLOCK with an audit-only error.
		hdr, err := policy.NewTransform("bad-header", `headers := {"x-num": 42}`)
		require.NoError(t, err)
		res := hdr.Evaluate(t.Context(), buildInput(t, `{"model":"claude-sonnet-4-6"}`, policy.Identity{}))
		require.Equal(t, policy.VerdictBlock, res.Verdict)
		require.NotNil(t, res.Err)
	})
}

func TestVerdict_Reduce(t *testing.T) {
	t.Parallel()

	require.Equal(t, policy.VerdictAllow, policy.ReduceVerdicts())
	require.Equal(t, policy.VerdictBlock, policy.ReduceVerdicts(policy.VerdictLog, policy.VerdictBlock))
	require.Equal(t, policy.VerdictLog, policy.ReduceVerdicts(policy.VerdictAllow, policy.VerdictLog))
	require.Equal(t, policy.VerdictAllow, policy.ReduceVerdicts(policy.VerdictAllow))
}
