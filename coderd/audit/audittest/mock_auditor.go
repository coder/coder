package audittest

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
	"golang.org/x/exp/slices"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
)

const bufSize = 0 // unbuffered

func NewMock(t testing.TB) *MockAuditor {
	return &MockAuditor{t: t}
}

type MockAuditor struct {
	t           testing.TB
	mutex       sync.Mutex
	auditLogs   []database.AuditLog
	subscribers []chan database.AuditLog
}

var _ audit.Auditor = (*MockAuditor)(nil)

// ResetLogs removes all audit logs from the mock auditor.
// This is helpful for testing to get a clean slate.
// It also closes all subscribers.
func (a *MockAuditor) ResetLogs() {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	a.auditLogs = make([]database.AuditLog, 0)
	for _, c := range a.subscribers {
		close(c)
	}
	a.subscribers = make([]chan database.AuditLog, 0)
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
	for _, c := range a.subscribers {
		go func() {
			c <- alog
		}()
	}
	return nil
}

func (*MockAuditor) Diff(any, any) audit.Map {
	return audit.Map{}
}

// Contains returns true if, for each non-zero-valued field in expected,
// there exists a corresponding audit log in the mock auditor that matches
// the expected values. Returns false otherwise.
func (a *MockAuditor) Contains(t testing.TB, expected database.AuditLog) bool {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	c := make(chan database.AuditLog, bufSize)
	a.subscribers = append(a.subscribers, c)
	a.mutex.Unlock()
	return MatchAuditLog(t, expected, c)
}

func MatchAuditLog(t testing.TB, expected database.AuditLog, c <-chan database.AuditLog) bool {
	t.Helper()
	for al := range c {
		if expected.ID != uuid.Nil && al.ID != expected.ID {
			t.Logf("audit log: expected ID %s, got %s", expected.ID, al.ID)
			continue
		}
		if !expected.Time.IsZero() && expected.Time != al.Time {
			t.Logf("audit log: expected Time %s, got %s", expected.Time, al.Time)
			continue
		}
		if expected.UserID != uuid.Nil && al.UserID != expected.UserID {
			t.Logf("audit log: expected UserID %s, got %s", expected.UserID, al.UserID)
			continue
		}
		if expected.OrganizationID != uuid.Nil && al.UserID != expected.UserID {
			t.Logf("audit log: expected OrganizationID %s, got %s", expected.OrganizationID, al.OrganizationID)
			continue
		}
		if expected.Ip.Valid && al.Ip.IPNet.String() != expected.Ip.IPNet.String() {
			t.Logf("audit log: expected Ip %s, got %s", expected.Ip.IPNet, al.Ip.IPNet)
			continue
		}
		if expected.UserAgent.Valid && al.UserAgent.String != expected.UserAgent.String {
			t.Logf("audit log: expected UserAgent %s, got %s", expected.UserAgent.String, al.UserAgent.String)
			continue
		}
		if expected.ResourceType != "" && expected.ResourceType != al.ResourceType {
			t.Logf("audit log: expected ResourceType %s, got %s", expected.ResourceType, al.ResourceType)
			continue
		}
		if expected.ResourceID != uuid.Nil && expected.ResourceID != al.ResourceID {
			t.Logf("audit log: expected ResourceID %s, got %s", expected.ResourceID, al.ResourceID)
			continue
		}
		if expected.ResourceTarget != "" && expected.ResourceTarget != al.ResourceTarget {
			t.Logf("audit log: expected ResourceTarget %s, got %s", expected.ResourceTarget, al.ResourceTarget)
			continue
		}
		if expected.Action != "" && expected.Action != al.Action {
			t.Logf("audit log: expected Action %s, got %s", expected.Action, al.Action)
			continue
		}
		if len(expected.Diff) > 0 && slices.Compare(expected.Diff, al.Diff) != 0 {
			t.Logf("audit log: expected Diff %s, got %s", string(expected.Diff), string(al.Diff))
			continue
		}
		if expected.StatusCode != 0 && expected.StatusCode != al.StatusCode {
			t.Logf("audit log: expected StatusCode %d, got %d", expected.StatusCode, al.StatusCode)
			continue
		}
		if len(expected.AdditionalFields) > 0 && slices.Compare(expected.AdditionalFields, al.AdditionalFields) != 0 {
			t.Logf("audit log: expected AdditionalFields %s, got %s", string(expected.AdditionalFields), string(al.AdditionalFields))
			continue
		}
		if expected.RequestID != uuid.Nil && expected.RequestID != al.RequestID {
			t.Logf("audit log: expected RequestID %s, got %s", expected.RequestID, al.RequestID)
			continue
		}
		if expected.ResourceIcon != "" && expected.ResourceIcon != al.ResourceIcon {
			t.Logf("audit log: expected ResourceIcon %s, got %s", expected.ResourceIcon, al.ResourceIcon)
			continue
		}
		t.Logf("audit log: matched expected log %s", expected.ID)
		return true
	}
	// If we get here, we didn't match any logs
	t.Error("audit log: expected more logs, but got none and couldn't match expected log")
	return false
}
