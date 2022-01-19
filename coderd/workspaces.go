package coderd

import (
	"net/http"

	"github.com/coder/coder/xjson"
)

type Workspace struct {
	Id        string `json:"id" validate:"required"`
	Name      string `json:"name" validate:"required"`
	ProjectId string `json:"project_id" validate:"required"`
}

// Placeholder type of workspaceService
type workspaceService struct {
}

func newWorkspaceService() *workspaceService {
	workspaceService := &workspaceService{}
	return workspaceService
}

func (ws *workspaceService) getWorkspaces(w http.ResponseWriter, r *http.Request) {
	// Dummy workspace to return
	workspace := Workspace{
		Id:        "test-workspace",
		Name:      "Test Workspace",
		ProjectId: "test-project-id",
	}

	workspaces := []Workspace{
		workspace,
	}

	xjson.Write(w, http.StatusOK, workspaces)
}

func (ws *workspaceService) getWorkspaceById(w http.ResponseWriter, r *http.Request) {
	// TODO: Read workspace off context
	// Dummy workspace to return
	workspace := Workspace{
		Id:        "test-workspace",
		Name:      "Test Workspace",
		ProjectId: "test-project-id",
	}
	xjson.Write(w, http.StatusOK, workspace)
}