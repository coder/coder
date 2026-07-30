package chatd

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

// TestChatModelOverrideSiteConfigKey pins the fixed per-org key family: the
// four shared contexts under agents_chat_<context>_model_override, the
// advisor override under its own agents_advisor_model_override family, and
// rejection of unknown contexts. The org is always the UUID suffix, never
// the mutable organization name.
func TestChatModelOverrideSiteConfigKey(t *testing.T) {
	t.Parallel()

	orgID := uuid.MustParse("33333333-3333-3333-3333-333333333333")

	tests := []struct {
		context codersdk.ChatModelOverrideContext
		want    string
	}{
		{
			codersdk.ChatModelOverrideContextGeneral,
			"agents_chat_general_model_override:33333333-3333-3333-3333-333333333333",
		},
		{
			codersdk.ChatModelOverrideContextExplore,
			"agents_chat_explore_model_override:33333333-3333-3333-3333-333333333333",
		},
		{
			codersdk.ChatModelOverrideContextTitleGeneration,
			"agents_chat_title_generation_model_override:33333333-3333-3333-3333-333333333333",
		},
		{
			codersdk.ChatModelOverrideContextCompaction,
			"agents_chat_compaction_model_override:33333333-3333-3333-3333-333333333333",
		},
		{
			codersdk.ChatModelOverrideContextAdvisor,
			"agents_advisor_model_override:33333333-3333-3333-3333-333333333333",
		},
	}
	for _, tt := range tests {
		t.Run(string(tt.context), func(t *testing.T) {
			t.Parallel()
			got, err := ChatModelOverrideSiteConfigKey(tt.context, orgID)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}

	t.Run("UnknownRejected", func(t *testing.T) {
		t.Parallel()
		_, err := ChatModelOverrideSiteConfigKey("bogus", orgID)
		require.ErrorContains(t, err, "unsupported chat model override context")
	})
}

// TestReadSubagentModelOverrideRejectsNonSubagentContexts proves the advisor
// context (and every other non-subagent context) is NOT resolvable as a
// subagent override context: the enum gained advisor as a fifth value, and
// the reader's switch must not bleed it into subagent spawn resolution.
func TestReadSubagentModelOverrideRejectsNonSubagentContexts(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()

	for _, overrideContext := range []codersdk.ChatModelOverrideContext{
		codersdk.ChatModelOverrideContextAdvisor,
		codersdk.ChatModelOverrideContextTitleGeneration,
		codersdk.ChatModelOverrideContextCompaction,
		"bogus",
	} {
		t.Run(string(overrideContext), func(t *testing.T) {
			t.Parallel()
			ctx := chatdTestContext(t)
			_, err := readSubagentModelOverride(ctx, nil, orgID, overrideContext)
			require.ErrorContains(t, err, "unsupported subagent model override context")
		})
	}
}
