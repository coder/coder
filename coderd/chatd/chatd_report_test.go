package chatd

import (
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/require"
)

func TestPrepareAgentStepResult_ReportOnly(t *testing.T) {
	t.Parallel()

	sentinel := "__sentinel__"
	result := prepareAgentStepResult(
		[]fantasy.Message{
			{
				Role: fantasy.MessageRoleUser,
				Content: []fantasy.MessagePart{
					fantasy.TextPart{Text: sentinel},
				},
			},
			{
				Role: fantasy.MessageRoleUser,
				Content: []fantasy.MessagePart{
					fantasy.TextPart{Text: "real message"},
				},
			},
		},
		sentinel,
		true,
	)

	require.Equal(t, []string{toolAgentReport}, result.ActiveTools)
	require.Len(t, result.Messages, 1)
	require.Equal(t, fantasy.MessageRoleUser, result.Messages[0].Role)
}
