package db2sdk_test

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisionersdk/proto"
)

func TestProvisionerJobStatus(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		job    database.ProvisionerJob
		status codersdk.ProvisionerJobStatus
	}{
		{
			name: "canceling",
			job: database.ProvisionerJob{
				CanceledAt: sql.NullTime{
					Time:  dbtime.Now().Add(-time.Minute),
					Valid: true,
				},
			},
			status: codersdk.ProvisionerJobCanceling,
		},
		{
			name: "canceled",
			job: database.ProvisionerJob{
				CanceledAt: sql.NullTime{
					Time:  dbtime.Now().Add(-time.Minute),
					Valid: true,
				},
				CompletedAt: sql.NullTime{
					Time:  dbtime.Now().Add(-30 * time.Second),
					Valid: true,
				},
			},
			status: codersdk.ProvisionerJobCanceled,
		},
		{
			name: "canceled_failed",
			job: database.ProvisionerJob{
				CanceledAt: sql.NullTime{
					Time:  dbtime.Now().Add(-time.Minute),
					Valid: true,
				},
				CompletedAt: sql.NullTime{
					Time:  dbtime.Now().Add(-30 * time.Second),
					Valid: true,
				},
				Error: sql.NullString{String: "badness", Valid: true},
			},
			status: codersdk.ProvisionerJobFailed,
		},
		{
			name:   "pending",
			job:    database.ProvisionerJob{},
			status: codersdk.ProvisionerJobPending,
		},
		{
			name: "succeeded",
			job: database.ProvisionerJob{
				StartedAt: sql.NullTime{
					Time:  dbtime.Now().Add(-time.Minute),
					Valid: true,
				},
				CompletedAt: sql.NullTime{
					Time:  dbtime.Now().Add(-30 * time.Second),
					Valid: true,
				},
			},
			status: codersdk.ProvisionerJobSucceeded,
		},
		{
			name: "completed_failed",
			job: database.ProvisionerJob{
				StartedAt: sql.NullTime{
					Time:  dbtime.Now().Add(-time.Minute),
					Valid: true,
				},
				CompletedAt: sql.NullTime{
					Time:  dbtime.Now().Add(-30 * time.Second),
					Valid: true,
				},
				Error: sql.NullString{String: "badness", Valid: true},
			},
			status: codersdk.ProvisionerJobFailed,
		},
		{
			name: "updated",
			job: database.ProvisionerJob{
				StartedAt: sql.NullTime{
					Time:  dbtime.Now().Add(-time.Minute),
					Valid: true,
				},
				UpdatedAt: dbtime.Now(),
			},
			status: codersdk.ProvisionerJobRunning,
		},
	}

	// Share db for all job inserts.
	db, _ := dbtestutil.NewDB(t)
	org := dbgen.Organization(t, db, database.Organization{})

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Populate standard fields
			now := dbtime.Now().Round(time.Minute)
			tc.job.ID = uuid.New()
			tc.job.CreatedAt = now
			tc.job.UpdatedAt = now
			tc.job.InitiatorID = org.ID
			tc.job.OrganizationID = org.ID
			tc.job.Input = []byte("{}")
			tc.job.Provisioner = database.ProvisionerTypeEcho
			// Unique tags for each job.
			tc.job.Tags = map[string]string{fmt.Sprintf("%d", i): "true"}

			inserted := dbgen.ProvisionerJob(t, db, nil, tc.job)
			// Make sure the inserted job has the right values.
			require.Equal(t, tc.job.StartedAt.Time.UTC(), inserted.StartedAt.Time.UTC(), "started at")
			require.Equal(t, tc.job.CompletedAt.Time.UTC(), inserted.CompletedAt.Time.UTC(), "completed at")
			require.Equal(t, tc.job.CanceledAt.Time.UTC(), inserted.CanceledAt.Time.UTC(), "canceled at")
			require.Equal(t, tc.job.Error, inserted.Error, "error")
			require.Equal(t, tc.job.ErrorCode, inserted.ErrorCode, "error code")

			actual := codersdk.ProvisionerJobStatus(inserted.JobStatus)
			require.Equal(t, tc.status, actual)
		})
	}
}

