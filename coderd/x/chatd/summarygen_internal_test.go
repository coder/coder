package chatd

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/codersdk"
)

func summaryTestMessage(
	t *testing.T,
	id int64,
	role database.ChatMessageRole,
	visibility database.ChatMessageVisibility,
	parts []codersdk.ChatMessagePart,
	compressed bool,
	createdAt time.Time,
) database.ChatMessage {
	t.Helper()
	content, err := chatprompt.MarshalParts(parts)
	require.NoError(t, err)
	return database.ChatMessage{
		ID:             id,
		Role:           role,
		Visibility:     visibility,
		Content:        content,
		ContentVersion: chatprompt.CurrentContentVersion,
		Compressed:     compressed,
		CreatedAt:      createdAt,
	}
}

func summaryTextMessage(
	t *testing.T,
	id int64,
	role database.ChatMessageRole,
	visibility database.ChatMessageVisibility,
	text string,
	compressed bool,
	createdAt time.Time,
) database.ChatMessage {
	t.Helper()
	return summaryTestMessage(t, id, role, visibility,
		[]codersdk.ChatMessagePart{codersdk.ChatMessageText(text)},
		compressed, createdAt)
}

func TestRenderChatSummaryTranscript(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC)
	messages := []database.ChatMessage{
		summaryTextMessage(t, 1, database.ChatMessageRoleUser, database.ChatMessageVisibilityModel, "you are a helpful agent", false, base),
		// Compaction summary (model-only but compressed) is kept.
		summaryTextMessage(t, 2, database.ChatMessageRoleUser, database.ChatMessageVisibilityModel, "earlier work compaction summary", true, base.Add(time.Minute)),
		// Injected context (model-only, not compressed) is skipped as noise.
		summaryTextMessage(t, 3, database.ChatMessageRoleUser, database.ChatMessageVisibilityModel, "AGENTS.md injected context", false, base.Add(2*time.Minute)),
		summaryTextMessage(t, 4, database.ChatMessageRoleUser, database.ChatMessageVisibilityBoth, "fix the bug in foo.go", false, base.Add(3*time.Minute)),
		summaryTextMessage(t, 8, database.ChatMessageRoleUser, database.ChatMessageVisibilityUser, "and please keep it simple", false, base.Add(3*time.Minute+30*time.Second)),
		summaryTestMessage(t, 5, database.ChatMessageRoleAssistant, database.ChatMessageVisibilityBoth,
			[]codersdk.ChatMessagePart{codersdk.ChatMessageToolCall("call-1", "bash", []byte(`{"cmd":"go test"}`))},
			false, base.Add(4*time.Minute)),
		summaryTextMessage(t, 6, database.ChatMessageRoleTool, database.ChatMessageVisibilityBoth, "tests passed", false, base.Add(5*time.Minute)),
		summaryTextMessage(t, 7, database.ChatMessageRoleAssistant, database.ChatMessageVisibilityBoth, "fixed the bug and added a test", false, base.Add(6*time.Minute)),
	}

	transcript := renderChatSummaryTranscript(messages)

	require.Contains(t, transcript, "earlier work compaction summary")
	require.Contains(t, transcript, "[user]: fix the bug in foo.go")
	require.Contains(t, transcript, "[user]: and please keep it simple")
	require.Contains(t, transcript, "[assistant]: fixed the bug and added a test")
	// System prompt, injected context, tool-call, and tool result are excluded.
	require.NotContains(t, transcript, "you are a helpful agent")
	require.NotContains(t, transcript, "AGENTS.md injected context")
	require.NotContains(t, transcript, "tests passed")
	require.NotContains(t, transcript, "go test")
}

func TestBoundTranscriptHeadTail(t *testing.T) {
	t.Parallel()

	t.Run("UnderBudgetReturnsAll", func(t *testing.T) {
		t.Parallel()
		lines := []string{"a", "b", "c"}
		require.Equal(t, "a\nb\nc", boundTranscriptHeadTail(lines, 1000))
	})

	t.Run("OverBudgetKeepsHeadAndTail", func(t *testing.T) {
		t.Parallel()
		lines := []string{
			"HEAD-FIRST " + strings.Repeat("x", 40),
			strings.Repeat("m", 200),
			strings.Repeat("n", 200),
			strings.Repeat("o", 200),
			"TAIL-LAST " + strings.Repeat("y", 40),
		}
		out := boundTranscriptHeadTail(lines, 160)
		require.Contains(t, out, "HEAD-FIRST")
		require.Contains(t, out, "TAIL-LAST")
		require.Contains(t, out, "[... earlier turns omitted ...]")
	})
}

