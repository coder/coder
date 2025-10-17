package connectionlog

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
)

type ConnectionLogger interface {
	Upsert(ctx context.Context, clog database.UpsertConnectionLogParams) error
}

type nop struct{}

func NewNop() ConnectionLogger {
	return nop{}
}

func (nop) Upsert(context.Context, database.UpsertConnectionLogParams) error {
	return nil
}

func NewFake() *FakeConnectionLogger {
	return &FakeConnectionLogger{}
}

type FakeConnectionLogger struct {
	mu         sync.Mutex
	upsertions []database.UpsertConnectionLogParams
}

func (m *FakeConnectionLogger) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.upsertions = make([]database.UpsertConnectionLogParams, 0)
}

func (m *FakeConnectionLogger) ConnectionLogs() []database.UpsertConnectionLogParams {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.upsertions
}

func (m *FakeConnectionLogger) Upsert(_ context.Context, clog database.UpsertConnectionLogParams) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.upsertions = append(m.upsertions, clog)

	return nil
}

func (m *FakeConnectionLogger) Contains(t testing.TB, expected database.UpsertConnectionLogParams) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for idx, cl := range m.upsertions {
		if expected.ID != uuid.Nil && cl.ID != expected.ID {
			t.Logf("connection log %d: expected ID %s, got %s", idx+1, expected.ID, cl.ID)
			continue
		}
		if expected.OrganizationID != uuid.Nil && cl.OrganizationID != expected.OrganizationID {
			t.Logf("connection log %d: expected OrganizationID %s, got %s", idx+1, expected.OrganizationID, cl.OrganizationID)
			continue
		}
		if expected.WorkspaceOwnerID != uuid.Nil && cl.WorkspaceOwnerID != expected.WorkspaceOwnerID {
			t.Logf("connection log %d: expected WorkspaceOwnerID %s, got %s", idx+1, expected.WorkspaceOwnerID, cl.WorkspaceOwnerID)
			continue
		}
		if expected.WorkspaceID != uuid.Nil && cl.WorkspaceID != expected.WorkspaceID {
			t.Logf("connection log %d: expected WorkspaceID %s, got %s", idx+1, expected.WorkspaceID, cl.WorkspaceID)
			continue
		}
		if expected.WorkspaceName != "" && cl.WorkspaceName != expected.WorkspaceName {
			t.Logf("connection log %d: expected WorkspaceName %s, got %s", idx+1, expected.WorkspaceName, cl.WorkspaceName)
			continue
		}
		if expected.AgentName != "" && cl.AgentName != expected.AgentName {
			t.Logf("connection log %d: expected AgentName %s, got %s", idx+1, expected.AgentName, cl.AgentName)
			continue
		}
		if expected.Type != "" && cl.Type != expected.Type {
			t.Logf("connection log %d: expected Type %s, got %s", idx+1, expected.Type, cl.Type)
			continue
		}
		if expected.Code.Valid && cl.Code.Int32 != expected.Code.Int32 {
			t.Logf("connection log %d: expected Code %d, got %d", idx+1, expected.Code.Int32, cl.Code.Int32)
			continue
		}
		if expected.Ip.Valid && cl.Ip.IPNet.String() != expected.Ip.IPNet.String() {
			t.Logf("connection log %d: expected IP %s, got %s", idx+1, expected.Ip.IPNet, cl.Ip.IPNet)
			continue
		}
		if expected.UserAgent.Valid && cl.UserAgent.String != expected.UserAgent.String {
			t.Logf("connection log %d: expected UserAgent %s, got %s", idx+1, expected.UserAgent.String, cl.UserAgent.String)
			continue
		}
		if expected.UserID.Valid && cl.UserID.UUID != expected.UserID.UUID {
			t.Logf("connection log %d: expected UserID %s, got %s", idx+1, expected.UserID.UUID, cl.UserID.UUID)
			continue
		}
		if expected.SlugOrPort.Valid && cl.SlugOrPort.String != expected.SlugOrPort.String {
			t.Logf("connection log %d: expected SlugOrPort %s, got %s", idx+1, expected.SlugOrPort.String, cl.SlugOrPort.String)
			continue
		}
		if expected.ConnectionID.Valid && cl.ConnectionID.UUID != expected.ConnectionID.UUID {
			t.Logf("connection log %d: expected ConnectionID %s, got %s", idx+1, expected.ConnectionID.UUID, cl.ConnectionID.UUID)
			continue
		}
		if expected.DisconnectReason.Valid && cl.DisconnectReason.String != expected.DisconnectReason.String {
			t.Logf("connection log %d: expected DisconnectReason %s, got %s", idx+1, expected.DisconnectReason.String, cl.DisconnectReason.String)
			continue
		}
		if !expected.Time.IsZero() && expected.Time != cl.Time {
			t.Logf("connection log %d: expected Time %s, got %s", idx+1, expected.Time, cl.Time)
			continue
		}
		if expected.ConnectionStatus != "" && expected.ConnectionStatus != cl.ConnectionStatus {
			t.Logf("connection log %d: expected ConnectionStatus %s, got %s", idx+1, expected.ConnectionStatus, cl.ConnectionStatus)
			continue
		}
		return true
	}

	return false
}
