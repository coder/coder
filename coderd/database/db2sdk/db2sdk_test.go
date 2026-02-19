package db2sdk_test

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"
	"time"

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
					ID:                 uuid.New(),
					InterceptionID:     interceptionID,
					ProviderResponseID: "resp-123",
					InputTokens:        100,
					OutputTokens:       200,
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
