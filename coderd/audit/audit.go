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
	AuditLogs []database.AuditLog
}

func (a *MockAuditor) Export(_ context.Context, alog database.AuditLog) error {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	a.AuditLogs = append(a.AuditLogs, alog)
	return nil
}

func (*MockAuditor) diff(any, any) Map {
	return Map{}
}
