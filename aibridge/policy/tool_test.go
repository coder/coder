package policy_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge/policy"
)

func buildToolInput(t *testing.T, call policy.ToolCall, body string, identity policy.Identity) policy.Input {
	t.Helper()
	in, err := policy.PreToolEnvelope{PreReqEnvelope: policy.PreReqEnvelope{Request: []byte(body), Identity: identity}, ToolCall: call}.
		Build()
	require.NoError(t, err)
	return in
}

func TestBuildToolInput_FieldsVisibleToDecide(t *testing.T) {
	t.Parallel()

	decide, err := policy.NewDecide("block-bash", `
default verdict := "ALLOW"
verdict := "BLOCK" if {
	input.tool_call.name == "bash"
	contains(input.tool_call.arguments.command, "rm -rf")
}
`)
	require.NoError(t, err)

	pipe, err := policy.NewToolPipeline(policy.PipelineConfig{Decide: []*policy.Decide{decide}})
	require.NoError(t, err)

	// Dangerous bash invocation is blocked.
	res, err := pipe.Evaluate(t.Context(), buildToolInput(t, policy.ToolCall{
		ID:        "toolu_1",
		Name:      "bash",
		Arguments: json.RawMessage(`{"command":"rm -rf /"}`),
		Index:     0,
	}, `{"model":"claude"}`, policy.Identity{}))
	require.NoError(t, err)
	require.Equal(t, policy.VerdictBlock, res.Verdict)
	require.Equal(t, "block-bash", res.BlockedBy)

	// A different tool with the same args passes.
	res, err = pipe.Evaluate(t.Context(), buildToolInput(t, policy.ToolCall{
		ID:        "toolu_2",
		Name:      "read_file",
		Arguments: json.RawMessage(`{"command":"rm -rf /"}`),
		Index:     1,
	}, `{"model":"claude"}`, policy.Identity{}))
	require.NoError(t, err)
	require.Equal(t, policy.VerdictAllow, res.Verdict)
}

func TestBuildToolInput_IndexAndIdentityVisible(t *testing.T) {
	t.Parallel()

	decide, err := policy.NewDecide("gate", `
default verdict := "ALLOW"
verdict := "LOG" if input.tool_call.index >= 2
verdict := "BLOCK" if "contractor" in input.identity.roles
`)
	require.NoError(t, err)
	pipe, err := policy.NewToolPipeline(policy.PipelineConfig{Decide: []*policy.Decide{decide}})
	require.NoError(t, err)

	res, err := pipe.Evaluate(t.Context(), buildToolInput(t, policy.ToolCall{Name: "x", Index: 3}, `{}`,
		policy.Identity{Roles: []string{"admin"}}))
	require.NoError(t, err)
	require.Equal(t, policy.VerdictLog, res.Verdict)

	res, err = pipe.Evaluate(t.Context(), buildToolInput(t, policy.ToolCall{Name: "x", Index: 0}, `{}`,
		policy.Identity{Roles: []string{"contractor"}}))
	require.NoError(t, err)
	require.Equal(t, policy.VerdictBlock, res.Verdict)
}

func TestBuildToolInput_EmptyArgsIsObject(t *testing.T) {
	t.Parallel()

	// arguments defaults to {} so object access does not error under strict
	// builtin errors.
	decide, err := policy.NewDecide("gate", `
default verdict := "ALLOW"
verdict := "BLOCK" if object.get(input.tool_call.arguments, "x", "") == "y"
`)
	require.NoError(t, err)
	pipe, err := policy.NewToolPipeline(policy.PipelineConfig{Decide: []*policy.Decide{decide}})
	require.NoError(t, err)

	res, err := pipe.Evaluate(t.Context(), buildToolInput(t, policy.ToolCall{Name: "x"}, `{}`, policy.Identity{}))
	require.NoError(t, err)
	require.Equal(t, policy.VerdictAllow, res.Verdict)
}

func TestBuildToolInput_InvalidArgsErrors(t *testing.T) {
	t.Parallel()

	_, err := policy.PreToolEnvelope{
		PreReqEnvelope: policy.PreReqEnvelope{Request: []byte(`{}`)},
		ToolCall: policy.ToolCall{
			Name:      "x",
			Arguments: json.RawMessage(`{not valid json`),
		},
	}.Build()
	require.Error(t, err)
}

func TestNewToolPipeline_RejectsRouteAndTransform(t *testing.T) {
	t.Parallel()

	route, err := policy.NewRoute("r", `model := "gpt-4o"`)
	require.NoError(t, err)
	_, err = policy.NewToolPipeline(policy.PipelineConfig{Route: route})
	require.ErrorContains(t, err, "route")

	tr, err := policy.NewTransform("t", `body := {}`)
	require.NoError(t, err)
	_, err = policy.NewToolPipeline(policy.PipelineConfig{Transform: []*policy.Transform{tr}})
	require.ErrorContains(t, err, "transform")
}

func TestKindValidAtHook(t *testing.T) {
	t.Parallel()

	cases := []struct {
		hook  policy.Hook
		kind  policy.Kind
		valid bool
	}{
		{policy.HookPreAuth, policy.KindClassify, true},
		{policy.HookPreAuth, policy.KindDecide, true},
		{policy.HookPreAuth, policy.KindRoute, false},
		{policy.HookPreAuth, policy.KindTransform, false},
		{policy.HookPreReq, policy.KindRoute, true},
		{policy.HookPreReq, policy.KindTransform, true},
		{policy.HookPreTool, policy.KindClassify, true},
		{policy.HookPreTool, policy.KindDecide, true},
		{policy.HookPreTool, policy.KindRoute, false},
		{policy.HookPreTool, policy.KindTransform, false},
		{policy.Hook("bogus"), policy.KindDecide, false},
	}
	for _, c := range cases {
		require.Equalf(t, c.valid, policy.KindValidAtHook(c.hook, c.kind),
			"hook=%s kind=%s", c.hook, c.kind)
	}
}
