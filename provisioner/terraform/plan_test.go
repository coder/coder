package terraform_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/provisioner/terraform"
	"github.com/coder/coder/provisionersdk"
	"github.com/coder/coder/provisionersdk/proto"
	"github.com/stretchr/testify/require"
)

func TestPlan(t *testing.T) {
	t.Parallel()

	client, server := provisionersdk.TransportPipe()
	ctx, cancelFunc := context.WithCancel(context.Background())
	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
		cancelFunc()
	})
	go func() {
		err := terraform.Serve(ctx, &terraform.ServeOptions{
			ServeOptions: &provisionersdk.ServeOptions{
				Listener: server,
			},
			Logger: slogtest.Make(t, nil).Leveled(slog.LevelDebug),
		})
		require.NoError(t, err)
	}()
	api := proto.NewDRPCProvisionerClient(provisionersdk.Conn(client))

	for _, testCase := range []struct {
		Name     string
		Files    map[string]string
		Request  *proto.Plan_Request
		Response *proto.Plan_Response
		Error    bool
	}{{
		Name: "single-variable",
		Files: map[string]string{
			"main.tf": `variable "A" {
				description = "Testing!"
			}`,
		},
		Request: &proto.Plan_Request{
			ParameterValues: []*proto.ParameterValue{{
				DestinationScheme: proto.ParameterDestination_PROVISIONER_VARIABLE,
				Name:              "A",
				Value:             "example",
			}},
		},
		Response: &proto.Plan_Response{
			Type: &proto.Plan_Response_Complete{
				Complete: &proto.Plan_Complete{},
			},
		},
	}, {
		Name: "missing-variable",
		Files: map[string]string{
			"main.tf": `variable "A" {
			}`,
		},
		Error: true,
	}, {
		Name: "single-resource",
		Files: map[string]string{
			"main.tf": `resource "null_resource" "A" {}`,
		},
		Response: &proto.Plan_Response{
			Type: &proto.Plan_Response_Complete{
				Complete: &proto.Plan_Complete{
					Resources: []*proto.PlannedResource{{
						Name: "A",
						Type: "null_resource",
					}},
				},
			},
		},
	}, {
		Name: "invalid-sourcecode",
		Files: map[string]string{
			"main.tf": `a`,
		},
		Error: true,
	}, {
		Name: "conditional-single-resource",
		Files: map[string]string{
			"main.tf": `
			variable "test" {
				default = "no"
			}
			resource "null_resource" "A" {
				count = var.test == "yes" ? 1 : 0
			}`,
		},
		Response: &proto.Plan_Response{
			Type: &proto.Plan_Response_Complete{
				Complete: &proto.Plan_Complete{},
			},
		},
	}} {
		testCase := testCase
		t.Run(testCase.Name, func(t *testing.T) {
			t.Parallel()

			directory := t.TempDir()
			for path, content := range testCase.Files {
				err := os.WriteFile(filepath.Join(directory, path), []byte(content), 0600)
				require.NoError(t, err)
			}

			request := &proto.Provision_Request{
				Directory: directory,
			}
			if testCase.Request != nil {
				request.ParameterValues = testCase.Request.ParameterValues
				request.State = testCase.Request.State
			}
			response, err := api.Provision(ctx, request)
			require.NoError(t, err)
			for {
				msg, err := response.Recv()
				if msg != nil && msg.GetLog() != nil {
					t.Logf("log: [%s] %s", msg.GetLog().Level, msg.GetLog().Output)
					continue
				}
				if testCase.Error {
					require.Error(t, err)
					return
				}
				require.NoError(t, err)

				if msg.GetComplete() == nil {
					continue
				}

				require.NoError(t, err)

				resourcesGot, err := json.Marshal(msg.GetComplete().Resources)
				require.NoError(t, err)

				resourcesWant, err := json.Marshal(testCase.Response.GetComplete().Resources)
				require.NoError(t, err)

				require.Equal(t, string(resourcesWant), string(resourcesGot))
				break
			}
		})
	}
}
