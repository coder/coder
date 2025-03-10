package audit

import (
	"context"
	"slices"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
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
	WorkspaceID    uuid.UUID            `json:"workspace_id"`
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

// Contains returns true if, for each non-zero-valued field in expected,
// there exists a corresponding audit log in the mock auditor that matches
// the expected values. Returns false otherwise.
func (a *MockAuditor) Contains(t testing.TB, expected database.AuditLog) bool {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	for idx, al := range a.auditLogs {
		if expected.ID != uuid.Nil && al.ID != expected.ID {
			t.Logf("audit log %d: expected ID %s, got %s", idx+1, expected.ID, al.ID)
			continue
		}
		if !expected.Time.IsZero() && expected.Time != al.Time {
			t.Logf("audit log %d: expected Time %s, got %s", idx+1, expected.Time, al.Time)
			continue
		}
		if expected.UserID != uuid.Nil && al.UserID != expected.UserID {
			t.Logf("audit log %d: expected UserID %s, got %s", idx+1, expected.UserID, al.UserID)
			continue
		}
		if expected.OrganizationID != uuid.Nil && al.UserID != expected.UserID {
			t.Logf("audit log %d: expected OrganizationID %s, got %s", idx+1, expected.OrganizationID, al.OrganizationID)
			continue
		}
		if expected.Ip.Valid && al.Ip.IPNet.String() != expected.Ip.IPNet.String() {
			t.Logf("audit log %d: expected Ip %s, got %s", idx+1, expected.Ip.IPNet, al.Ip.IPNet)
			continue
		}
		if expected.UserAgent.Valid && al.UserAgent.String != expected.UserAgent.String {
			t.Logf("audit log %d: expected UserAgent %s, got %s", idx+1, expected.UserAgent.String, al.UserAgent.String)
			continue
		}
		if expected.ResourceType != "" && expected.ResourceType != al.ResourceType {
			t.Logf("audit log %d: expected ResourceType %s, got %s", idx+1, expected.ResourceType, al.ResourceType)
			continue
		}
		if expected.ResourceID != uuid.Nil && expected.ResourceID != al.ResourceID {
			t.Logf("audit log %d: expected ResourceID %s, got %s", idx+1, expected.ResourceID, al.ResourceID)
			continue
		}
		if expected.ResourceTarget != "" && expected.ResourceTarget != al.ResourceTarget {
			t.Logf("audit log %d: expected ResourceTarget %s, got %s", idx+1, expected.ResourceTarget, al.ResourceTarget)
			continue
		}
		if expected.Action != "" && expected.Action != al.Action {
			t.Logf("audit log %d: expected Action %s, got %s", idx+1, expected.Action, al.Action)
			continue
		}
		if len(expected.Diff) > 0 && slices.Compare(expected.Diff, al.Diff) != 0 {
			t.Logf("audit log %d: expected Diff %s, got %s", idx+1, string(expected.Diff), string(al.Diff))
			continue
		}
		if expected.StatusCode != 0 && expected.StatusCode != al.StatusCode {
			t.Logf("audit log %d: expected StatusCode %d, got %d", idx+1, expected.StatusCode, al.StatusCode)
			continue
		}
		if len(expected.AdditionalFields) > 0 && slices.Compare(expected.AdditionalFields, al.AdditionalFields) != 0 {
			t.Logf("audit log %d: expected AdditionalFields %s, got %s", idx+1, string(expected.AdditionalFields), string(al.AdditionalFields))
			continue
		}
		if expected.RequestID != uuid.Nil && expected.RequestID != al.RequestID {
			t.Logf("audit log %d: expected RequestID %s, got %s", idx+1, expected.RequestID, al.RequestID)
			continue
		}
		if expected.ResourceIcon != "" && expected.ResourceIcon != al.ResourceIcon {
			t.Logf("audit log %d: expected ResourceIcon %s, got %s", idx+1, expected.ResourceIcon, al.ResourceIcon)
			continue
		}
		return true
	}

	return false
}