func TestShouldGenerateChatSummary(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC)
	userMsg := func(id int64, at time.Time) database.ChatMessage {
		return database.ChatMessage{
			ID:         id,
			Role:       database.ChatMessageRoleUser,
			Visibility: database.ChatMessageVisibilityBoth,
			CreatedAt:  at,
		}
	}
	assistantMsg := func(id int64, at time.Time) database.ChatMessage {
		return database.ChatMessage{
			ID:         id,
			Role:       database.ChatMessageRoleAssistant,
			Visibility: database.ChatMessageVisibilityBoth,
			CreatedAt:  at,
		}
	}

	t.Run("FirstSummaryAtFirstTurn", func(t *testing.T) {
		t.Parallel()
		chat := database.Chat{}
		msgs := []database.ChatMessage{
			userMsg(1, base),
			assistantMsg(2, base.Add(time.Minute)),
		}
		require.True(t, shouldGenerateChatSummary(chat, msgs))
	})

	t.Run("FirstSummarySkippedWithNoTurns", func(t *testing.T) {
		t.Parallel()
		chat := database.Chat{}
		require.False(t, shouldGenerateChatSummary(chat, nil))
	})

	t.Run("MultiStepTurnDoesNotInflateCount", func(t *testing.T) {
		t.Parallel()
		marker := base
		chat := database.Chat{
			Summary:            sql.NullString{String: "existing", Valid: true},
			SummaryGeneratedAt: sql.NullTime{Time: marker, Valid: true},
		}
		// One user turn plus many assistant steps stays below the threshold.
		msgs := []database.ChatMessage{
			userMsg(1, marker.Add(time.Minute)),
			assistantMsg(2, marker.Add(2*time.Minute)),
			assistantMsg(3, marker.Add(3*time.Minute)),
			assistantMsg(4, marker.Add(4*time.Minute)),
		}
		require.False(t, shouldGenerateChatSummary(chat, msgs))
	})

	t.Run("ModelOnlyUserMessagesAreNotTurns", func(t *testing.T) {
		t.Parallel()
		marker := base
		chat := database.Chat{
			Summary:            sql.NullString{String: "existing", Valid: true},
			SummaryGeneratedAt: sql.NullTime{Time: marker, Valid: true},
		}
		modelOnlyUserMsg := func(id int64, at time.Time) database.ChatMessage {
			return database.ChatMessage{
				ID:         id,
				Role:       database.ChatMessageRoleUser,
				Visibility: database.ChatMessageVisibilityModel,
				CreatedAt:  at,
			}
		}
		// The model-only user message must not count as a turn, else these
		// three messages would trip the threshold of 3.
		msgs := []database.ChatMessage{
			userMsg(1, marker.Add(time.Minute)),
			modelOnlyUserMsg(2, marker.Add(2*time.Minute)),
			userMsg(3, marker.Add(3*time.Minute)),
		}
		require.False(t, shouldGenerateChatSummary(chat, msgs))
	})

	t.Run("RefreshAfterThresholdTurns", func(t *testing.T) {
		t.Parallel()
		marker := base
		chat := database.Chat{
			Summary:            sql.NullString{String: "existing", Valid: true},
			SummaryGeneratedAt: sql.NullTime{Time: marker, Valid: true},
		}
		msgs := []database.ChatMessage{
			// Pre-marker turn is not counted.
			userMsg(1, marker.Add(-time.Minute)),
			userMsg(2, marker.Add(time.Minute)),
			userMsg(3, marker.Add(2*time.Minute)),
			userMsg(4, marker.Add(3*time.Minute)),
		}
		require.True(t, shouldGenerateChatSummary(chat, msgs))
	})

	t.Run("PreMarkerTurnsAreNotCounted", func(t *testing.T) {
		t.Parallel()
		marker := base
		chat := database.Chat{
			Summary:            sql.NullString{String: "existing", Valid: true},
			SummaryGeneratedAt: sql.NullTime{Time: marker, Valid: true},
		}
		// The pre-marker turn would tip the total to the threshold; this stays
		// false only because countCompletedTurnsSince excludes pre-marker turns.
		msgs := []database.ChatMessage{
			userMsg(1, marker.Add(-time.Minute)),
			userMsg(2, marker.Add(time.Minute)),
			userMsg(3, marker.Add(2*time.Minute)),
		}
		require.False(t, shouldGenerateChatSummary(chat, msgs))
	})
}

