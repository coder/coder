package guardrail_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"

	"github.com/coder/coder/v2/aibridge/guardrail"
	"github.com/coder/coder/v2/aibridge/policy"
)

// fake is a Guardrail with a fixed result/error, for exercising the Stage.
type fake struct {
	name string
	res  guardrail.Result
	err  error
}

func (f fake) Name() string { return f.name }
func (f fake) Evaluate(context.Context, guardrail.Request) (guardrail.Result, error) {
	return f.res, f.err
}

const body = `{"messages":[{"role":"user","content":"hi"}]}`

func TestStage_EnforcingEditAppliedAndAnnotated(t *testing.T) {
	t.Parallel()
	g := fake{name: "redactor", res: guardrail.Result{
		Annotations: map[string]any{"entities": map[string]int{"EMAIL_ADDRESS": 1}},
		Edits:       []guardrail.Edit{{Pointer: "messages.0.content", Value: "<REDACTED>"}},
	}}
	st, err := guardrail.NewStage(guardrail.Member{Guardrail: g})
	require.NoError(t, err)

	res, err := st.Run(context.Background(), []byte(body), "gpt-4")
	require.NoError(t, err)
	require.False(t, res.Verdict.Blocks())
	require.Equal(t, "<REDACTED>", gjson.GetBytes(res.RequestBody, "messages.0.content").String())
	require.Contains(t, res.Annotations, "redactor")
}

func TestStage_BlockWinsLowestName(t *testing.T) {
	t.Parallel()
	a := fake{name: "a", res: guardrail.Result{Action: guardrail.ActionBlock, Reason: "a-reason"}}
	b := fake{name: "b", res: guardrail.Result{Action: guardrail.ActionBlock, Reason: "b-reason"}}
	st, err := guardrail.NewStage(
		guardrail.Member{Guardrail: b},
		guardrail.Member{Guardrail: a},
	)
	require.NoError(t, err)

	res, err := st.Run(context.Background(), []byte(body), "")
	require.NoError(t, err)
	require.True(t, res.Verdict.Blocks())
	require.Equal(t, "a", res.BlockedBy)
	require.Equal(t, "a-reason", res.Message)
}

func TestStage_BlockSuppressesBodyEdit(t *testing.T) {
	t.Parallel()
	masker := fake{name: "masker", res: guardrail.Result{
		Edits: []guardrail.Edit{{Pointer: "messages.0.content", Value: "<REDACTED>"}},
	}}
	blocker := fake{name: "blocker", res: guardrail.Result{Action: guardrail.ActionBlock, Reason: "no"}}
	st, err := guardrail.NewStage(
		guardrail.Member{Guardrail: masker},
		guardrail.Member{Guardrail: blocker},
	)
	require.NoError(t, err)

	res, err := st.Run(context.Background(), []byte(body), "")
	require.NoError(t, err)
	require.True(t, res.Verdict.Blocks())
	require.Nil(t, res.RequestBody)
}

func TestStage_AnnotationsOnlyDoesNotBlockOrMutate(t *testing.T) {
	t.Parallel()
	// A scorer-style guardrail returns only annotations. Authority is intrinsic
	// (there is no advisory/enforcing mode), so an outcome with no block and no
	// edits neither blocks nor mutates the body; a downstream decide turns the
	// score into a verdict.
	g := fake{name: "scorer", res: guardrail.Result{
		Annotations: map[string]any{"score": 0.9},
	}}
	st, err := guardrail.NewStage(guardrail.Member{Guardrail: g})
	require.NoError(t, err)

	res, err := st.Run(context.Background(), []byte(body), "")
	require.NoError(t, err)
	require.False(t, res.Verdict.Blocks())
	require.Nil(t, res.RequestBody)
	require.Contains(t, res.Annotations, "scorer")
}

func TestStage_FailModes(t *testing.T) {
	t.Parallel()
	boom := fake{name: "boom", err: context.DeadlineExceeded}

	closed, err := guardrail.NewStage(guardrail.Member{Guardrail: boom, FailMode: guardrail.FailClosed})
	require.NoError(t, err)
	res, err := closed.Run(context.Background(), []byte(body), "")
	require.NoError(t, err)
	require.True(t, res.Verdict.Blocks())
	// A synthesized failure block is anonymous to the client; the failing
	// guardrail's identity rides the audit-only error record instead.
	require.Empty(t, res.BlockedBy)
	require.Len(t, res.Errors, 1)
	require.Equal(t, "boom", res.Errors[0].Stage)

	open, err := guardrail.NewStage(guardrail.Member{Guardrail: boom, FailMode: guardrail.FailOpen})
	require.NoError(t, err)
	res, err = open.Run(context.Background(), []byte(body), "")
	require.NoError(t, err)
	// Fail-open synthesizes LOG (visible), never a silent pass-through.
	require.False(t, res.Verdict.Blocks())
	require.Equal(t, policy.VerdictLog, res.Verdict)
	require.Len(t, res.Errors, 1)
}

func TestStage_DuplicateNameRejected(t *testing.T) {
	t.Parallel()
	_, err := guardrail.NewStage(
		guardrail.Member{Guardrail: fake{name: "dup"}},
		guardrail.Member{Guardrail: fake{name: "dup"}},
	)
	require.ErrorContains(t, err, "duplicate guardrail name")
}
