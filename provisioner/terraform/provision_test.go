//go:build linux

package terraform_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/provisioner/terraform"
	"github.com/coder/coder/provisionersdk"
	"github.com/coder/coder/provisionersdk/proto"
)

func TestProvision(t *testing.T) {
	t.Parallel()

	provider := `
terraform {
	required_providers {
		coder = {
			source = "coder/coder"
			version = "0.1.0"
		}
	}
}

provider "coder" {
}
	`
	t.Log(provider)

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
		Request  *proto.Provision_Request
		Response *proto.Provision_Response
		Error    bool
	}{{
		Name: "single-variable",
		Files: map[string]string{
			"main.tf": `variable "A" {
				description = "Testing!"
			}`,
		},
		Request: &proto.Provision_Request{
			Type: &proto.Provision_Request_Start{
				Start: &proto.Provision_Start{
					ParameterValues: []*proto.ParameterValue{{
						DestinationScheme: proto.ParameterDestination_PROVISIONER_VARIABLE,
						Name:              "A",
						Value:             "example",
					}},
				},
			},
		},
		Response: &proto.Provision_Response{
			Type: &proto.Provision_Response_Complete{
				Complete: &proto.Provision_Complete{},
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
		Response: &proto.Provision_Response{
			Type: &proto.Provision_Response_Complete{
				Complete: &proto.Provision_Complete{
					Resources: []*proto.Resource{{
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
		Name: "dryrun-single-resource",
		Files: map[string]string{
			"main.tf": `resource "null_resource" "A" {}`,
		},
		Request: &proto.Provision_Request{
			Type: &proto.Provision_Request_Start{
				Start: &proto.Provision_Start{
					DryRun: true,
				},
			},
		},
		Response: &proto.Provision_Response{
			Type: &proto.Provision_Response_Complete{
				Complete: &proto.Provision_Complete{
					Resources: []*proto.Resource{{
						Name: "A",
						Type: "null_resource",
					}},
				},
			},
		},
	}, {
		Name: "dryrun-conditional-single-resource",
		Files: map[string]string{
			"main.tf": `
			variable "test" {
				default = "no"
			}
			resource "null_resource" "A" {
				count = var.test == "yes" ? 1 : 0
			}`,
		},
		Response: &proto.Provision_Response{
			Type: &proto.Provision_Response_Complete{
				Complete: &proto.Provision_Complete{
					Resources: nil,
				},
			},
		},
	}, {
		Name: "resource-associated-with-agent",
		Files: map[string]string{
			"main.tf": provider + `
			resource "coder_agent" "A" {}
			resource "null_resource" "A" {
				depends_on = [
					coder_agent.A
				]
			}`,
		},
		Request: &proto.Provision_Request{
			Type: &proto.Provision_Request_Start{
				Start: &proto.Provision_Start{
					Metadata: &proto.Provision_Metadata{
						CoderUrl: "https://example.com",
					},
				},
			},
		},
		Response: &proto.Provision_Response{
			Type: &proto.Provision_Response_Complete{
				Complete: &proto.Provision_Complete{
					Resources: []*proto.Resource{{
						Name: "A",
						Type: "null_resource",
						Agent: &proto.Agent{
							Auth: &proto.Agent_Token{
								Token: "",
							},
						},
					}},
				},
			},
		},
	}, {
		Name: "agent-associated-with-resource",
		Files: map[string]string{
			"main.tf": provider + `
			resource "coder_agent" "A" {
				depends_on = [
					null_resource.A
				]
				auth {
					type = "google-instance-identity"
					instance_id = "an-instance"
				}
			}
			resource "null_resource" "A" {}`,
		},
		Request: &proto.Provision_Request{
			Type: &proto.Provision_Request_Start{
				Start: &proto.Provision_Start{
					Metadata: &proto.Provision_Metadata{
						CoderUrl: "https://example.com",
					},
				},
			},
		},
		Response: &proto.Provision_Response{
			Type: &proto.Provision_Response_Complete{
				Complete: &proto.Provision_Complete{
					Resources: []*proto.Resource{{
						Name: "A",
						Type: "null_resource",
					}},
				},
			},
		},
	}, {
		Name: "dryrun-resource-associated-with-agent",
		Files: map[string]string{
			"main.tf": provider + `
			resource "coder_agent" "A" {
				count = 1
			}
			resource "null_resource" "A" {
				count = length(coder_agent.A)
			}`,
		},
		Request: &proto.Provision_Request{
			Type: &proto.Provision_Request_Start{
				Start: &proto.Provision_Start{
					DryRun: true,
					Metadata: &proto.Provision_Metadata{
						CoderUrl: "https://example.com",
					},
				},
			},
		},
		Response: &proto.Provision_Response{
			Type: &proto.Provision_Response_Complete{
				Complete: &proto.Provision_Complete{
					Resources: []*proto.Resource{{
						Name: "A",
						Type: "null_resource",
						Agent: &proto.Agent{
							Auth: &proto.Agent_Token{},
						},
					}},
				},
			},
		},
	}, {
		Name: "dryrun-agent-associated-with-resource",
		Files: map[string]string{
			"main.tf": provider + `
			resource "coder_agent" "A" {
				count = length(null_resource.A)
				auth {
					type = "google-instance-identity"
					instance_id = "an-instance"
				}
			}
			resource "null_resource" "A" {
				count = 1
			}`,
		},
		Request: &proto.Provision_Request{
			Type: &proto.Provision_Request_Start{
				Start: &proto.Provision_Start{
					DryRun: true,
					Metadata: &proto.Provision_Metadata{
						CoderUrl: "https://example.com",
					},
				},
			},
		},
		Response: &proto.Provision_Response{
			Type: &proto.Provision_Response_Complete{
				Complete: &proto.Provision_Complete{
					Resources: []*proto.Resource{{
						Name: "A",
						Type: "null_resource",
						Agent: &proto.Agent{
							Auth: &proto.Agent_GoogleInstanceIdentity{
								GoogleInstanceIdentity: &proto.GoogleInstanceIdentityAuth{
									InstanceId: "an-instance",
								},
							},
						},
					}},
				},
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
				Type: &proto.Provision_Request_Start{
					Start: &proto.Provision_Start{
						Directory: directory,
					},
				},
			}
			if testCase.Request != nil {
				request.GetStart().ParameterValues = testCase.Request.GetStart().ParameterValues
				request.GetStart().State = testCase.Request.GetStart().State
				request.GetStart().DryRun = testCase.Request.GetStart().DryRun
				request.GetStart().Metadata = testCase.Request.GetStart().Metadata
			}
			if request.GetStart().Metadata == nil {
				request.GetStart().Metadata = &proto.Provision_Metadata{}
			}
			response, err := api.Provision(ctx)
			require.NoError(t, err)
			err = response.Send(request)
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
				if !request.GetStart().DryRun {
					require.Greater(t, len(msg.GetComplete().State), 0)
				}

				// Remove randomly generated data.
				for _, resource := range msg.GetComplete().Resources {
					if resource.Agent == nil {
						continue
					}
					resource.Agent.Id = ""
					if resource.Agent.GetToken() == "" {
						continue
					}
					resource.Agent.Auth = &proto.Agent_Token{
						Token: "",
					}
				}

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