func TestValidateGeneratedChatSummary(t *testing.T) {
	t.Parallel()

	require.Error(t, validateGeneratedChatSummary(""))
	require.Error(t, validateGeneratedChatSummary(strings.Repeat("a", summaryMaxRunes+1)))
	require.Error(t, validateGeneratedChatSummary("One. Two. Three. Four. Five. Six. Seven."))
	require.NoError(t, validateGeneratedChatSummary("Implemented the summary feature. Added tests."))
}

func TestCountSentenceTerminators(t *testing.T) {
	t.Parallel()

	// Periods inside dotted identifiers (pkg.cmd.server) are not boundaries.
	require.Equal(t, 2, countSentenceTerminators("Fixed pkg.cmd.server in file.go. Added a test."))
	require.Equal(t, 3, countSentenceTerminators("One. Two! Three?"))
	require.Equal(t, 0, countSentenceTerminators("auth.rbac.Policy"))

	// Dotted identifiers must not push a valid summary over the sentence cap.
	require.NoError(t, validateGeneratedChatSummary(
		"Refactored pkg.cmd.server and auth.rbac.Policy in main.go and util.go. Added coverage in foo_test.go.",
	))
}

func TestSubagentReportSummarySnippet(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		report string
		want   string
	}{
		{
			name:   "ProseLeadKeepsFirstSentences",
			report: "Done. Both fixes are pushed as separate commits. Validation passed. Extra detail here.\n\nMore paragraphs follow.",
			want:   "Done. Both fixes are pushed as separate commits. Validation passed.",
		},
		{
			name:   "SkipsLeadingHeading",
			report: "## Summary\n\nFixed the flaky test by pinning the clock.",
			want:   "Fixed the flaky test by pinning the clock.",
		},
		{
			name:   "StripsInlineMarkdown",
			report: "Fixed **the race** in `cache.go`; see [the PR](https://example.com) for details.",
			want:   "Fixed the race in cache.go; see the PR for details.",
		},
		{
			name:   "JoinsWrappedLines",
			report: "Fixed the race\nin the cache layer.\n\nDetails below.",
			want:   "Fixed the race in the cache layer.",
		},
		{
			name:   "BulletLeadReport",
			report: "- Fixed A.\n- Fixed B.\n- Fixed C.\n- Fixed D.",
			want:   "Fixed A. Fixed B. Fixed C.",
		},
		{
			name:   "SkipsLeadingCodeFence",
			report: "```\ngo test ./...\n```\n\nAll tests passed.",
			want:   "All tests passed.",
		},
		{
			name:   "FenceEndsParagraph",
			report: "Ran the suite:\n```\nok 12 packages\n```\nThen more prose.",
			want:   "Ran the suite:",
		},
		{
			name:   "SkipsTableAndRule",
			report: "| a | b |\n|---|---|\n\n---\n\nRolled out the migration.",
			want:   "Rolled out the migration.",
		},
		{
			name:   "PreservesSnakeCaseIdentifiers",
			report: "Renamed parent_chat_id to root_chat_id in the query.",
			want:   "Renamed parent_chat_id to root_chat_id in the query.",
		},
		{
			name:   "TruncatesUnterminatedText",
			report: strings.Repeat("a", subagentReportSummaryMaxRunes+100),
			want:   strings.Repeat("a", subagentReportSummaryMaxRunes-1) + "…",
		},
		{
			name: "DropsSentencesPastRuneCap",
			report: "Short lead sentence. " +
				strings.Repeat("b", subagentReportSummaryMaxRunes) + ".",
			want: "Short lead sentence.",
		},
		{
			name:   "EmptyReport",
			report: "  \n\t\n",
			want:   "",
		},
		{
			name:   "OnlyCodeAndHeadings",
			report: "## Log\n```\nstack trace\n```\n",
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, subagentReportSummarySnippet(tt.report))
		})
	}
}
