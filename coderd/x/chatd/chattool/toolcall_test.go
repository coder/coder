package chattool_test

import (
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
)

func TestToolCallID(t *testing.T) {
	t.Parallel()

	// A changed value would make the agent run retried calls again.
	chatID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	require.Equal(t, uuid.MustParse("ca92e3e7-ee42-5889-b1df-75869f9117ee"), chattool.ToolCallID(chatID, 42, "call_1"))
}

func TestToolCallIDs(t *testing.T) {
	t.Parallel()

	chatID := uuid.New()
	tests := []struct {
		name        string
		providerIDs []string
		want        []string
	}{
		{name: "Unique", providerIDs: []string{"a", "b"}, want: []string{"a", "b"}},
		{name: "Empty", providerIDs: []string{"", "b"}, want: []string{"b"}},
		{name: "Repeated", providerIDs: []string{"a", "a", "b"}, want: []string{"b"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			calls := make([]fantasy.ToolCallContent, 0, len(tc.providerIDs))
			for _, id := range tc.providerIDs {
				calls = append(calls, fantasy.ToolCallContent{ToolCallID: id})
			}
			want := make(map[string]uuid.UUID, len(tc.want))
			for _, id := range tc.want {
				want[id] = chattool.ToolCallID(chatID, 7, id)
			}
			require.Equal(t, want, chattool.ToolCallIDs(chatID, 7, calls))
		})
	}
}
