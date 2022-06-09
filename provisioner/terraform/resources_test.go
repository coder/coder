package terraform_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/provisioner/terraform"
	"github.com/coder/coder/provisionersdk/proto"
)

func TestConvertResources(t *testing.T) {
	t.Parallel()
	// nolint:dogsled
	_, filename, _, _ := runtime.Caller(0)
	// nolint:paralleltest
	for folderName, expected := range map[string][]*proto.Resource{
		"chaining-resources": {{
			Name: "first",
			Type: "null_resource",
			Agents: []*proto.Agent{{
				Name:            "dev1",
				OperatingSystem: "linux",
				Architecture:    "amd64",
				Auth:            &proto.Agent_Token{},
			}},
		}, {
			Name: "second",
			Type: "null_resource",
		}},
		"instance-id": {{
			Name: "dev",
			Type: "null_resource",
			Agents: []*proto.Agent{{
				Name:            "dev",
				OperatingSystem: "linux",
				Architecture:    "amd64",
				Auth:            &proto.Agent_InstanceId{},
			}},
		}},
		"calling-module": {{
			Name: "example",
			Type: "null_resource",
			Agents: []*proto.Agent{{
				Name:            "dev",
				OperatingSystem: "linux",
				Architecture:    "amd64",
				Auth:            &proto.Agent_Token{},
			}},
		}},
		"multiple-agents": {{
			Name: "dev",
			Type: "null_resource",
			Agents: []*proto.Agent{{
				Name:            "dev1",
				OperatingSystem: "linux",
				Architecture:    "amd64",
				Auth:            &proto.Agent_Token{},
			}, {
				Name:            "dev2",
				OperatingSystem: "darwin",
				Architecture:    "amd64",
				Auth:            &proto.Agent_Token{},
			}, {
				Name:            "dev3",
				OperatingSystem: "windows",
				Architecture:    "arm64",
				Auth:            &proto.Agent_Token{},
			}},
		}},
	} {
		folderName := folderName
		expected := expected
		t.Run(folderName, func(t *testing.T) {
			t.Parallel()
			dir := filepath.Join(filepath.Dir(filename), "testdata", folderName)
			t.Run("Plan", func(t *testing.T) {
				t.Parallel()

				tfPlanRaw, err := os.ReadFile(filepath.Join(dir, folderName+".tfplan.json"))
				require.NoError(t, err)
				var tfPlan tfjson.Plan
				err = json.Unmarshal(tfPlanRaw, &tfPlan)
				require.NoError(t, err)
				tfPlanGraph, err := os.ReadFile(filepath.Join(dir, folderName+".tfplan.dot"))
				require.NoError(t, err)

				resources, err := terraform.ConvertResources(tfPlan.PlannedValues.RootModule, string(tfPlanGraph))
				require.NoError(t, err)
				for _, resource := range resources {
					sort.Slice(resource.Agents, func(i, j int) bool {
						return resource.Agents[i].Name < resource.Agents[j].Name
					})
				}
				resourcesWant, err := json.Marshal(expected)
				require.NoError(t, err)
				resourcesGot, err := json.Marshal(resources)
				require.NoError(t, err)
				require.Equal(t, string(resourcesWant), string(resourcesGot))
			})
			t.Run("Provision", func(t *testing.T) {
				t.Parallel()
				tfStateRaw, err := os.ReadFile(filepath.Join(dir, folderName+".tfstate.json"))
				require.NoError(t, err)
				var tfState tfjson.State
				err = json.Unmarshal(tfStateRaw, &tfState)
				require.NoError(t, err)
				tfStateGraph, err := os.ReadFile(filepath.Join(dir, folderName+".tfstate.dot"))
				require.NoError(t, err)

				resources, err := terraform.ConvertResources(tfState.Values.RootModule, string(tfStateGraph))
				require.NoError(t, err)
				for _, resource := range resources {
					sort.Slice(resource.Agents, func(i, j int) bool {
						return resource.Agents[i].Name < resource.Agents[j].Name
					})
					for _, agent := range resource.Agents {
						agent.Id = ""
						if agent.GetToken() != "" {
							agent.Auth = &proto.Agent_Token{}
						}
						if agent.GetInstanceId() != "" {
							agent.Auth = &proto.Agent_InstanceId{}
						}
					}
				}
				resourcesWant, err := json.Marshal(expected)
				require.NoError(t, err)
				resourcesGot, err := json.Marshal(resources)
				require.NoError(t, err)

				require.Equal(t, string(resourcesWant), string(resourcesGot))
			})
		})
	}
}
