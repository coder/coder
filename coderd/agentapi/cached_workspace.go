package agentapi

import (
	"database/sql"
	"sync"

	"github.com/google/uuid"

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

	// Identity fields
	ID             uuid.UUID
	OwnerID        uuid.UUID
	OrganizationID uuid.UUID
	TemplateID     uuid.UUID

	// Display fields for logging/metrics
	Name          string
	OwnerUsername string
	TemplateName  string

	// Lifecycle fields needed for stats reporting
	AutostartSchedule sql.NullString
}

func (cws *CachedWorkspaceFields) Equal(cws2 *CachedWorkspaceFields) bool {
	cws.lock.RLock()
	defer cws.lock.RUnlock()
	cws2.lock.RLock()
	defer cws2.lock.RUnlock()

	return cws.ID == cws2.ID && cws.OwnerID == cws2.OwnerID && cws.OrganizationID == cws2.OrganizationID &&
		cws.TemplateID == cws2.TemplateID && cws.Name == cws2.Name && cws.OwnerUsername == cws2.OwnerUsername &&
		cws.TemplateName == cws2.TemplateName && cws.AutostartSchedule == cws2.AutostartSchedule
}

func (cws *CachedWorkspaceFields) Clear() {
	cws.lock.Lock()
	defer cws.lock.Unlock()
	cws.ID = uuid.UUID{}
	cws.OwnerID = uuid.UUID{}
	cws.OrganizationID = uuid.UUID{}
	cws.TemplateID = uuid.UUID{}
	cws.Name = ""
	cws.OwnerUsername = ""
	cws.TemplateName = ""
	cws.AutostartSchedule = sql.NullString{}
}

func (cws *CachedWorkspaceFields) UpdateValues(ws database.Workspace) {
	cws.lock.Lock()
	defer cws.lock.Unlock()
	cws.ID = ws.ID
	cws.OwnerID = ws.OwnerID
	cws.OrganizationID = ws.OrganizationID
	cws.TemplateID = ws.TemplateID
	cws.Name = ws.Name
	cws.OwnerUsername = ws.OwnerUsername
	cws.TemplateName = ws.TemplateName
	cws.AutostartSchedule = ws.AutostartSchedule
}

func (cws *CachedWorkspaceFields) AsDatabaseWorkspace() database.Workspace {
	cws.lock.RLock()
	defer cws.lock.RUnlock()
	return database.Workspace{
		ID:                cws.ID,
		OwnerID:           cws.OwnerID,
		OrganizationID:    cws.OrganizationID,
		TemplateID:        cws.TemplateID,
		Name:              cws.Name,
		OwnerUsername:     cws.OwnerUsername,
		TemplateName:      cws.TemplateName,
		AutostartSchedule: cws.AutostartSchedule,
	}
}
