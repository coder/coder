package audit_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/aiagentidentity"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/entity"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/tracing"
)

func TestAIAgentAuditAttribution(t *testing.T) {
	t.Parallel()

	agentUserID := uuid.New()
	ownerUserID := uuid.New()
	actor := aiagentidentity.AIAgentActor{
		AgentUserID: agentUserID,
		OwnerUserID: ownerUserID,
		OriginType:  entity.CreationSiteTypeChat,
		OriginID:    uuid.New(),
	}

	r := httptest.NewRequest(http.MethodPost, "/api/v2/test", nil)
	ctx := httpmw.WithRequestID(r.Context(), uuid.New())
	r = r.WithContext(aiagentidentity.WithActor(ctx, actor))
	sw := &tracing.StatusWriter{ResponseWriter: httptest.NewRecorder()}
	auditor := audit.NewMock()
	req, commit := audit.InitRequest[database.User](sw, &audit.RequestParams{
		Audit:   auditor,
		Log:     slogtest.Make(t, nil),
		Request: r,
		Action:  database.AuditActionCreate,
	})
	req.New = database.User{ID: uuid.New(), Username: "created-user"}
	sw.WriteHeader(http.StatusCreated)
	commit()

	logs := auditor.AuditLogs()
	require.Len(t, logs, 1)
	require.Equal(t, agentUserID, logs[0].UserID)
	require.True(t, logs[0].OnBehalfOfUserID.Valid)
	require.Equal(t, ownerUserID, logs[0].OnBehalfOfUserID.UUID)
}
