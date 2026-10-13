package coderd_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/testutil"
)

// TestChatAutomationRunAudit checks that a webhook delivery with the right
// secret leaves one start entry for the automation, with the sender's
// address, and that a wrong secret or an unknown automation leaves none.
func TestChatAutomationRunAudit(t *testing.T) {
	t.Parallel()
	auditor := audit.NewMock()
	env := newChatAutomationTestEnv(t, nil, func(o *coderdtest.Options) { o.Auditor = auditor })
	ctx := testutil.Context(t, testutil.WaitLong)
	created, err := env.member.CreateChatAutomation(ctx, env.orgID, env.webhookRequest())
	require.NoError(t, err)
	id := created.Automation.ID
	starts := func() []database.AuditLog {
		var logs []database.AuditLog
		for _, entry := range auditor.AuditLogs() {
			if entry.Action == database.AuditActionStart && entry.ResourceType == database.ResourceTypeChatAutomation {
				logs = append(logs, entry)
			}
		}
		return logs
	}

	status, _ := postChatAutomationEvent(t, env.member, id, created.WebhookSecret+"x", []byte(`{}`))
	require.Equal(t, http.StatusUnauthorized, status)
	status, _ = postChatAutomationEvent(t, env.member, uuid.New(), created.WebhookSecret, []byte(`{}`))
	require.Equal(t, http.StatusUnauthorized, status)
	require.Empty(t, starts(), "a request without the right secret is not a run")

	status, body := postChatAutomationEvent(t, env.member, id, created.WebhookSecret, []byte(`{"ok":true}`))
	require.Equal(t, http.StatusAccepted, status, string(body))
	// An authenticated refusal is a run too.
	status, _ = postChatAutomationEvent(t, env.member, id, created.WebhookSecret, []byte(`not json`))
	require.Equal(t, http.StatusBadRequest, status)

	logs := starts()
	require.Len(t, logs, 2)
	require.Equal(t, env.memberID, logs[0].UserID, "the owner is the actor")
	require.Equal(t, id, logs[0].ResourceID)
	require.EqualValues(t, http.StatusAccepted, logs[0].StatusCode)
	require.True(t, logs[0].Ip.Valid, "the sender's address is recorded")
	require.NotEmpty(t, logs[0].UserAgent.String)
	var fields map[string]string
	require.NoError(t, json.Unmarshal(logs[0].AdditionalFields, &fields))
	require.Equal(t, "webhook", fields["trigger"])
	require.Equal(t, "accepted", fields["outcome"])
	require.Equal(t, env.memberChat.ID.String(), fields["chat_id"])
	require.NotEmpty(t, fields["input_id"])

	require.EqualValues(t, http.StatusBadRequest, logs[1].StatusCode)
	require.NoError(t, json.Unmarshal(logs[1].AdditionalFields, &fields))
	require.Equal(t, "refused", fields["outcome"])
	require.Equal(t, "invalid_body", fields["reason"])
}
