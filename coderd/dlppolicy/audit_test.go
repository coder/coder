package dlppolicy_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/connectionlog"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/dlppolicy"
	"github.com/coder/coder/v2/testutil"
)

func TestLogDenial(t *testing.T) {
	t.Parallel()

	logger := testutil.Logger(t)
	fake := connectionlog.NewFake()

	workspaceID := uuid.New()
	userID := uuid.New()

	dlppolicy.LogDenial(context.Background(), logger, fake, dlppolicy.DenialParams{
		OrganizationID:   uuid.New(),
		WorkspaceOwnerID: uuid.New(),
		WorkspaceID:      workspaceID,
		WorkspaceName:    "ws",
		AgentName:        "main",
		Type:             database.ConnectionTypeSsh,
		Reason:           `DLP policy "strict" denied ssh_access`,
		UserID:           uuid.NullUUID{UUID: userID, Valid: true},
		UserAgent:        sql.NullString{String: "test/1.0", Valid: true},
	})

	logs := fake.ConnectionLogs()
	require.Len(t, logs, 1)
	got := logs[0]
	require.Equal(t, workspaceID, got.WorkspaceID)
	require.Equal(t, "ws", got.WorkspaceName)
	require.Equal(t, "main", got.AgentName)
	require.Equal(t, database.ConnectionTypeSsh, got.Type)
	require.Equal(t, database.ConnectionStatusDisconnected, got.ConnectionStatus)
	require.True(t, got.Code.Valid)
	require.EqualValues(t, http.StatusForbidden, got.Code.Int32)
	require.True(t, got.DisconnectReason.Valid)
	require.Contains(t, got.DisconnectReason.String, `"strict"`)
	require.Equal(t, userID, got.UserID.UUID)
	require.True(t, got.UserID.Valid)
}

func TestLogDenial_LoggerErrorIsNotPropagated(t *testing.T) {
	t.Parallel()

	logger := testutil.Logger(t)
	// Sanity check: the helper has no return value and does not panic
	// when the logger's Upsert errors out. We assert the deny path is
	// unaffected by audit failure.
	dlppolicy.LogDenial(context.Background(), logger, errLogger{}, dlppolicy.DenialParams{
		WorkspaceID: uuid.New(),
		Type:        database.ConnectionTypeSsh,
	})
}

func TestLogClipboardBlock(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	logger := testutil.Logger(t)
	db, _ := dbtestutil.NewDB(t)

	orgID := uuid.New()
	workspaceID := uuid.New()
	workspaceOwnerID := uuid.New()
	userID := uuid.New()

	dlppolicy.LogClipboardBlock(ctx, logger, db, dlppolicy.ClipboardBlockParams{
		OrganizationID:   orgID,
		WorkspaceOwnerID: workspaceOwnerID,
		WorkspaceID:      workspaceID,
		WorkspaceName:    "dev-env",
		AgentName:        "main",
		Direction:        "client-to-server",
		Drops:            3,
		Bytes:            56,
		UserID:           userID,
		UserAgent:        sql.NullString{String: "test/1.0", Valid: true},
	})

	rows, err := db.GetAuditLogsOffset(ctx, database.GetAuditLogsOffsetParams{
		LimitOpt: 10,
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	got := rows[0]
	require.Equal(t, database.AuditActionBlock, got.AuditLog.Action)
	require.Equal(t, database.ResourceTypeWorkspace, got.AuditLog.ResourceType)
	require.Equal(t, workspaceID, got.AuditLog.ResourceID)
	require.Equal(t, "dev-env", got.AuditLog.ResourceTarget)
	require.Equal(t, int32(http.StatusOK), got.AuditLog.StatusCode)
	require.JSONEq(t, "{}", string(got.AuditLog.Diff))

	var fields struct {
		Operation string `json:"operation"`
		Direction string `json:"direction"`
		Drops     int    `json:"drops"`
		Bytes     int64  `json:"bytes"`
		Agent     string `json:"agent"`
	}
	require.NoError(t, json.Unmarshal(got.AuditLog.AdditionalFields, &fields))
	require.Equal(t, "clipboard", fields.Operation)
	require.Equal(t, "client-to-server", fields.Direction)
	require.Equal(t, 3, fields.Drops)
	require.EqualValues(t, 56, fields.Bytes)
	require.Equal(t, "main", fields.Agent)
}

func TestLogClipboardBlock_TruncatesLongUserAgent(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	logger := testutil.Logger(t)
	db, _ := dbtestutil.NewDB(t)

	long := make([]byte, 512)
	for i := range long {
		long[i] = 'x'
	}

	dlppolicy.LogClipboardBlock(ctx, logger, db, dlppolicy.ClipboardBlockParams{
		OrganizationID: uuid.New(),
		WorkspaceID:    uuid.New(),
		WorkspaceName:  "ws",
		Direction:      "server-to-client",
		Drops:          1,
		UserID:         uuid.New(),
		UserAgent:      sql.NullString{String: string(long), Valid: true},
	})

	rows, err := db.GetAuditLogsOffset(ctx, database.GetAuditLogsOffsetParams{LimitOpt: 10})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.True(t, rows[0].AuditLog.UserAgent.Valid)
	require.LessOrEqual(t, len(rows[0].AuditLog.UserAgent.String), 256)
}

func TestIPFromRequest(t *testing.T) {
	t.Parallel()

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "192.0.2.4:54321"
	ip := dlppolicy.IPFromRequest(r)
	require.True(t, ip.Valid)
	require.Equal(t, "192.0.2.4/32", ip.IPNet.String())

	// IPv6 with port.
	r.RemoteAddr = "[2001:db8::1]:1234"
	ip = dlppolicy.IPFromRequest(r)
	require.True(t, ip.Valid)
}

type errLogger struct{}

func (errLogger) Upsert(_ context.Context, _ database.UpsertConnectionLogParams) error {
	return errFake
}

var errFake = sentinelError("upsert failed")

type sentinelError string

func (e sentinelError) Error() string { return string(e) }
