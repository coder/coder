package connectionlog

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
)

type ConnectionLogger interface {
	Export(ctx context.Context, clog database.ConnectionLog) error
}

type nop struct{}

func NewNop() ConnectionLogger {
	return nop{}
}

func (nop) Export(context.Context, database.ConnectionLog) error {
	return nil
}

func NewMock() *MockConnectionLogger {
	return &MockConnectionLogger{}
}

type MockConnectionLogger struct {
	mu             sync.Mutex
	connectionLogs []database.ConnectionLog
}

func (m *MockConnectionLogger) ResetLogs() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connectionLogs = make([]database.ConnectionLog, 0)
}

func (m *MockConnectionLogger) ConnectionLogs() []database.ConnectionLog {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.connectionLogs
}

func (m *MockConnectionLogger) Export(_ context.Context, clog database.ConnectionLog) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.connectionLogs = append(m.connectionLogs, clog)

	return nil
}

func (m *MockConnectionLogger) Contains(t testing.TB, expected database.ConnectionLog) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for idx, cl := range m.connectionLogs {
		if expected.ID != uuid.Nil && cl.ID != expected.ID {
			t.Logf("connection log %d: expected ID %s, got %s", idx+1, expected.ID, cl.ID)
			continue
		}
		if !expected.Time.IsZero() && expected.Time != cl.Time {
			t.Logf("connection log %d: expected Time %s, got %s", idx+1, expected.Time, cl.Time)
			continue
		}
		if expected.ConnectionID != uuid.Nil && cl.ConnectionID != expected.ConnectionID {
			t.Logf("connection log %d: expected ConnectionID %s, got %s", idx+1, expected.ConnectionID, cl.ConnectionID)
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
		if expected.Action != "" && cl.Action != expected.Action {
			t.Logf("connection log %d: expected Action %s, got %s", idx+1, expected.Action, cl.Action)
			continue
		}
		if expected.Code != 0 && cl.Code != expected.Code {
			t.Logf("connection log %d: expected Code %d, got %d", idx+1, expected.Code, cl.Code)
			continue
		}
		if expected.Ip.Valid && cl.Ip.IPNet.String() != expected.Ip.IPNet.String() {
			t.Logf("connection log %d: expected IP %s, got %s", idx+1, expected.Ip.IPNet, cl.Ip.IPNet)
			continue
		}
		if expected.SlugOrPort.Valid && cl.SlugOrPort != expected.SlugOrPort {
			t.Logf("connection log %d: expected SlugOrPort %s, got %s", idx+1, expected.SlugOrPort.String, cl.SlugOrPort.String)
			continue
		}
		if expected.UserAgent.Valid && cl.UserAgent != expected.UserAgent {
			t.Logf("connection log %d: expected UserAgent %s, got %s", idx+1, expected.UserAgent.String, cl.UserAgent.String)
			continue
		}
		if expected.UserID != uuid.Nil && cl.UserID != expected.UserID {
			t.Logf("connection log %d: expected UserID %s, got %s", idx+1, expected.UserID, cl.UserID)
			continue
		}
		if expected.ConnectionType.Valid && cl.ConnectionType.ConnectionTypeEnum != expected.ConnectionType.ConnectionTypeEnum {
			t.Logf("connection log %d: expected ConnectionType %s, got %s", idx+1, expected.ConnectionType.ConnectionTypeEnum, cl.ConnectionType.ConnectionTypeEnum)
			continue
		}
		if expected.Reason.Valid && cl.Reason != expected.Reason {
			t.Logf("connection log %d: expected Reason %s, got %s", idx+1, expected.Reason.String, cl.Reason.String)
			continue
		}
		return true
	}

	return false
}
