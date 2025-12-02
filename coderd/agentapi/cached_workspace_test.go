package agentapi_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/agentapi"
	"github.com/coder/coder/v2/coderd/database"
)

func TestCacheClear(t *testing.T) {
	t.Parallel()

	var (
		user = database.User{
			ID:       uuid.New(),
			Username: "bill",
		}
		template = database.Template{
			ID:   uuid.New(),
			Name: "tpl",
		}
		workspace = database.Workspace{
			ID:            uuid.New(),
			OwnerID:       user.ID,
			OwnerUsername: user.Username,
			TemplateID:    template.ID,
			Name:          "xyz",
			TemplateName:  template.Name,
		}
		workspaceAsCacheFields = agentapi.CachedWorkspaceFields{}
	)

	workspaceAsCacheFields.UpdateValues(database.Workspace{
		ID:                workspace.ID,
		OwnerID:           workspace.OwnerID,
		OwnerUsername:     workspace.OwnerUsername,
		TemplateID:        workspace.TemplateID,
		Name:              workspace.Name,
		TemplateName:      workspace.TemplateName,
		AutostartSchedule: workspace.AutostartSchedule,
	},
	)

	emptyCws := agentapi.CachedWorkspaceFields{}
	workspaceAsCacheFields.Clear()
	wsi, ok := workspaceAsCacheFields.AsWorkspaceIdentity()
	require.False(t, ok)
	ecwsi, ok := emptyCws.AsWorkspaceIdentity()
	require.False(t, ok)
	require.True(t, ecwsi.Equal(wsi))
}

func TestCacheUpdate(t *testing.T) {
	t.Parallel()

	var (
		user = database.User{
			ID:       uuid.New(),
			Username: "bill",
		}
		template = database.Template{
			ID:   uuid.New(),
			Name: "tpl",
		}
		workspace = database.Workspace{
			ID:            uuid.New(),
			OwnerID:       user.ID,
			OwnerUsername: user.Username,
			TemplateID:    template.ID,
			Name:          "xyz",
			TemplateName:  template.Name,
		}
		workspaceAsCacheFields = agentapi.CachedWorkspaceFields{}
	)

	workspaceAsCacheFields.UpdateValues(database.Workspace{
		ID:                workspace.ID,
		OwnerID:           workspace.OwnerID,
		OwnerUsername:     workspace.OwnerUsername,
		TemplateID:        workspace.TemplateID,
		Name:              workspace.Name,
		TemplateName:      workspace.TemplateName,
		AutostartSchedule: workspace.AutostartSchedule,
	},
	)

	cws := agentapi.CachedWorkspaceFields{}
	cws.UpdateValues(workspace)
	wsi, ok := workspaceAsCacheFields.AsWorkspaceIdentity()
	require.True(t, ok)
	cwsi, ok := cws.AsWorkspaceIdentity()
	require.True(t, ok)
	require.True(t, wsi.Equal(cwsi))
}