func TestTemplateVersionParameter_OK(t *testing.T) {
	t.Parallel()
	req := require.New(t)

	// In this test we're just going to cover the fields that have to get parsed.
	options := []*proto.RichParameterOption{
		{
			Name:        "foo",
			Description: "bar",
			Value:       "baz",
			Icon:        "David Bowie",
		},
	}
	ob, err := json.Marshal(&options)
	req.NoError(err)

	db := database.TemplateVersionParameter{
		Options:     json.RawMessage(ob),
		Description: "_The Rise and Fall of **Ziggy Stardust** and the Spiders from Mars_",
	}
	sdk, err := db2sdk.TemplateVersionParameter(db)
	req.NoError(err)
	req.Len(sdk.Options, 1)
	req.Equal("foo", sdk.Options[0].Name)
	req.Equal("bar", sdk.Options[0].Description)
	req.Equal("baz", sdk.Options[0].Value)
	req.Equal("David Bowie", sdk.Options[0].Icon)
	req.Equal("The Rise and Fall of Ziggy Stardust and the Spiders from Mars", sdk.DescriptionPlaintext)
}

func TestTemplateVersionParameter_BadOptions(t *testing.T) {
	t.Parallel()
	req := require.New(t)

	db := database.TemplateVersionParameter{
		Options:     json.RawMessage("not really JSON!"),
		Description: "_The Rise and Fall of **Ziggy Stardust** and the Spiders from Mars_",
	}
	_, err := db2sdk.TemplateVersionParameter(db)
	req.Error(err)
}

func TestTemplateVersionParameter_BadDescription(t *testing.T) {
	t.Parallel()
	req := require.New(t)
	desc := make([]byte, 300)
	_, err := rand.Read(desc)
	req.NoError(err)

	db := database.TemplateVersionParameter{
		Options:     json.RawMessage("[]"),
		Description: string(desc),
	}
	sdk, err := db2sdk.TemplateVersionParameter(db)
	// Although the markdown parser can return an error, the way we use it should not, even
	// if we feed it garbage data.
	req.NoError(err)
	req.NotEmpty(sdk.DescriptionPlaintext, "broke the markdown parser with %v", desc)
}

func TestChatDebugRunSummary(t *testing.T) {
	t.Parallel()

	startedAt := time.Now().UTC().Round(time.Second)
	finishedAt := startedAt.Add(5 * time.Second)

	run := database.ChatDebugRun{
		ID:         uuid.New(),
		ChatID:     uuid.New(),
		Kind:       "chat_turn",
		Status:     "completed",
		Provider:   sql.NullString{String: "openai", Valid: true},
		Model:      sql.NullString{String: "gpt-4o", Valid: true},
		Summary:    json.RawMessage(`{"step_count":3,"has_error":false}`),
		StartedAt:  startedAt,
		UpdatedAt:  finishedAt,
		FinishedAt: sql.NullTime{Time: finishedAt, Valid: true},
	}

	sdk := db2sdk.ChatDebugRunSummary(run)

	require.Equal(t, run.ID, sdk.ID)
	require.Equal(t, run.ChatID, sdk.ChatID)
	require.Equal(t, codersdk.ChatDebugRunKindChatTurn, sdk.Kind)
	require.Equal(t, codersdk.ChatDebugStatusCompleted, sdk.Status)
	require.NotNil(t, sdk.Provider)
	require.Equal(t, "openai", *sdk.Provider)
	require.NotNil(t, sdk.Model)
	require.Equal(t, "gpt-4o", *sdk.Model)
	require.Equal(t, map[string]any{"step_count": float64(3), "has_error": false}, sdk.Summary)
	require.Equal(t, startedAt, sdk.StartedAt)
	require.Equal(t, finishedAt, sdk.UpdatedAt)
	require.NotNil(t, sdk.FinishedAt)
	require.Equal(t, finishedAt, *sdk.FinishedAt)
}

