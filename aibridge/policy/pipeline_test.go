package policy_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge/policy"
)

// errModule errors at evaluation time under StrictBuiltinErrors: object.get on
// a string (input.request.body.model) is a builtin type error.
const errModule = `
default verdict := "ALLOW"
verdict := "BLOCK" if { object.get(input.request.body.model, "k", "") == "" }
`

func mustDecide(t *testing.T, module string, opts ...policy.Option) *policy.Decide {
	t.Helper()
	d, err := policy.NewDecide("test-decide", module, opts...)
	require.NoError(t, err)
	return d
}

func TestPipeline_ClassifyAnnotationVisibleToDecide(t *testing.T) {
	t.Parallel()

	classify, err := policy.NewClassify("test-classify", `
annotations := {"risk": "high"} if object.get(input.request.body, "max_tokens", 0) > 1000
`)
	require.NoError(t, err)
	// LOG (a pass-through verdict) so the pipeline completes and surfaces the
	// annotations; a BLOCK would short-circuit and is exercised separately.
	decide := mustDecide(t, `
default verdict := "ALLOW"
verdict := "LOG" if input.annotations.risk == "high"
`)

	pipe, err := policy.NewPipeline(policy.PipelineConfig{
		Classify: []*policy.Classify{classify},
		Decide:   []*policy.Decide{decide},
	})
	require.NoError(t, err)

	// High max_tokens -> classify sets risk=high -> decide (reading the
	// threaded annotation) flags. The LOG proves the annotation was visible.
	res, err := pipe.Evaluate(t.Context(), buildInput(t, `{"model":"gpt-4o","max_tokens":5000}`, policy.Identity{}))
	require.NoError(t, err)
	require.Equal(t, policy.VerdictLog, res.Verdict)
	require.Equal(t, "high", res.Annotations["risk"])

	// Low max_tokens -> no annotation -> allowed.
	res, err = pipe.Evaluate(t.Context(), buildInput(t, `{"model":"gpt-4o","max_tokens":100}`, policy.Identity{}))
	require.NoError(t, err)
	require.Equal(t, policy.VerdictAllow, res.Verdict)
}

func TestPipeline_RouteModelVisibleToDecide(t *testing.T) {
	t.Parallel()

	route, err := policy.NewRoute("test-route", `
model := "blocked-model" if input.request.body.model == "trigger"
`)
	require.NoError(t, err)
	// LOG (pass-through) so the rewritten body is surfaced rather than
	// short-circuited by a BLOCK.
	decide := mustDecide(t, `
default verdict := "ALLOW"
verdict := "LOG" if input.request.body.model == "blocked-model"
`)

	pipe, err := policy.NewPipeline(policy.PipelineConfig{
		Route:  route,
		Decide: []*policy.Decide{decide},
	})
	require.NoError(t, err)

	// Route rewrites the model; the later decide sees the rewritten value (LOG
	// only fires on the rewritten model).
	res, err := pipe.Evaluate(t.Context(), buildInput(t, `{"model":"trigger"}`, policy.Identity{}))
	require.NoError(t, err)
	require.Equal(t, policy.VerdictLog, res.Verdict)

	// The rewritten body is surfaced for the host to forward.
	require.NotNil(t, res.RequestBody)
	var got map[string]any
	require.NoError(t, json.Unmarshal(res.RequestBody, &got))
	require.Equal(t, "blocked-model", got["model"])
}

func TestPipeline_FailMode(t *testing.T) {
	t.Parallel()

	t.Run("closed_blocks", func(t *testing.T) {
		t.Parallel()
		pipe, err := policy.NewPipeline(policy.PipelineConfig{
			Decide: []*policy.Decide{mustDecide(t, errModule)}, // default FailClosed
		})
		require.NoError(t, err)
		res, err := pipe.Evaluate(t.Context(), buildInput(t, `{"model":"gpt-4o"}`, policy.Identity{}))
		require.NoError(t, err)
		require.Equal(t, policy.VerdictBlock, res.Verdict)
	})

	t.Run("open_skips", func(t *testing.T) {
		t.Parallel()
		pipe, err := policy.NewPipeline(policy.PipelineConfig{
			Decide: []*policy.Decide{mustDecide(t, errModule, policy.WithFailMode(policy.FailOpen))},
		})
		require.NoError(t, err)
		res, err := pipe.Evaluate(t.Context(), buildInput(t, `{"model":"gpt-4o"}`, policy.Identity{}))
		require.NoError(t, err)
		require.Equal(t, policy.VerdictAllow, res.Verdict)
	})
}

func TestPipeline_BlockShortCircuitsTransform(t *testing.T) {
	t.Parallel()

	decide := mustDecide(t, `
default verdict := "BLOCK"
`)
	tr, err := policy.NewTransform("test-transform", `
body := {"mutated": true}
`)
	require.NoError(t, err)
	pipe, err := policy.NewPipeline(policy.PipelineConfig{
		Decide:    []*policy.Decide{decide},
		Transform: []*policy.Transform{tr},
	})
	require.NoError(t, err)

	res, err := pipe.Evaluate(t.Context(), buildInput(t, `{"model":"gpt-4o"}`, policy.Identity{}))
	require.NoError(t, err)
	require.Equal(t, policy.VerdictBlock, res.Verdict)
	require.Nil(t, res.RequestBody) // transform did not run
}

func TestPipeline_CardinalityValidation(t *testing.T) {
	t.Parallel()

	c1 := func() *policy.Classify {
		c, err := policy.NewClassify("test-classify", `
annotations := {}
`)
		require.NoError(t, err)
		return c
	}
	_, err := policy.NewPipeline(policy.PipelineConfig{Classify: []*policy.Classify{c1(), c1()}})
	require.Error(t, err)
}
