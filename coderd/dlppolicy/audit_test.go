package dlppolicy_test

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/connectionlog"
	"github.com/coder/coder/v2/coderd/database"
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
