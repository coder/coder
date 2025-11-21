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
		workspaceAsCacheFields = agentapi.CachedWorkspaceFields{
			ID:                workspace.ID,
			OwnerID:           workspace.OwnerID,
			OwnerUsername:     workspace.OwnerUsername,
			TemplateID:        workspace.TemplateID,
			Name:              workspace.Name,
			TemplateName:      workspace.TemplateName,
			AutostartSchedule: workspace.AutostartSchedule,
		}
	)

	emptyCws := agentapi.CachedWorkspaceFields{}
	workspaceAsCacheFields.Clear()
	require.True(t, emptyCws.Equal(&workspaceAsCacheFields))
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
		workspaceAsCacheFields = agentapi.CachedWorkspaceFields{
			ID:                workspace.ID,
			OwnerID:           workspace.OwnerID,
			OwnerUsername:     workspace.OwnerUsername,
			TemplateID:        workspace.TemplateID,
			Name:              workspace.Name,
			TemplateName:      workspace.TemplateName,
			AutostartSchedule: workspace.AutostartSchedule,
		}
	)

	cws := agentapi.CachedWorkspaceFields{}
	cws.UpdateValues(workspace)
	require.True(t, workspaceAsCacheFields.Equal(&cws))
}
