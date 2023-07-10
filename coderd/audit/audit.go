package audit

import (
	"context"
	"sync"

	"github.com/coder/coder/coderd/database"
)

type Auditor interface {
	Export(ctx context.Context, alog database.AuditLog) error
	diff(old, new any) Map
}

type AdditionalFields struct {
	WorkspaceName  string               `json:"workspace_name"`
	BuildNumber    string               `json:"build_number"`
	BuildReason    database.BuildReason `json:"build_reason"`
	WorkspaceOwner string               `json:"workspace_owner"`
}

func NewNop() Auditor {
	return nop{}
}

type nop struct{}

func (nop) Export(context.Context, database.AuditLog) error {
	return nil
}

func (nop) diff(any, any) Map {
	return Map{}
}

func NewMock() *MockAuditor {
	return &MockAuditor{}
}

type MockAuditor struct {
	mutex     sync.Mutex
	auditLogs []database.AuditLog
}

// ResetLogs removes all audit logs from the mock auditor.
// This is helpful for testing to get a clean slate.
func (a *MockAuditor) ResetLogs() {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	a.auditLogs = make([]database.AuditLog, 0)
}

func (a *MockAuditor) AuditLogs() []database.AuditLog {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	logs := make([]database.AuditLog, len(a.auditLogs))
	copy(logs, a.auditLogs)
	return logs
}

func (a *MockAuditor) Export(_ context.Context, alog database.AuditLog) error {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	a.auditLogs = append(a.auditLogs, alog)
	return nil
}

func (*MockAuditor) diff(any, any) Map {
	return Map{}
}
