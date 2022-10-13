package audit

import (
	"context"

	"github.com/coder/coder/coderd/database"
)

type Auditor interface {
	Export(ctx context.Context, alog database.AuditLog) error
	diff(old, new any) Map
}

func NewNop() Auditor {
	return nop{}
}

type nop struct{}

func (nop) Export(context.Context, database.AuditLog) error {
	return nil
}

func (nop) diff(any, any) Map { return Map{} }

func NewMock() *MockAuditor {
	return &MockAuditor{}
}

type MockAuditor struct {
	AuditLogs []database.AuditLog
}

func (a *MockAuditor) Export(_ context.Context, alog database.AuditLog) error {
	a.AuditLogs = append(a.AuditLogs, alog)
	return nil
}

func (*MockAuditor) diff(any, any) Map { return Map{} }
