package dbauthz_test

import (
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbgen"
	"github.com/coder/coder/coderd/rbac"
)

func (s *MethodTestSuite) TestAuditLogs() {
	s.Run("InsertAuditLog", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.InsertAuditLogParams{
			ResourceType: database.ResourceTypeOrganization,
			Action:       database.AuditActionCreate,
		}).Asserts(rbac.ResourceAuditLog, rbac.ActionCreate)
	}))
	s.Run("GetAuditLogsOffset", s.Subtest(func(db database.Store, check *expects) {
		_ = dbgen.AuditLog(s.T(), db, database.AuditLog{})
		_ = dbgen.AuditLog(s.T(), db, database.AuditLog{})
		check.Args(database.GetAuditLogsOffsetParams{
			Limit: 10,
		}).Asserts(rbac.ResourceAuditLog, rbac.ActionRead)
	}))
}
