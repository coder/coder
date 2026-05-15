package dlppolicy

import (
	"context"
	"database/sql"
	"encoding/json"
	"net"
	"net/http"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/connectionlog"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
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

// ClipboardBlockParams describes a single coalesced DLP clipboard
// suppression event over a noVNC desktop session. Build one of these
// at session close (after counting drops per direction) and hand it
// to LogClipboardBlock.
type ClipboardBlockParams struct {
	OrganizationID   uuid.UUID
	WorkspaceOwnerID uuid.UUID
	WorkspaceID      uuid.UUID
	WorkspaceName    string
	AgentName        string
	// Direction is either "client-to-server" or "server-to-client",
	// matching the RFB message direction that was dropped.
	Direction string
	// Drops is the number of clipboard messages dropped in this
	// direction during the session.
	Drops int
	// Bytes is the total payload size dropped in this direction.
	Bytes     int64
	UserID    uuid.UUID
	IP        pqtype.Inet
	UserAgent sql.NullString
}

// clipboardBlockFields is the shape persisted under
// audit_logs.additional_fields for a clipboard block row.
type clipboardBlockFields struct {
	Operation string `json:"operation"`
	Direction string `json:"direction"`
	Drops     int    `json:"drops"`
	Bytes     int64  `json:"bytes"`
	Agent     string `json:"agent"`
}

// userAgentMaxLen is the audit_logs.user_agent column width
// (varchar(256)) declared in migration 000010_audit_logs.up.sql.
const userAgentMaxLen = 256

// LogClipboardBlock writes one audit_logs row recording that a DLP
// policy suppressed clipboard traffic on the noVNC desktop session.
// Errors are logged but not returned: auditing is best-effort and
// must never block the deny path.
//
// The action is recorded as block with status_code=200 (the block
// action itself succeeded). The diff column is empty because this
// is not a CRUD event; the policy-specific detail lives in
// additional_fields.
//
// enforcement is a system decision, and the workspace user does not
// need audit_log:create RBAC.
//
//nolint:gocritic // System-restricted ctx is intentional: the policy
func LogClipboardBlock(ctx context.Context, logger slog.Logger, db database.Store, p ClipboardBlockParams) {
	fields := clipboardBlockFields{
		Operation: "clipboard",
		Direction: p.Direction,
		Drops:     p.Drops,
		Bytes:     p.Bytes,
		Agent:     p.AgentName,
	}
	additional, err := json.Marshal(fields)
	if err != nil {
		logger.Warn(ctx, "failed to marshal dlp clipboard block additional_fields",
			slog.F("workspace_id", p.WorkspaceID),
			slog.Error(err),
		)
		return
	}

	ua := p.UserAgent
	if ua.Valid && len(ua.String) > userAgentMaxLen {
		ua.String = ua.String[:userAgentMaxLen]
	}

	_, err = db.InsertAuditLog(dbauthz.AsSystemRestricted(ctx), database.InsertAuditLogParams{
		ID:               uuid.New(),
		Time:             dbtime.Now(),
		UserID:           p.UserID,
		OrganizationID:   p.OrganizationID,
		Ip:               p.IP,
		UserAgent:        ua,
		ResourceType:     database.ResourceTypeWorkspace,
		ResourceID:       p.WorkspaceID,
		ResourceTarget:   p.WorkspaceName,
		Action:           database.AuditActionBlock,
		Diff:             json.RawMessage("{}"),
		StatusCode:       http.StatusOK,
		AdditionalFields: additional,
		RequestID:        uuid.Nil,
		ResourceIcon:     "",
	})
	if err != nil {
		logger.Warn(ctx, "failed to write dlp clipboard block audit log",
			slog.F("workspace_id", p.WorkspaceID),
			slog.F("agent_name", p.AgentName),
			slog.F("direction", p.Direction),
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
