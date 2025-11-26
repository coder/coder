package agentapi

import (
	"sync"

	"github.com/coder/coder/v2/coderd/database"
)

// CachedWorkspaceFields contains workspace data that is safe to cache for the
// duration of an agent connection. These fields are used to reduce database calls
// in high-frequency operations like stats reporting and metadata updates.
// Prebuild workspaces should not be cached using this struct within the API struct,
// however some of these fields for a workspace can be updated live so there is a
// routine in the API for refreshing the workspace on a timed interval.
//
// IMPORTANT: ACL fields (GroupACL, UserACL) are NOT cached because they can be
// modified in the database and we must use fresh data for authorization checks.
type CachedWorkspaceFields struct {
	lock sync.RWMutex

	identity database.WorkspaceIdentity
}

func (cws *CachedWorkspaceFields) Clear() {
	cws.lock.Lock()
	defer cws.lock.Unlock()
	cws.identity = database.WorkspaceIdentity{}
}

func (cws *CachedWorkspaceFields) UpdateValues(ws database.Workspace) {
	cws.lock.Lock()
	defer cws.lock.Unlock()
	cws.identity.ID = ws.ID
	cws.identity.OwnerID = ws.OwnerID
	cws.identity.OrganizationID = ws.OrganizationID
	cws.identity.TemplateID = ws.TemplateID
	cws.identity.Name = ws.Name
	cws.identity.OwnerUsername = ws.OwnerUsername
	cws.identity.TemplateName = ws.TemplateName
	cws.identity.AutostartSchedule = ws.AutostartSchedule
}

// Returns the Workspace, true, unless the workspace has not been cached (nuked or was a prebuild).
func (cws *CachedWorkspaceFields) AsWorkspaceIdentity() (database.WorkspaceIdentity, bool) {
	cws.lock.RLock()
	defer cws.lock.RUnlock()
	// Should we be more explicit about all fields being set to be valid?
	if cws.identity.Equal(database.WorkspaceIdentity{}) {
		return database.WorkspaceIdentity{}, false
	}
	return cws.identity, true
}
