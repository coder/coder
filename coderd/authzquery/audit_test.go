package authzquery_test

import (
	"testing"

	"github.com/coder/coder/coderd/database/dbgen"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/rbac"
)

func (s *MethodTestSuite) TestAuditLogs() {
	s.Run("InsertAuditLog", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			return methodCase(values(database.InsertAuditLogParams{
				ResourceType: database.ResourceTypeOrganization,
				Action:       database.AuditActionCreate,
			}),
				asserts(rbac.ResourceAuditLog, rbac.ActionCreate),
				nil)
		})
	})
	s.Run("GetAuditLogsOffset", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			_ = dbgen.AuditLog(t, db, database.AuditLog{})
			_ = dbgen.AuditLog(t, db, database.AuditLog{})
			return methodCase(values(database.GetAuditLogsOffsetParams{
				Limit: 10,
			}),
				asserts(rbac.ResourceAuditLog, rbac.ActionRead),
				nil)
		})
	})
}
