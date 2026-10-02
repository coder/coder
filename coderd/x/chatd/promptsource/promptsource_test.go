package promptsource_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/promptsource"
)

func TestRegistry(t *testing.T) {
	t.Parallel()

	seen := make(map[string]bool, len(promptsource.All))
	for _, s := range promptsource.All {
		require.True(t, strings.HasPrefix(s.Tag, promptsource.TagPrefix), "tag %q lacks prefix", s.Tag)
		require.False(t, seen[s.Tag], "duplicate tag %q", s.Tag)
		seen[s.Tag] = true
		require.NotEmpty(t, s.Origin, s.Tag)
		require.NotEmpty(t, s.Description, s.Tag)
		require.Contains(t, promptsource.GuideBlock, "<"+s.Tag+">", "guide omits %q", s.Tag)
	}
}

func TestWrap(t *testing.T) {
	t.Parallel()

	require.Equal(t,
		"<coder-agents-user-instructions>\nbe terse\n</coder-agents-user-instructions>",
		promptsource.UserInstructions.Wrap("  be terse\n"),
	)
	require.Empty(t, promptsource.UserInstructions.Wrap(" \n\t"))
	require.True(t, strings.HasPrefix(promptsource.GuideBlock, promptsource.Guide.Open()+"\n"))
	require.True(t, strings.HasSuffix(promptsource.GuideBlock, "\n"+promptsource.Guide.Close()))
}
