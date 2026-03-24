package chat_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/scaletest/chat"
	"github.com/coder/coder/v2/scaletest/harness"
)

func TestNewSummary(t *testing.T) {
	t.Parallel()

	workspaceID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	templateID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	modelConfigID := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	startedAt := time.Date(2026, time.March, 17, 4, 0, 0, 0, time.UTC)
	completedAt := startedAt.Add(3 * time.Minute)
	followUpReleasedAt := startedAt.Add(90 * time.Second)
	results := harness.Results{
		TotalRuns: 25,
		TotalPass: 24,
		TotalFail: 1,
		Elapsed:   httpapi.Duration(3 * time.Minute),
		ElapsedMS: (3 * time.Minute).Milliseconds(),
	}

	t.Run("SharedWorkspaceMode", func(t *testing.T) {
		t.Parallel()

		summary := chat.NewSummary(chat.SummaryConfig{
			RunID:              "run-shared",
			WorkspaceMode:      chat.WorkspaceModeSharedWorkspace,
			WorkspaceID:        &workspaceID,
			WorkspaceCount:     1,
			ChatsPerWorkspace:  25,
			ModelConfigID:      &modelConfigID,
			Count:              25,
			Turns:              10,
			Prompt:             "Reply with one short sentence.",
			FollowUpPrompt:     "Continue.",
			FollowUpStartDelay: 45 * time.Second,
			LLMMockURL:         "http://127.0.0.1:6061/v1",
			OutputSpecs:        []string{"text", "json:/tmp/results.json"},
		}, results, startedAt, completedAt, &followUpReleasedAt)

		require.Equal(t, chat.WorkspaceModeSharedWorkspace, summary.WorkspaceMode)
		require.NotNil(t, summary.WorkspaceID)
		require.Equal(t, workspaceID, *summary.WorkspaceID)
		require.Nil(t, summary.TemplateID)
		require.Empty(t, summary.TemplateName)
		require.EqualValues(t, 1, summary.WorkspaceCount)
		require.EqualValues(t, 25, summary.ChatsPerWorkspace)
		require.Zero(t, summary.CreatedWorkspaceCount)
		require.True(t, summary.FollowUpDelayEnabled)
		require.Equal(t, followUpReleasedAt, *summary.FollowUpPhaseReleasedAt)
		require.Equal(t, 25, summary.Results.TotalRuns)
		require.Equal(t, int64(45000), summary.FollowUpStartDelayMS)
		require.Len(t, summary.RawOutputSpecs, 2)
		require.NotEmpty(t, summary.PromptFingerprint)
		require.NotEmpty(t, summary.FollowUpPromptFingerprint)

		compactJSON, err := summary.CompactJSON()
		require.NoError(t, err)
		require.Contains(t, string(compactJSON), `"workspace_mode":"shared_workspace"`)
		require.Contains(t, string(compactJSON), `"workspace_count":1`)
		require.Contains(t, string(compactJSON), `"chats_per_workspace":25`)
	})

	t.Run("TemplateMode", func(t *testing.T) {
		t.Parallel()

		summary := chat.NewSummary(chat.SummaryConfig{
			RunID:                 "run-template",
			WorkspaceMode:         chat.WorkspaceModeTemplate,
			TemplateID:            &templateID,
			TemplateName:          "starter-template",
			WorkspaceCount:        5,
			ChatsPerWorkspace:     5,
			CreatedWorkspaceCount: 5,
			ModelConfigID:         &modelConfigID,
			Count:                 25,
			Turns:                 1,
			Prompt:                "Reply with one short sentence.",
			FollowUpStartDelay:    0,
			OutputSpecs:           []string{"text"},
		}, results, startedAt, completedAt, nil)

		require.Equal(t, chat.WorkspaceModeTemplate, summary.WorkspaceMode)
		require.Nil(t, summary.WorkspaceID)
		require.NotNil(t, summary.TemplateID)
		require.Equal(t, templateID, *summary.TemplateID)
		require.Equal(t, "starter-template", summary.TemplateName)
		require.EqualValues(t, 5, summary.WorkspaceCount)
		require.EqualValues(t, 5, summary.ChatsPerWorkspace)
		require.EqualValues(t, 5, summary.CreatedWorkspaceCount)
		require.False(t, summary.FollowUpDelayEnabled)
		require.Empty(t, summary.FollowUpPromptFingerprint)

		compactJSON, err := summary.CompactJSON()
		require.NoError(t, err)
		require.Contains(t, string(compactJSON), `"template_name":"starter-template"`)
		require.Contains(t, string(compactJSON), `"workspace_count":5`)
		require.Contains(t, string(compactJSON), `"chats_per_workspace":5`)
		require.Contains(t, string(compactJSON), `"created_workspace_count":5`)
	})
}