func TestChatDebugRunSummary_NullableFieldsNil(t *testing.T) {
	t.Parallel()

	run := database.ChatDebugRun{
		ID:        uuid.New(),
		ChatID:    uuid.New(),
		Kind:      "title_generation",
		Status:    "in_progress",
		Summary:   json.RawMessage(`{}`),
		StartedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	sdk := db2sdk.ChatDebugRunSummary(run)

	require.Nil(t, sdk.Provider, "NULL Provider should map to nil")
	require.Nil(t, sdk.Model, "NULL Model should map to nil")
	require.Nil(t, sdk.FinishedAt, "NULL FinishedAt should map to nil")
}

func TestChatDebugStep(t *testing.T) {
	t.Parallel()

	startedAt := time.Now().UTC().Round(time.Second)
	finishedAt := startedAt.Add(2 * time.Second)
	attempts := json.RawMessage(`[
		{
			"attempt_number": 1,
			"status": "completed",
			"raw_request": {"url": "https://example.com"},
			"raw_response": {"status": "200"},
			"duration_ms": 123,
			"started_at": "2026-03-01T10:00:01Z",
			"finished_at": "2026-03-01T10:00:02Z"
		}
	]`)
	step := database.ChatDebugStep{
		ID:                uuid.New(),
		RunID:             uuid.New(),
		ChatID:            uuid.New(),
		StepNumber:        1,
		Operation:         "stream",
		Status:            "completed",
		NormalizedRequest: json.RawMessage(`{"messages":[]}`),
		Attempts:          attempts,
		Metadata:          json.RawMessage(`{"provider":"openai"}`),
		StartedAt:         startedAt,
		UpdatedAt:         finishedAt,
		FinishedAt:        sql.NullTime{Time: finishedAt, Valid: true},
	}

	sdk := db2sdk.ChatDebugStep(step)

	// Verify all scalar fields are mapped correctly.
	require.Equal(t, step.ID, sdk.ID)
	require.Equal(t, step.RunID, sdk.RunID)
	require.Equal(t, step.ChatID, sdk.ChatID)
	require.Equal(t, step.StepNumber, sdk.StepNumber)
	require.Equal(t, codersdk.ChatDebugStepOperationStream, sdk.Operation)
	require.Equal(t, codersdk.ChatDebugStatusCompleted, sdk.Status)
	require.Equal(t, startedAt, sdk.StartedAt)
	require.Equal(t, finishedAt, sdk.UpdatedAt)
	require.Equal(t, &finishedAt, sdk.FinishedAt)

	// Verify JSON object fields are deserialized.
	require.NotNil(t, sdk.NormalizedRequest)
	require.Equal(t, map[string]any{"messages": []any{}}, sdk.NormalizedRequest)
	require.NotNil(t, sdk.Metadata)
	require.Equal(t, map[string]any{"provider": "openai"}, sdk.Metadata)

	// Verify nullable fields are nil when the DB row has NULL values.
	require.Nil(t, sdk.HistoryTipMessageID, "NULL HistoryTipMessageID should map to nil")
	require.Nil(t, sdk.AssistantMessageID, "NULL AssistantMessageID should map to nil")
	require.Nil(t, sdk.NormalizedResponse, "NULL NormalizedResponse should map to nil")
	require.Nil(t, sdk.Usage, "NULL Usage should map to nil")
	require.Nil(t, sdk.Error, "NULL Error should map to nil")

	// Verify attempts are preserved with all fields.
	require.Len(t, sdk.Attempts, 1)
	require.Equal(t, float64(1), sdk.Attempts[0]["attempt_number"])
	require.Equal(t, "completed", sdk.Attempts[0]["status"])
	require.Equal(t, float64(123), sdk.Attempts[0]["duration_ms"])
	require.Equal(t, map[string]any{"url": "https://example.com"}, sdk.Attempts[0]["raw_request"])
	require.Equal(t, map[string]any{"status": "200"}, sdk.Attempts[0]["raw_response"])
}

func TestChatDebugStep_NullableFieldsPopulated(t *testing.T) {
	t.Parallel()

	tipID := int64(42)
	asstID := int64(99)
	step := database.ChatDebugStep{
		ID:                  uuid.New(),
		RunID:               uuid.New(),
		ChatID:              uuid.New(),
		StepNumber:          2,
		Operation:           "generate",
		Status:              "completed",
		HistoryTipMessageID: sql.NullInt64{Int64: tipID, Valid: true},
		AssistantMessageID:  sql.NullInt64{Int64: asstID, Valid: true},
		NormalizedRequest:   json.RawMessage(`{}`),
		NormalizedResponse:  pqtype.NullRawMessage{RawMessage: json.RawMessage(`{"text":"hi"}`), Valid: true},
		Usage:               pqtype.NullRawMessage{RawMessage: json.RawMessage(`{"tokens":10}`), Valid: true},
		Error:               pqtype.NullRawMessage{RawMessage: json.RawMessage(`{"code":"rate_limit"}`), Valid: true},
		Attempts:            json.RawMessage(`[]`),
		Metadata:            json.RawMessage(`{}`),
		StartedAt:           time.Now().UTC(),
		UpdatedAt:           time.Now().UTC(),
	}

	sdk := db2sdk.ChatDebugStep(step)

	require.NotNil(t, sdk.HistoryTipMessageID)
	require.Equal(t, tipID, *sdk.HistoryTipMessageID)
	require.NotNil(t, sdk.AssistantMessageID)
	require.Equal(t, asstID, *sdk.AssistantMessageID)
	require.NotNil(t, sdk.NormalizedResponse)
	require.Equal(t, map[string]any{"text": "hi"}, sdk.NormalizedResponse)
	require.NotNil(t, sdk.Usage)
	require.Equal(t, map[string]any{"tokens": float64(10)}, sdk.Usage)
	require.NotNil(t, sdk.Error)
	require.Equal(t, map[string]any{"code": "rate_limit"}, sdk.Error)
}

func TestChatDebugStep_PreservesMalformedAttempts(t *testing.T) {
	t.Parallel()

	step := database.ChatDebugStep{
		ID:                uuid.New(),
		RunID:             uuid.New(),
		ChatID:            uuid.New(),
		StepNumber:        1,
		Operation:         "stream",
		Status:            "completed",
		NormalizedRequest: json.RawMessage(`{"messages":[]}`),
		Attempts:          json.RawMessage(`{"bad":true}`),
		Metadata:          json.RawMessage(`{"provider":"openai"}`),
		StartedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	}

	sdk := db2sdk.ChatDebugStep(step)
	require.Len(t, sdk.Attempts, 1)
	require.Equal(t, "malformed attempts payload", sdk.Attempts[0]["error"])
	require.NotEmpty(t, sdk.Attempts[0]["parse_error"], "parse_error should contain the unmarshal error")
	require.Equal(t, `{"bad":true}`, sdk.Attempts[0]["raw"])
}

func TestChatDebugRunSummary_PreservesMalformedSummary(t *testing.T) {
	t.Parallel()

	run := database.ChatDebugRun{
		ID:        uuid.New(),
		ChatID:    uuid.New(),
		Kind:      "chat_turn",
		Status:    "completed",
		Summary:   json.RawMessage(`not-an-object`),
		StartedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	sdk := db2sdk.ChatDebugRunSummary(run)
	require.Equal(t, "malformed debug payload", sdk.Summary["error"])
	require.NotEmpty(t, sdk.Summary["parse_error"], "parse_error should contain the unmarshal error")
	require.Equal(t, "not-an-object", sdk.Summary["raw"])
}

func TestChatDebugStep_PreservesMalformedRequest(t *testing.T) {
	t.Parallel()

	step := database.ChatDebugStep{
		ID:                uuid.New(),
		RunID:             uuid.New(),
		ChatID:            uuid.New(),
		StepNumber:        1,
		Operation:         "stream",
		Status:            "completed",
		NormalizedRequest: json.RawMessage(`[1,2,3]`),
		Attempts:          json.RawMessage(`[]`),
		Metadata:          json.RawMessage(`"just-a-string"`),
		StartedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	}

	sdk := db2sdk.ChatDebugStep(step)
	require.Equal(t, "malformed debug payload", sdk.NormalizedRequest["error"])
	require.NotEmpty(t, sdk.NormalizedRequest["parse_error"], "parse_error should contain the unmarshal error")
	require.Equal(t, "[1,2,3]", sdk.NormalizedRequest["raw"])
	require.Equal(t, "malformed debug payload", sdk.Metadata["error"])
	require.NotEmpty(t, sdk.Metadata["parse_error"], "parse_error should contain the unmarshal error")
	require.Equal(t, `"just-a-string"`, sdk.Metadata["raw"])
}

func TestChatDebugRunSummary_JSONNullYieldsEmptyMap(t *testing.T) {
	t.Parallel()

	run := database.ChatDebugRun{
		ID:        uuid.New(),
		ChatID:    uuid.New(),
		Kind:      "chat_turn",
		Status:    "completed",
		Summary:   json.RawMessage(`null`),
		StartedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	sdk := db2sdk.ChatDebugRunSummary(run)
	require.NotNil(t, sdk.Summary, "JSON literal null must produce non-nil map")
	require.Empty(t, sdk.Summary, "JSON literal null must produce empty map")
}

func TestChatDebugStep_JSONNullYieldsEmptyStructures(t *testing.T) {
	t.Parallel()

	step := database.ChatDebugStep{
		ID:                uuid.New(),
		RunID:             uuid.New(),
		ChatID:            uuid.New(),
		StepNumber:        1,
		Operation:         "stream",
		Status:            "completed",
		NormalizedRequest: json.RawMessage(`null`),
		Attempts:          json.RawMessage(`null`),
		Metadata:          json.RawMessage(`null`),
		StartedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	}

	sdk := db2sdk.ChatDebugStep(step)
	require.NotNil(t, sdk.NormalizedRequest, "JSON literal null must produce non-nil map")
	require.Empty(t, sdk.NormalizedRequest, "JSON literal null must produce empty map")
	require.NotNil(t, sdk.Attempts, "JSON literal null must produce non-nil slice")
	require.Empty(t, sdk.Attempts, "JSON literal null must produce empty slice")
	require.NotNil(t, sdk.Metadata, "JSON literal null must produce non-nil map")
	require.Empty(t, sdk.Metadata, "JSON literal null must produce empty map")
}

func TestAIBridgeInterception(t *testing.T) {
	t.Parallel()

	now := dbtime.Now()
	interceptionID := uuid.New()
	initiatorID := uuid.New()

	cases := []struct {
		name         string
		interception database.AIBridgeInterception
		initiator    database.VisibleUser
		tokenUsages  []database.AIBridgeTokenUsage
		userPrompts  []database.AIBridgeUserPrompt
		toolUsages   []database.AIBridgeToolUsage
		expected     codersdk.AIBridgeInterception
	}{
		{
			name: "all_optional_values_set",
			interception: database.AIBridgeInterception{
				ID:          interceptionID,
				InitiatorID: initiatorID,
				Provider:    "anthropic",
				Model:       "claude-3-opus",
				StartedAt:   now,
				Metadata: pqtype.NullRawMessage{
					RawMessage: json.RawMessage(`{"key":"value"}`),
					Valid:      true,
				},
				EndedAt: sql.NullTime{
					Time:  now.Add(time.Minute),
					Valid: true,
				},
				APIKeyID: sql.NullString{
					String: "api-key-123",
					Valid:  true,
				},
				Client: sql.NullString{
					String: "claude-code/1.0.0",
					Valid:  true,
				},
			},
			initiator: database.VisibleUser{
				ID:        initiatorID,
				Username:  "testuser",
				Name:      "Test User",
				AvatarURL: "https://example.com/avatar.png",
			},
			tokenUsages: []database.AIBridgeTokenUsage{
				{
					ID:                    uuid.New(),
					InterceptionID:        interceptionID,
					ProviderResponseID:    "resp-123",
					InputTokens:           100,
					OutputTokens:          200,
					CacheReadInputTokens:  50,
					CacheWriteInputTokens: 10,
					Metadata: pqtype.NullRawMessage{
						RawMessage: json.RawMessage(`{"cache":"hit"}`),
						Valid:      true,
					},
					CreatedAt: now.Add(10 * time.Second),
				},
			},
			userPrompts: []database.AIBridgeUserPrompt{
				{
					ID:                 uuid.New(),
					InterceptionID:     interceptionID,
					ProviderResponseID: "resp-123",
					Prompt:             "Hello, world!",
					Metadata: pqtype.NullRawMessage{
						RawMessage: json.RawMessage(`{"role":"user"}`),
						Valid:      true,
					},
					CreatedAt: now.Add(5 * time.Second),
				},
			},
			toolUsages: []database.AIBridgeToolUsage{
				{
					ID:                 uuid.New(),
					InterceptionID:     interceptionID,
					ProviderResponseID: "resp-123",
					ServerUrl: sql.NullString{
						String: "https://mcp.example.com",
						Valid:  true,
					},
					Tool:     "read_file",
					Input:    `{"path":"/tmp/test.txt"}`,
					Injected: true,
					InvocationError: sql.NullString{
						String: "file not found",
						Valid:  true,
					},
					Metadata: pqtype.NullRawMessage{
						RawMessage: json.RawMessage(`{"duration_ms":50}`),
						Valid:      true,
					},
					CreatedAt: now.Add(15 * time.Second),
				},
			},
			expected: codersdk.AIBridgeInterception{
				ID: interceptionID,
				Initiator: codersdk.MinimalUser{
					ID:        initiatorID,
					Username:  "testuser",
					Name:      "Test User",
					AvatarURL: "https://example.com/avatar.png",
				},
				Provider:  "anthropic",
				Model:     "claude-3-opus",
				Metadata:  map[string]any{"key": "value"},
				StartedAt: now,
			},
		},
		{
			name: "no_optional_values_set",
			interception: database.AIBridgeInterception{
				ID:          interceptionID,
				InitiatorID: initiatorID,
				Provider:    "openai",
				Model:       "gpt-4",
				StartedAt:   now,
				Metadata:    pqtype.NullRawMessage{Valid: false},
				EndedAt:     sql.NullTime{Valid: false},
				APIKeyID:    sql.NullString{Valid: false},
				Client:      sql.NullString{Valid: false},
			},
			initiator: database.VisibleUser{
				ID:        initiatorID,
				Username:  "minimaluser",
				Name:      "",
				AvatarURL: "",
			},
			tokenUsages: nil,
			userPrompts: nil,
			toolUsages:  nil,
			expected: codersdk.AIBridgeInterception{
				ID: interceptionID,
				Initiator: codersdk.MinimalUser{
					ID:        initiatorID,
					Username:  "minimaluser",
					Name:      "",
					AvatarURL: "",
				},
				Provider:  "openai",
				Model:     "gpt-4",
				Metadata:  nil,
				StartedAt: now,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := db2sdk.AIBridgeInterception(
				tc.interception,
				tc.initiator,
				tc.tokenUsages,
				tc.userPrompts,
				tc.toolUsages,
			)

			// Check basic fields.
			require.Equal(t, tc.expected.ID, result.ID)
			require.Equal(t, tc.expected.Initiator, result.Initiator)
			require.Equal(t, tc.expected.Provider, result.Provider)
			require.Equal(t, tc.expected.Model, result.Model)
			require.Equal(t, tc.expected.StartedAt.UTC(), result.StartedAt.UTC())
			require.Equal(t, tc.expected.Metadata, result.Metadata)

			// Check optional pointer fields.
			if tc.interception.APIKeyID.Valid {
				require.NotNil(t, result.APIKeyID)
				require.Equal(t, tc.interception.APIKeyID.String, *result.APIKeyID)
			} else {
				require.Nil(t, result.APIKeyID)
			}

			if tc.interception.EndedAt.Valid {
				require.NotNil(t, result.EndedAt)
				require.Equal(t, tc.interception.EndedAt.Time.UTC(), result.EndedAt.UTC())
			} else {
				require.Nil(t, result.EndedAt)
			}

			if tc.interception.Client.Valid {
				require.NotNil(t, result.Client)
				require.Equal(t, tc.interception.Client.String, *result.Client)
			} else {
				require.Nil(t, result.Client)
			}

			// Check slices.
			require.Len(t, result.TokenUsages, len(tc.tokenUsages))
			require.Len(t, result.UserPrompts, len(tc.userPrompts))
			require.Len(t, result.ToolUsages, len(tc.toolUsages))

			// Verify token usages are converted correctly.
			for i, tu := range tc.tokenUsages {
				require.Equal(t, tu.ID, result.TokenUsages[i].ID)
				require.Equal(t, tu.InterceptionID, result.TokenUsages[i].InterceptionID)
				require.Equal(t, tu.ProviderResponseID, result.TokenUsages[i].ProviderResponseID)
				require.Equal(t, tu.InputTokens, result.TokenUsages[i].InputTokens)
				require.Equal(t, tu.OutputTokens, result.TokenUsages[i].OutputTokens)
				require.Equal(t, tu.CacheReadInputTokens, result.TokenUsages[i].CacheReadInputTokens)
				require.Equal(t, tu.CacheWriteInputTokens, result.TokenUsages[i].CacheWriteInputTokens)
			}

			// Verify user prompts are converted correctly.
			for i, up := range tc.userPrompts {
				require.Equal(t, up.ID, result.UserPrompts[i].ID)
				require.Equal(t, up.InterceptionID, result.UserPrompts[i].InterceptionID)
				require.Equal(t, up.ProviderResponseID, result.UserPrompts[i].ProviderResponseID)
				require.Equal(t, up.Prompt, result.UserPrompts[i].Prompt)
			}

			// Verify tool usages are converted correctly.
			for i, toolUsage := range tc.toolUsages {
				require.Equal(t, toolUsage.ID, result.ToolUsages[i].ID)
				require.Equal(t, toolUsage.InterceptionID, result.ToolUsages[i].InterceptionID)
				require.Equal(t, toolUsage.ProviderResponseID, result.ToolUsages[i].ProviderResponseID)
				require.Equal(t, toolUsage.ServerUrl.String, result.ToolUsages[i].ServerURL)
				require.Equal(t, toolUsage.Tool, result.ToolUsages[i].Tool)
				require.Equal(t, toolUsage.Input, result.ToolUsages[i].Input)
				require.Equal(t, toolUsage.Injected, result.ToolUsages[i].Injected)
				require.Equal(t, toolUsage.InvocationError.String, result.ToolUsages[i].InvocationError)
			}
		})
	}
}

func TestChatMessage_PreservesProviderExecutedOnToolResults(t *testing.T) {
	t.Parallel()

	toolCallID := uuid.New().String()
	toolName := "web_search"

	// Build assistant content blocks with ProviderExecuted set.
	toolCall := fantasy.ToolCallContent{
		ToolCallID:       toolCallID,
		ToolName:         toolName,
		Input:            `{"query":"test"}`,
		ProviderExecuted: true,
	}
	toolResult := fantasy.ToolResultContent{
		ToolCallID:       toolCallID,
		ToolName:         toolName,
		Result:           fantasy.ToolResultOutputContentText{Text: `{"results":[]}`},
		ProviderExecuted: true,
	}

	tcJSON, err := json.Marshal(toolCall)
	require.NoError(t, err)
	trJSON, err := json.Marshal(toolResult)
	require.NoError(t, err)

	rawContent := json.RawMessage("[" + string(tcJSON) + "," + string(trJSON) + "]")

	dbMsg := database.ChatMessage{
		ID:     1,
		ChatID: uuid.New(),
		Role:   database.ChatMessageRoleAssistant,
		Content: pqtype.NullRawMessage{
			RawMessage: rawContent,
			Valid:      true,
		},
		CreatedAt: time.Now(),
	}

	result := db2sdk.ChatMessage(dbMsg)

	require.Len(t, result.Content, 2)

	// First part: tool call.
	require.Equal(t, codersdk.ChatMessagePartTypeToolCall, result.Content[0].Type)
	require.Equal(t, toolCallID, result.Content[0].ToolCallID)
	require.Equal(t, toolName, result.Content[0].ToolName)
	require.True(t, result.Content[0].ProviderExecuted, "tool call should preserve ProviderExecuted")

	// Second part: tool result.
	require.Equal(t, codersdk.ChatMessagePartTypeToolResult, result.Content[1].Type)
	require.Equal(t, toolCallID, result.Content[1].ToolCallID)
	require.Equal(t, toolName, result.Content[1].ToolName)
	require.True(t, result.Content[1].ProviderExecuted, "tool result should preserve ProviderExecuted")
}

func TestChatQueuedMessage_ParsesUserContentParts(t *testing.T) {
	t.Parallel()

	// Queued messages are always written via MarshalParts (SDK format).
	rawContent, err := json.Marshal([]codersdk.ChatMessagePart{
		codersdk.ChatMessageText("queued text"),
	})
	require.NoError(t, err)

	queued := db2sdk.ChatQueuedMessage(database.ChatQueuedMessage{
		ID:        1,
		ChatID:    uuid.New(),
		Content:   rawContent,
		CreatedAt: time.Now(),
	})

	require.Len(t, queued.Content, 1)
	require.Equal(t, codersdk.ChatMessagePartTypeText, queued.Content[0].Type)
	require.Equal(t, "queued text", queued.Content[0].Text)
}

func TestChat_AllFieldsPopulated(t *testing.T) {
	t.Parallel()

	// Every field of database.Chat is set to a non-zero value so
	// that the reflection check below catches any field that
	// db2sdk.Chat forgets to populate. When someone adds a new
	// field to codersdk.Chat, this test will fail until the
	// converter is updated.
	now := dbtime.Now()
	input := database.Chat{
		ID:                uuid.New(),
		OwnerID:           uuid.New(),
		OrganizationID:    uuid.New(),
		WorkspaceID:       uuid.NullUUID{UUID: uuid.New(), Valid: true},
		BuildID:           uuid.NullUUID{UUID: uuid.New(), Valid: true},
		AgentID:           uuid.NullUUID{UUID: uuid.New(), Valid: true},
		ParentChatID:      uuid.NullUUID{UUID: uuid.New(), Valid: true},
		RootChatID:        uuid.NullUUID{UUID: uuid.New(), Valid: true},
		LastModelConfigID: uuid.New(),
		Title:             "all-fields-test",
		Status:            database.ChatStatusRunning,
		ClientType:        database.ChatClientTypeUi,
		LastError:         sql.NullString{String: "boom", Valid: true},
		CreatedAt:         now,
		UpdatedAt:         now,
		Archived:          true,
		PinOrder:          1,
		PlanMode:          database.NullChatPlanMode{ChatPlanMode: database.ChatPlanModePlan, Valid: true},
		MCPServerIDs:      []uuid.UUID{uuid.New()},
		Labels:            database.StringMap{"env": "prod"},
		LastInjectedContext: pqtype.NullRawMessage{
			// Use a context-file part to verify internal
			// fields are not present (they are stripped at
			// write time by chatd, not at read time).
			RawMessage: json.RawMessage(`[{"type":"context-file","context_file_path":"/AGENTS.md"}]`),
			Valid:      true,
		},
		DynamicTools: pqtype.NullRawMessage{
			RawMessage: json.RawMessage(`[{"name":"tool1","description":"test tool","inputSchema":{"type":"object"}}]`),
			Valid:      true,
		},
	}
	// Only ChatID is needed here. This test checks that
	// Chat.DiffStatus is non-nil, not that every DiffStatus
	// field is populated — that would be a separate test for
	// the ChatDiffStatus converter.
	diffStatus := &database.ChatDiffStatus{
		ChatID: input.ID,
	}

	fileRows := []database.GetChatFileMetadataByChatIDRow{
		{
			ID:             uuid.New(),
			OwnerID:        input.OwnerID,
			OrganizationID: uuid.New(),
			Name:           "test.png",
			Mimetype:       "image/png",
			CreatedAt:      now,
		},
	}

	got := db2sdk.Chat(input, diffStatus, fileRows)

	v := reflect.ValueOf(got)
	typ := v.Type()
	// HasUnread is populated by ChatRows (which joins the
	// read-cursor query), not by Chat. Warnings is a transient
	// field populated by handlers, not the converter. Both are
	// expected to remain zero here.
	skip := map[string]bool{"HasUnread": true, "Warnings": true}
	for i := range typ.NumField() {
		field := typ.Field(i)
		if skip[field.Name] {
			continue
		}
		require.False(t, v.Field(i).IsZero(),
			"codersdk.Chat field %q is zero-valued — db2sdk.Chat may not be populating it",
			field.Name,
		)
	}
}

func TestChat_FileMetadataConversion(t *testing.T) {
	t.Parallel()

	ownerID := uuid.New()
	orgID := uuid.New()
	fileID := uuid.New()
	now := dbtime.Now()

	chat := database.Chat{
		ID:                uuid.New(),
		OwnerID:           ownerID,
		LastModelConfigID: uuid.New(),
		Title:             "file metadata test",
		Status:            database.ChatStatusWaiting,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	rows := []database.GetChatFileMetadataByChatIDRow{
		{
			ID:             fileID,
			OwnerID:        ownerID,
			OrganizationID: orgID,
			Name:           "screenshot.png",
			Mimetype:       "image/png",
			CreatedAt:      now,
		},
	}

	result := db2sdk.Chat(chat, nil, rows)

	require.Len(t, result.Files, 1)
	f := result.Files[0]
	require.Equal(t, fileID, f.ID)
	require.Equal(t, ownerID, f.OwnerID, "OwnerID must be mapped from DB row")
	require.Equal(t, orgID, f.OrganizationID, "OrganizationID must be mapped from DB row")
	require.Equal(t, "screenshot.png", f.Name)
	require.Equal(t, "image/png", f.MimeType)
	require.Equal(t, now, f.CreatedAt)

	// Verify JSON serialization uses snake_case for mime_type.
	data, err := json.Marshal(f)
	require.NoError(t, err)
	require.Contains(t, string(data), `"mime_type"`)
	require.NotContains(t, string(data), `"mimetype"`)
}

func TestChat_NilFilesOmitted(t *testing.T) {
	t.Parallel()

	chat := database.Chat{
		ID:                uuid.New(),
		OwnerID:           uuid.New(),
		LastModelConfigID: uuid.New(),
		Title:             "no files",
		Status:            database.ChatStatusWaiting,
		CreatedAt:         dbtime.Now(),
		UpdatedAt:         dbtime.Now(),
	}

	result := db2sdk.Chat(chat, nil, nil)
	require.Empty(t, result.Files)
}

func TestChat_MultipleFiles(t *testing.T) {
	t.Parallel()

	now := dbtime.Now()
	file1 := uuid.New()
	file2 := uuid.New()

	chat := database.Chat{
		ID:                uuid.New(),
		OwnerID:           uuid.New(),
		LastModelConfigID: uuid.New(),
		Title:             "multi file test",
		Status:            database.ChatStatusWaiting,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	rows := []database.GetChatFileMetadataByChatIDRow{
		{
			ID:             file1,
			OwnerID:        chat.OwnerID,
			OrganizationID: uuid.New(),
			Name:           "a.png",
			Mimetype:       "image/png",
			CreatedAt:      now,
		},
		{
			ID:             file2,
			OwnerID:        chat.OwnerID,
			OrganizationID: uuid.New(),
			Name:           "b.txt",
			Mimetype:       "text/plain",
			CreatedAt:      now,
		},
	}

	result := db2sdk.Chat(chat, nil, rows)
	require.Len(t, result.Files, 2)
	require.Equal(t, "a.png", result.Files[0].Name)
	require.Equal(t, "b.txt", result.Files[1].Name)
}

func TestChatQueuedMessage_MalformedContent(t *testing.T) {
	t.Parallel()

	queued := db2sdk.ChatQueuedMessage(database.ChatQueuedMessage{
		ID:        1,
		ChatID:    uuid.New(),
		Content:   json.RawMessage(`{"unexpected":"shape"}`),
		CreatedAt: time.Now(),
	})

	require.Empty(t, queued.Content)
}
