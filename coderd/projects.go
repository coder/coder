package coderd

import (
	"net/http"

	"github.com/coder/coder/xjson"
)

type ProjectParameter struct {
	Id          string `json:"id" validate:"required"`
	Name        string `json:"name" validate:"required"`
	Description string `json:"description"`

	// Validation Parameters
	ValueType string `json:"validation_value_type"`
}

// Project is a Go representation of the workspaces v2 project,
// defined here: https://www.notion.so/coderhq/Workspaces-v2-e908a8cd54804ddd910367abf03c8d0a#befa328add894231979e6cf8a378d2ec
type Project struct {
	Id              string `json:"id" validate:"required"`
	Name            string `json:"name" validate:"required"`
	Description     string `json:"description" validate:"required"`
	ProvisionerType string `json:"provisioner_type" validate:"required"`

	Parameters []ProjectParameter `json:"parameters" validate:"required"`
}

// Placeholder type of projectService
type projectService struct {
}

func newProjectService() *projectService {
	projectService := &projectService{}
	return projectService
}

func (ps *projectService) getProjects(w http.ResponseWriter, r *http.Request) {

	// Construct a couple hard-coded projects to return the UI

	terraformProject := Project{
		Id:          "test_terraform_project_id",
		Name:        "Terraform",
		Description: "Kubernetes on Terraform",
		Parameters: []ProjectParameter{
			{
				Id:          "parameter_cluster_namespace",
				Name:        "Namespace",
				Description: "Kubernetes namespace to host workspace pod",
				ValueType:   "string",
			},
			{
				Id:          "parameter_cpu",
				Name:        "CPU",
				Description: "CPU Cores to Allocate",
				ValueType:   "number",
			},
		},
	}

	echoProject := Project{
		Id:          "test_echo_project_id",
		Name:        "Echo Project",
		Description: "A simple echo provider",
		Parameters: []ProjectParameter{
			{
				Id:          "parameter_echo_string",
				Name:        "Echo String",
				Description: "String that should be echo'd out in build log",
				ValueType:   "string",
			},
		},
	}

	projects := []Project{
		terraformProject,
		echoProject,
	}

	xjson.Write(w, http.StatusOK, projects)
}

func (ps *projectService) createProject(w http.ResponseWriter, r *http.Request) {
	// TODO: Validate arguments
	// Organization context
	// User
	// Parameter values
	// Submit to provisioner
	xjson.Write(w, http.StatusOK, nil)
}
