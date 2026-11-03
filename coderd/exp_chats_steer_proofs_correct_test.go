package coderd_test

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// TestProofMessageThenInterruptCancelsCorrectionTurn sends a correction
// to a busy chat and then interrupts it. When a step boundary passes
// between the two requests, the worker has already delivered the
// correction and started the model call that answers it, so the
// interrupt cancels that call and the chat stops with the correction
// unanswered.
func TestProofMessageThenInterruptCancelsCorrectionTurn(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, model, chat, first := proofASetup(ctx, t)

	const correction = "proof-correction"
	proofASend(ctx, t, client, chat.ID, correction, "")

	// The step in flight ends with a tool call, so the turn continues and
	// the worker delivers the steer lane before the next model call.
	first.reply <- proofAReply{toolCall: true}
	require.Equal(t, "replied", testutil.TryReceive(ctx, t, first.outcome))
	second := model.next(ctx, t)
	require.True(t, slices.Contains(second.userTexts, correction),
		"the correction is delivered before the second model call")
	require.Empty(t, proofAQueue(ctx, t, client, chat.ID))

	// The interrupt arrives after that step boundary.
	_, err := client.InterruptChat(ctx, chat.ID)
	require.NoError(t, err)
	outcome := testutil.TryReceive(ctx, t, second.outcome)
	status := proofAWaitStatus(ctx, t, client, chat.ID, proofANot(codersdk.ChatStatusInterrupting))
	t.Logf("model call answering the correction: %s; status after the interrupt: %s; queue=%v",
		outcome, status, proofAQueueIDs(proofAQueue(ctx, t, client, chat.ID)))
	require.Equal(t, "replied", outcome,
		"message-then-interrupt must not cancel the model call that answers the correction")
}
