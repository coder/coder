package agentfake

import (
	"database/sql"
	"testing"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/codersdk"
	sdkproto "github.com/coder/coder/v2/provisionersdk/proto"
)

// BuildExternalAgentWorkspace creates one workspace with a coder_external_agent resource, an agent, and
// HasExternalAgent=true on the latest build. If templateID is uuid.Nil, dbfake mints a fresh template (and the caller
// can pass the returned Workspace.TemplateID into subsequent calls to share the template).
func BuildExternalAgentWorkspace(
	t *testing.T,
	db database.Store,
	user codersdk.CreateFirstUserResponse,
	templateID uuid.UUID,
) dbfake.WorkspaceResponse {
	t.Helper()

	ws := database.WorkspaceTable{
		OrganizationID: user.OrganizationID,
		OwnerID:        user.UserID,
	}
	if templateID != uuid.Nil {
		ws.TemplateID = templateID
	}
	return dbfake.WorkspaceBuild(t, db, ws).
		Seed(database.WorkspaceBuild{
			HasExternalAgent: sql.NullBool{Bool: true, Valid: true},
		}).
		Resource(&sdkproto.Resource{
			Name: "external",
			Type: "coder_external_agent",
		}).
		WithAgent().
		Do()
}
