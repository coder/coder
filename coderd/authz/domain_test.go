package authz

import (
	"testing"

	"github.com/coder/coder/coderd/authz/rbac"
	"github.com/stretchr/testify/assert"
)

func TestResolveSiteEnforcer(t *testing.T) {
	siteEnforcerMap := SiteEnforcer.Resolve()

	// SiteAdmin inherited from Auditor
	assert.Exactly(t, siteEnforcerMap[SiteAdmin][AuditLogs][ReadAll], true)

	// SiteAuditor inherited from SiteMember
	assert.Exactly(t, siteEnforcerMap[SiteAuditor][DevURLs][CreateOwn], true)

	// SiteManager merged Metrics perms with SiteMember
	assert.Exactly(t, siteEnforcerMap[SiteManager][Metrics][ReadOwn], true)
	assert.Exactly(t, siteEnforcerMap[SiteManager][Metrics][ReadAll], true)
}

func TestResolveOrgEnforcer(t *testing.T) {
	orgEnforcerMap := OrganizationEnforcer.Resolve()

	// OrganizationAdmin is non-nill. This is a useful test right now because
	// OrganizationAdmin is not defined in the enforcer permissions, but is
	// defined in the inheritances.
	assert.NotNil(t, orgEnforcerMap[OrganizationAdmin])

	// OrganizationManager merged Workspaces perms with OrganizationMember
	testOps := rbac.Operations{CreateOwn, ReadAll, UpdateAll, UpdateOwn, DeleteAll}
	for _, op := range testOps {
		assert.Exactly(t, orgEnforcerMap[OrganizationManager][Workspaces][op], true)
	}
}
