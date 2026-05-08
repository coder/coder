package dlppolicy

import (
	"context"
	"database/sql"
	"net"
	"net/http"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/connectionlog"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
)

// DenialParams describes a single DLP-denied workspace traffic attempt.
// Build one of these at the gate site and hand it to LogDenial.
type DenialParams struct {
	OrganizationID   uuid.UUID
	WorkspaceOwnerID uuid.UUID
	WorkspaceID      uuid.UUID
	WorkspaceName    string
	AgentName        string
	// Type is the connection_logs.type value matching the gate that fired.
	// Path A → ConnectionTypeSsh, PTY → ConnectionTypeReconnectingPty, app
	// proxy → ConnectionTypeWorkspaceApp, port view → ConnectionTypePortForwarding.
	Type database.ConnectionType
	// Reason is the human-readable disconnect_reason text. Should name the
	// policy and the gated field, e.g. `DLP policy "strict" denied ssh_access`.
	Reason     string
	UserID     uuid.NullUUID
	IP         pqtype.Inet
	UserAgent  sql.NullString
	SlugOrPort sql.NullString
}

// LogDenial writes a connection_logs row recording that a DLP gate denied
// a request. Errors are logged but not returned: auditing is best-effort
// and must never block the deny response.
//
// Denied attempts are encoded as connection_status=disconnected with
// code=403 and a free-form disconnect_reason. There is no dedicated
// "denied" status enum value; this convention keeps the schema unchanged
// while still being unambiguous to grep over.
func LogDenial(ctx context.Context, logger slog.Logger, connLogger connectionlog.ConnectionLogger, p DenialParams) {
	now := dbtime.Now()
	err := connLogger.Upsert(ctx, database.UpsertConnectionLogParams{
		ID:               uuid.New(),
		Time:             now,
		OrganizationID:   p.OrganizationID,
		WorkspaceOwnerID: p.WorkspaceOwnerID,
		WorkspaceID:      p.WorkspaceID,
		WorkspaceName:    p.WorkspaceName,
		AgentName:        p.AgentName,
		Type:             p.Type,
		ConnectionStatus: database.ConnectionStatusDisconnected,
		Code:             sql.NullInt32{Int32: http.StatusForbidden, Valid: true},
		DisconnectReason: sql.NullString{String: p.Reason, Valid: p.Reason != ""},
		UserID:           p.UserID,
		IP:               p.IP,
		UserAgent:        p.UserAgent,
		SlugOrPort:       p.SlugOrPort,
		ConnectionID:     uuid.NullUUID{},
	})
	if err != nil {
		logger.Warn(ctx, "failed to write dlp denial connection log",
			slog.F("workspace_id", p.WorkspaceID),
			slog.F("agent_name", p.AgentName),
			slog.F("type", p.Type),
			slog.Error(err),
		)
	}
}

// IPFromRequest extracts the remote address as a database-friendly IP value
// for connection_logs. Returns an invalid pqtype.Inet when parsing fails.
func IPFromRequest(r *http.Request) pqtype.Inet {
	if r == nil {
		return pqtype.Inet{}
	}
	return database.ParseIP(remoteAddrHost(r))
}

// remoteAddrHost trims the optional :port suffix from r.RemoteAddr.
func remoteAddrHost(r *http.Request) string {
	addr := r.RemoteAddr
	// IPv6 has the form "[::1]:1234"; net.SplitHostPort handles both.
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return host
}
