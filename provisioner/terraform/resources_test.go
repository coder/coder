package terraform_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	tfjson "github.com/hashicorp/terraform-json"
	"github.com/stretchr/testify/require"
	protobuf "google.golang.org/protobuf/proto"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/testutil"

	"github.com/coder/coder/v2/cryptorand"
	"github.com/coder/coder/v2/provisioner/terraform"
	"github.com/coder/coder/v2/provisionersdk/proto"
)

func ctxAndLogger(t *testing.T) (context.Context, slog.Logger) {
	return context.Background(), testutil.Logger(t)
}

func TestConvertResources(t *testing.T) {
	t.Parallel()
	// nolint:dogsled
	_, filename, _, _ := runtime.Caller(0)
	type testCase struct {
		resources             []*proto.Resource
		parameters            []*proto.RichParameter
		Presets               []*proto.Preset
		externalAuthProviders []*proto.ExternalAuthProviderResource
	}

	// If a user doesn't specify 'display_apps' then they default
	// into all apps except VSCode Insiders.
	displayApps := proto.DisplayApps{
		Vscode:               true,
		VscodeInsiders:       false,
		WebTerminal:          true,
		PortForwardingHelper: true,
		SshHelper:            true,
	}

	// nolint:paralleltest
	for folderName, expected := range map[string]testCase{
		// When a resource depends on another, the shortest route
		// to a resource should always be chosen for the agent.
		"chaining-resources": {
			resources: []*proto.Resource{{
				Name: "a",
				Type: "null_resource",
			}, {
				Name: "b",
				Type: "null_resource",
				Agents: []*proto.Agent{{
					Name:                     "main",
					OperatingSystem:          "linux",
					Architecture:             "amd64",
					Auth:                     &proto.Agent_Token{},
					ConnectionTimeoutSeconds: 120,
					DisplayApps:              &displayApps,
					ResourcesMonitoring:      &proto.ResourcesMonitoring{},
				}},
			}},
		},
		// This can happen when resources hierarchically conflict.
		// When multiple resources exist at the same level, the first
		// listed in state will be chosen.
		"conflicting-resources": {
			resources: []*proto.Resource{{
				Name: "first",
				Type: "null_resource",
				Agents: []*proto.Agent{{
					Name:                     "main",
					OperatingSystem:          "linux",
					Architecture:             "amd64",
					Auth:                     &proto.Agent_Token{},
					ConnectionTimeoutSeconds: 120,
					DisplayApps:              &displayApps,
					ResourcesMonitoring:      &proto.ResourcesMonitoring{},
				}},
			}, {
				Name: "second",
				Type: "null_resource",
			}},
		},
		// Ensures the instance ID authentication type surfaces.
		"instance-id": {
			resources: []*proto.Resource{{
				Name: "main",
				Type: "null_resource",
				Agents: []*proto.Agent{{
					Name:                     "main",
					OperatingSystem:          "linux",
					Architecture:             "amd64",
					Auth:                     &proto.Agent_InstanceId{},
					ConnectionTimeoutSeconds: 120,
					DisplayApps:              &displayApps,
					ResourcesMonitoring:      &proto.ResourcesMonitoring{},
				}},
			}},
		},
		// Ensures that calls to resources through modules work
		// as expected.
		"calling-module": {
			resources: []*proto.Resource{{
				Name: "example",
				Type: "null_resource",
				Agents: []*proto.Agent{{
					Name:                     "main",
					OperatingSystem:          "linux",
					Architecture:             "amd64",
					Auth:                     &proto.Agent_Token{},
					ConnectionTimeoutSeconds: 120,
					DisplayApps:              &displayApps,
					ResourcesMonitoring:      &proto.ResourcesMonitoring{},
				}},
				ModulePath: "module.module",
			}},
		},
		// Ensures the attachment of multiple agents to a single
		// resource is successful.
		"multiple-agents": {
			resources: []*proto.Resource{{
				Name: "dev",
				Type: "null_resource",
				Agents: []*proto.Agent{{
					Name:                     "dev1",
					OperatingSystem:          "linux",
					Architecture:             "amd64",
					Auth:                     &proto.Agent_Token{},
					ConnectionTimeoutSeconds: 120,
					DisplayApps:              &displayApps,
					ResourcesMonitoring:      &proto.ResourcesMonitoring{},
				}, {
					Name:                     "dev2",
					OperatingSystem:          "darwin",
					Architecture:             "amd64",
					Auth:                     &proto.Agent_Token{},
					ConnectionTimeoutSeconds: 1,
					MotdFile:                 "/etc/motd",
					DisplayApps:              &displayApps,
					ResourcesMonitoring:      &proto.ResourcesMonitoring{},
					Scripts: []*proto.Script{{
						Icon:        "/emojis/25c0.png",
						DisplayName: "Shutdown Script",
						RunOnStop:   true,
						LogPath:     "coder-shutdown-script.log",
						Script:      "echo bye bye",
					}},
				}, {
					Name:                     "dev3",
					OperatingSystem:          "windows",
					Architecture:             "arm64",
					Auth:                     &proto.Agent_Token{},
					ConnectionTimeoutSeconds: 120,
					TroubleshootingUrl:       "https://coder.com/troubleshoot",
					DisplayApps:              &displayApps,
					ResourcesMonitoring:      &proto.ResourcesMonitoring{},
				}, {
					Name:                     "dev4",
					OperatingSystem:          "linux",
					Architecture:             "amd64",
					Auth:                     &proto.Agent_Token{},
					ConnectionTimeoutSeconds: 120,
					DisplayApps:              &displayApps,
					ResourcesMonitoring:      &proto.ResourcesMonitoring{},
				}},
			}},
		},
		// Ensures multiple applications can be set for a single agent.
		"multiple-apps": {
			resources: []*proto.Resource{{
				Name: "dev",
				Type: "null_resource",
				Agents: []*proto.Agent{{
					Name:            "dev1",
					OperatingSystem: "linux",
					Architecture:    "amd64",
					Apps: []*proto.App{
						{
							Slug:        "app1",
							DisplayName: "app1",
							// Subdomain defaults to false if unspecified.
							Subdomain: false,
							OpenIn:    proto.AppOpenIn_SLIM_WINDOW,
						},
						{
							Slug:        "app2",
							DisplayName: "app2",
							Subdomain:   true,
							Healthcheck: &proto.Healthcheck{
								Url:       "http://localhost:13337/healthz",
								Interval:  5,
								Threshold: 6,
							},
							OpenIn: proto.AppOpenIn_SLIM_WINDOW,
						},
						{
							Slug:        "app3",
							DisplayName: "app3",
							Subdomain:   false,
							OpenIn:      proto.AppOpenIn_SLIM_WINDOW,
						},
					},
					Auth:                     &proto.Agent_Token{},
					ConnectionTimeoutSeconds: 120,
					DisplayApps:              &displayApps,
					ResourcesMonitoring:      &proto.ResourcesMonitoring{},
				}},
			}},
		},
		"mapped-apps": {
			resources: []*proto.Resource{{
				Name: "dev",
				Type: "null_resource",
				Agents: []*proto.Agent{{
					Name:            "dev",
					OperatingSystem: "linux",
					Architecture:    "amd64",
					Apps: []*proto.App{
						{
							Slug:        "app1",
							DisplayName: "app1",
							OpenIn:      proto.AppOpenIn_SLIM_WINDOW,
						},
						{
							Slug:        "app2",
							DisplayName: "app2",
							OpenIn:      proto.AppOpenIn_SLIM_WINDOW,
						},
					},
					Auth:                     &proto.Agent_Token{},
					ConnectionTimeoutSeconds: 120,
					DisplayApps:              &displayApps,
					ResourcesMonitoring:      &proto.ResourcesMonitoring{},
				}},
			}},
		},
		"multiple-agents-multiple-apps": {
			resources: []*proto.Resource{{
				Name: "dev1",
				Type: "null_resource",
				Agents: []*proto.Agent{{
					Name:            "dev1",
					OperatingSystem: "linux",
					Architecture:    "amd64",
					Apps: []*proto.App{
						{
							Slug:        "app1",
							DisplayName: "app1",
							// Subdomain defaults to false if unspecified.
							Subdomain: false,
							OpenIn:    proto.AppOpenIn_SLIM_WINDOW,
						},
						{
							Slug:        "app2",
							DisplayName: "app2",
							Subdomain:   true,
							Healthcheck: &proto.Healthcheck{
								Url:       "http://localhost:13337/healthz",
								Interval:  5,
								Threshold: 6,
							},
							OpenIn: proto.AppOpenIn_SLIM_WINDOW,
						},
					},
					Auth:                     &proto.Agent_Token{},
					ConnectionTimeoutSeconds: 120,
					DisplayApps:              &displayApps,
					ResourcesMonitoring:      &proto.ResourcesMonitoring{},
				}},
			}, {
				Name: "dev2",
				Type: "null_resource",
				Agents: []*proto.Agent{{
					Name:            "dev2",
					OperatingSystem: "linux",
					Architecture:    "amd64",
					Apps: []*proto.App{
						{
							Slug:        "app3",
							DisplayName: "app3",
							Subdomain:   false,
							OpenIn:      proto.AppOpenIn_SLIM_WINDOW,
						},
					},
					Auth:                     &proto.Agent_Token{},
					ConnectionTimeoutSeconds: 120,
					DisplayApps:              &displayApps,
					ResourcesMonitoring:      &proto.ResourcesMonitoring{},
				}},
			}},
		},
		"multiple-agents-multiple-envs": {
			resources: []*proto.Resource{{
				Name: "dev1",
				Type: "null_resource",
				Agents: []*proto.Agent{{
					Name:            "dev1",
					OperatingSystem: "linux",
					Architecture:    "amd64",
					ExtraEnvs: []*proto.Env{
						{
							Name:  "ENV_1",
							Value: "Env 1",
						},
						{
							Name:  "ENV_2",
							Value: "Env 2",
						},
					},
					Auth:                     &proto.Agent_Token{},
					ConnectionTimeoutSeconds: 120,
					DisplayApps:              &displayApps,
					ResourcesMonitoring:      &proto.ResourcesMonitoring{},
				}},
			}, {
				Name: "dev2",
				Type: "null_resource",
				Agents: []*proto.Agent{{
					Name:            "dev2",
					OperatingSystem: "linux",
					Architecture:    "amd64",
					ExtraEnvs: []*proto.Env{
						{
							Name:  "ENV_3",
							Value: "Env 3",
						},
					},
					Auth:                     &proto.Agent_Token{},
					ConnectionTimeoutSeconds: 120,
					DisplayApps:              &displayApps,
					ResourcesMonitoring:      &proto.ResourcesMonitoring{},
				}},
			}, {
				Name: "env1",
				Type: "coder_env",
			}, {
				Name: "env2",
				Type: "coder_env",
			}, {
				Name: "env3",
				Type: "coder_env",
			}},
		},
		"multiple-agents-multiple-monitors": {
			resources: []*proto.Resource{{
				Name: "dev",
				Type: "null_resource",
				Agents: []*proto.Agent{
					{
						Name:            "dev1",
						OperatingSystem: "linux",
						Architecture:    "amd64",
						Apps: []*proto.App{
							{
								Slug:        "app1",
								DisplayName: "app1",
								// Subdomain defaults to false if unspecified.
								Subdomain: false,
								OpenIn:    proto.AppOpenIn_SLIM_WINDOW,
							},
							{
								Slug:        "app2",
								DisplayName: "app2",
								Subdomain:   true,
								Healthcheck: &proto.Healthcheck{
									Url:       "http://localhost:13337/healthz",
									Interval:  5,
									Threshold: 6,
								},
								OpenIn: proto.AppOpenIn_SLIM_WINDOW,
							},
						},
						Auth:                     &proto.Agent_Token{},
						ConnectionTimeoutSeconds: 120,
						DisplayApps:              &displayApps,
						ResourcesMonitoring: &proto.ResourcesMonitoring{
							Memory: &proto.MemoryResourceMonitor{
								Enabled:   true,
								Threshold: 80,
							},
						},
					},
					{
						Name:                     "dev2",
						OperatingSystem:          "linux",
						Architecture:             "amd64",
						Apps:                     []*proto.App{},
						Auth:                     &proto.Agent_Token{},
						ConnectionTimeoutSeconds: 120,
						DisplayApps:              &displayApps,
						ResourcesMonitoring: &proto.ResourcesMonitoring{
							Memory: &proto.MemoryResourceMonitor{
								Enabled:   true,
								Threshold: 99,
							},
							Volumes: []*proto.VolumeResourceMonitor{
								{
									Path:      "/volume2",
									Enabled:   false,
									Threshold: 50,
								},
								{
									Path:      "/volume1",
									Enabled:   true,
									Threshold: 80,
								},
							},
						},
					},
				},
			}},
		},
		"multiple-agents-multiple-scripts": {
			resources: []*proto.Resource{{
				Name: "dev1",
				Type: "null_resource",
				Agents: []*proto.Agent{{
					Name:            "dev1",
					OperatingSystem: "linux",
					Architecture:    "amd64",
					Scripts: []*proto.Script{
						{
							DisplayName: "Foobar Script 1",
							Script:      "echo foobar 1",
							RunOnStart:  true,
						},
						{
							DisplayName: "Foobar Script 2",
							Script:      "echo foobar 2",
							RunOnStart:  true,
						},
					},
					Auth:                     &proto.Agent_Token{},
					ConnectionTimeoutSeconds: 120,
					DisplayApps:              &displayApps,
					ResourcesMonitoring:      &proto.ResourcesMonitoring{},
				}},
			}, {
				Name: "dev2",
				Type: "null_resource",
				Agents: []*proto.Agent{{
					Name:            "dev2",
					OperatingSystem: "linux",
					Architecture:    "amd64",
					Scripts: []*proto.Script{
						{
							DisplayName: "Foobar Script 3",
							Script:      "echo foobar 3",
							RunOnStart:  true,
						},
					},
					Auth:                     &proto.Agent_Token{},
					ConnectionTimeoutSeconds: 120,
					DisplayApps:              &displayApps,
					ResourcesMonitoring:      &proto.ResourcesMonitoring{},
				}},
			}},
		},
		// Tests fetching metadata about workspace resources.
		"resource-metadata": {
			resources: []*proto.Resource{{
				Name:      "about",
				Type:      "null_resource",
				Hide:      true,
				Icon:      "/icon/server.svg",
				DailyCost: 29,
				Metadata: []*proto.Resource_Metadata{{
					Key:   "hello",
					Value: "world",
				}, {
					Key:    "null",
					IsNull: true,
				}, {
					Key: "empty",
				}, {
					Key:       "secret",
					Value:     "squirrel",
					Sensitive: true,
				}},
				Agents: []*proto.Agent{{
					Name:            "main",
					Auth:            &proto.Agent_Token{},
					OperatingSystem: "linux",
					Architecture:    "amd64",
					Metadata: []*proto.Agent_Metadata{{
						Key:         "process_count",
						DisplayName: "Process Count",
						Script:      "ps -ef | wc -l",
						Interval:    5,
						Timeout:     1,
						Order:       7,
					}},
					ConnectionTimeoutSeconds: 120,
					DisplayApps:              &displayApps,
					ResourcesMonitoring:      &proto.ResourcesMonitoring{},
				}},
			}},
		},
		// Tests that resources with the same id correctly get metadata applied
		// to them.
		"kubernetes-metadata": {
			resources: []*proto.Resource{
				{
					Name: "coder_workspace",
					Type: "kubernetes_config_map",
				}, {
					Name: "coder_workspace",
					Type: "kubernetes_role",
				}, {
					Name: "coder_workspace",
					Type: "kubernetes_role_binding",
				}, {
					Name: "coder_workspace",
					Type: "kubernetes_secret",
				}, {
					Name: "coder_workspace",
					Type: "kubernetes_service_account",
				}, {
					Name: "main",
					Type: "kubernetes_pod",
					Metadata: []*proto.Resource_Metadata{{
						Key:   "cpu",
						Value: "1",
					}, {
						Key:   "memory",
						Value: "1Gi",
					}, {
						Key:   "gpu",
						Value: "1",
					}},
					Agents: []*proto.Agent{{
						Name:            "main",
						OperatingSystem: "linux",
						Architecture:    "amd64",
						Apps: []*proto.App{
							{
								Icon:        "/icon/code.svg",
								Slug:        "code-server",
								DisplayName: "code-server",
								Url:         "http://localhost:13337?folder=/home/coder",
								OpenIn:      proto.AppOpenIn_SLIM_WINDOW,
							},
						},
						Auth:                     &proto.Agent_Token{},
						ConnectionTimeoutSeconds: 120,
						DisplayApps:              &displayApps,
						ResourcesMonitoring:      &proto.ResourcesMonitoring{},
						Scripts: []*proto.Script{{
							DisplayName: "Startup Script",
							RunOnStart:  true,
							LogPath:     "coder-startup-script.log",
							Icon:        "/emojis/25b6.png",
							Script:      "    #!/bin/bash\n    # home folder can be empty, so copying default bash settings\n    if [ ! -f ~/.profile ]; then\n      cp /etc/skel/.profile $HOME\n    fi\n    if [ ! -f ~/.bashrc ]; then\n      cp /etc/skel/.bashrc $HOME\n    fi\n    # install and start code-server\n    curl -fsSL https://code-server.dev/install.sh | sh  | tee code-server-install.log\n    code-server --auth none --port 13337 | tee code-server-install.log &\n",
						}},
					}},
				},
			},
		},
		"rich-parameters": {
			resources: []*proto.Resource{{
				Name: "dev",
				Type: "null_resource",
				Agents: []*proto.Agent{{
					Name:                     "dev",
					OperatingSystem:          "windows",
					Architecture:             "arm64",
					Auth:                     &proto.Agent_Token{},
					ConnectionTimeoutSeconds: 120,
					DisplayApps:              &displayApps,
					ResourcesMonitoring:      &proto.ResourcesMonitoring{},
				}},
			}},
			parameters: []*proto.RichParameter{{
				Name:         "First parameter from child module",
				Type:         "string",
				Description:  "First parameter from child module",
				Mutable:      true,
				DefaultValue: "abcdef",
			}, {
				Name:         "Second parameter from child module",
				Type:         "string",
				Description:  "Second parameter from child module",
				Mutable:      true,
				DefaultValue: "ghijkl",
			}, {
				Name:         "First parameter from module",
				Type:         "string",
				Description:  "First parameter from module",
				Mutable:      true,
				DefaultValue: "abcdef",
			}, {
				Name:         "Second parameter from module",
				Type:         "string",
				Description:  "Second parameter from module",
				Mutable:      true,
				DefaultValue: "ghijkl",
			}, {
				Name: "Example",
				Type: "string",
				Options: []*proto.RichParameterOption{{
					Name:  "First Option",
					Value: "first",
				}, {
					Name:  "Second Option",
					Value: "second",
				}},
				Required: true,
			}, {
				Name:          "number_example",
				Type:          "number",
				DefaultValue:  "4",
				ValidationMin: nil,
				ValidationMax: nil,
			}, {
				Name:          "number_example_max_zero",
				Type:          "number",
				DefaultValue:  "-2",
				ValidationMin: terraform.PtrInt32(-3),
				ValidationMax: terraform.PtrInt32(0),
			}, {
				Name:          "number_example_min_max",
				Type:          "number",
				DefaultValue:  "4",
				ValidationMin: terraform.PtrInt32(3),
				ValidationMax: terraform.PtrInt32(6),
			}, {
				Name:          "number_example_min_zero",
				Type:          "number",
				DefaultValue:  "4",
				ValidationMin: terraform.PtrInt32(0),
				ValidationMax: terraform.PtrInt32(6),
			}, {
				Name:         "Sample",
				Type:         "string",
				Description:  "blah blah",
				DefaultValue: "ok",
			}},
		},
		"rich-parameters-order": {
			resources: []*proto.Resource{{
				Name: "dev",
				Type: "null_resource",
				Agents: []*proto.Agent{{
					Name:                     "dev",
					OperatingSystem:          "windows",
					Architecture:             "arm64",
					Auth:                     &proto.Agent_Token{},
					ConnectionTimeoutSeconds: 120,
					DisplayApps:              &displayApps,
					ResourcesMonitoring:      &proto.ResourcesMonitoring{},
				}},
			}},
			parameters: []*proto.RichParameter{{
				Name:     "Example",
				Type:     "string",
				Required: true,
				Order:    55,
			}, {
				Name:         "Sample",
				Type:         "string",
				Description:  "blah blah",
				DefaultValue: "ok",
				Order:        99,
			}},
		},
		"rich-parameters-validation": {
			resources: []*proto.Resource{{
				Name: "dev",
				Type: "null_resource",
				Agents: []*proto.Agent{{
					Name:                     "dev",
					OperatingSystem:          "windows",
					Architecture:             "arm64",
					Auth:                     &proto.Agent_Token{},
					ConnectionTimeoutSeconds: 120,
					DisplayApps:              &displayApps,
					ResourcesMonitoring:      &proto.ResourcesMonitoring{},
				}},
			}},
			parameters: []*proto.RichParameter{{
				Name:          "number_example",
				Type:          "number",
				DefaultValue:  "4",
				Ephemeral:     true,
				Mutable:       true,
				ValidationMin: nil,
				ValidationMax: nil,
			}, {
				Name:          "number_example_max",
				Type:          "number",
				DefaultValue:  "4",
				ValidationMin: nil,
				ValidationMax: terraform.PtrInt32(6),
			}, {
				Name:          "number_example_max_zero",
				Type:          "number",
				DefaultValue:  "-3",
				ValidationMin: nil,
				ValidationMax: terraform.PtrInt32(0),
			}, {
				Name:          "number_example_min",
				Type:          "number",
				DefaultValue:  "4",
				ValidationMin: terraform.PtrInt32(3),
				ValidationMax: nil,
			}, {
				Name:          "number_example_min_max",
				Type:          "number",
				DefaultValue:  "4",
				ValidationMin: terraform.PtrInt32(3),
				ValidationMax: terraform.PtrInt32(6),
			}, {
				Name:          "number_example_min_zero",
				Type:          "number",
				DefaultValue:  "4",
				ValidationMin: terraform.PtrInt32(0),
				ValidationMax: nil,
			}},
		},
		"external-auth-providers": {
			resources: []*proto.Resource{{
				Name: "dev",
				Type: "null_resource",
				Agents: []*proto.Agent{{
					Name:                     "main",
					OperatingSystem:          "linux",
					Architecture:             "amd64",
					Auth:                     &proto.Agent_Token{},
					ConnectionTimeoutSeconds: 120,
					DisplayApps:              &displayApps,
					ResourcesMonitoring:      &proto.ResourcesMonitoring{},
				}},
			}},
			externalAuthProviders: []*proto.ExternalAuthProviderResource{{Id: "github"}, {Id: "gitlab", Optional: true}},
		},
		"display-apps": {
			resources: []*proto.Resource{{
				Name: "dev",
				Type: "null_resource",
				Agents: []*proto.Agent{{
					Name:                     "main",
					OperatingSystem:          "linux",
					Architecture:             "amd64",
					Auth:                     &proto.Agent_Token{},
					ConnectionTimeoutSeconds: 120,
					DisplayApps: &proto.DisplayApps{
						VscodeInsiders: true,
						WebTerminal:    true,
					},
					ResourcesMonitoring: &proto.ResourcesMonitoring{},
				}},
			}},
		},
		"display-apps-disabled": {
			resources: []*proto.Resource{{
				Name: "dev",
				Type: "null_resource",
				Agents: []*proto.Agent{{
					Name:                     "main",
					OperatingSystem:          "linux",
					Architecture:             "amd64",
					Auth:                     &proto.Agent_Token{},
					ConnectionTimeoutSeconds: 120,
					DisplayApps:              &proto.DisplayApps{},
					ResourcesMonitoring:      &proto.ResourcesMonitoring{},
				}},
			}},
		},
		"presets": {
			resources: []*proto.Resource{{
				Name: "dev",
				Type: "null_resource",
				Agents: []*proto.Agent{{
					Name:                     "dev",
					OperatingSystem:          "windows",
					Architecture:             "arm64",
					Auth:                     &proto.Agent_Token{},
					ConnectionTimeoutSeconds: 120,
					DisplayApps:              &displayApps,
					ResourcesMonitoring:      &proto.ResourcesMonitoring{},
				}},
			}},
			parameters: []*proto.RichParameter{{
				Name:         "First parameter from child module",
				Type:         "string",
				Description:  "First parameter from child module",
				Mutable:      true,
				DefaultValue: "abcdef",
			}, {
				Name:         "Second parameter from child module",
				Type:         "string",
				Description:  "Second parameter from child module",
				Mutable:      true,
				DefaultValue: "ghijkl",
			}, {
				Name:         "First parameter from module",
				Type:         "string",
				Description:  "First parameter from module",
				Mutable:      true,
				DefaultValue: "abcdef",
			}, {
				Name:         "Second parameter from module",
				Type:         "string",
				Description:  "Second parameter from module",
				Mutable:      true,
				DefaultValue: "ghijkl",
			}, {
				Name:         "Sample",
				Type:         "string",
				Description:  "blah blah",
				DefaultValue: "ok",
			}},
			Presets: []*proto.Preset{{
				Name: "My First Project",
				Parameters: []*proto.PresetParameter{{
					Name:  "Sample",
					Value: "A1B2C3",
				}},
			}},
		},
	} {
		folderName := folderName
		expected := expected
		t.Run(folderName, func(t *testing.T) {
			t.Parallel()
			dir := filepath.Join(filepath.Dir(filename), "testdata", folderName)
			t.Run("Plan", func(t *testing.T) {
				t.Parallel()
				ctx, logger := ctxAndLogger(t)

				tfPlanRaw, err := os.ReadFile(filepath.Join(dir, folderName+".tfplan.json"))
				require.NoError(t, err)
				var tfPlan tfjson.Plan
				err = json.Unmarshal(tfPlanRaw, &tfPlan)
				require.NoError(t, err)
				tfPlanGraph, err := os.ReadFile(filepath.Join(dir, folderName+".tfplan.dot"))
				require.NoError(t, err)

				modules := []*tfjson.StateModule{tfPlan.PlannedValues.RootModule}
				if tfPlan.PriorState != nil {
					modules = append(modules, tfPlan.PriorState.Values.RootModule)
				} else {
					// Ensure that resources can be duplicated in the source state
					// and that no errors occur!
					modules = append(modules, tfPlan.PlannedValues.RootModule)
				}
				state, err := terraform.ConvertState(ctx, modules, string(tfPlanGraph), logger)
				require.NoError(t, err)
				sortResources(state.Resources)
				sortExternalAuthProviders(state.ExternalAuthProviders)

				for _, resource := range state.Resources {
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

				expectedNoMetadata := make([]*proto.Resource, 0)
				for _, resource := range expected.resources {
					resourceCopy, _ := protobuf.Clone(resource).(*proto.Resource)
					// plan cannot know whether values are null or not
					for _, metadata := range resourceCopy.Metadata {
						metadata.IsNull = false
					}
					expectedNoMetadata = append(expectedNoMetadata, resourceCopy)
				}

				// Convert expectedNoMetadata and resources into a
				// []map[string]interface{} so they can be compared easily.
				data, err := json.Marshal(expectedNoMetadata)
				require.NoError(t, err)
				var expectedNoMetadataMap []map[string]interface{}
				err = json.Unmarshal(data, &expectedNoMetadataMap)
				require.NoError(t, err)

				data, err = json.Marshal(state.Resources)
				require.NoError(t, err)
				var resourcesMap []map[string]interface{}
				err = json.Unmarshal(data, &resourcesMap)
				require.NoError(t, err)
				if diff := cmp.Diff(expectedNoMetadataMap, resourcesMap); diff != "" {
					require.Failf(t, "unexpected resources", "diff (-want +got):\n%s", diff)
				}

				expectedParams := expected.parameters
				if expectedParams == nil {
					expectedParams = []*proto.RichParameter{}
				}
				parametersWant, err := json.Marshal(expectedParams)
				require.NoError(t, err)
				parametersGot, err := json.Marshal(state.Parameters)
				require.NoError(t, err)
				require.Equal(t, string(parametersWant), string(parametersGot))
				require.Equal(t, expectedNoMetadataMap, resourcesMap)

				require.ElementsMatch(t, expected.externalAuthProviders, state.ExternalAuthProviders)

				require.ElementsMatch(t, expected.Presets, state.Presets)
			})

			t.Run("Provision", func(t *testing.T) {
				t.Parallel()
				ctx, logger := ctxAndLogger(t)
				tfStateRaw, err := os.ReadFile(filepath.Join(dir, folderName+".tfstate.json"))
				require.NoError(t, err)
				var tfState tfjson.State
				err = json.Unmarshal(tfStateRaw, &tfState)
				require.NoError(t, err)
				tfStateGraph, err := os.ReadFile(filepath.Join(dir, folderName+".tfstate.dot"))
				require.NoError(t, err)

				state, err := terraform.ConvertState(ctx, []*tfjson.StateModule{tfState.Values.RootModule}, string(tfStateGraph), logger)
				require.NoError(t, err)
				sortResources(state.Resources)
				sortExternalAuthProviders(state.ExternalAuthProviders)
				for _, resource := range state.Resources {
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
				// Convert expectedNoMetadata and resources into a
				// []map[string]interface{} so they can be compared easily.
				data, err := json.Marshal(expected.resources)
				require.NoError(t, err)
				var expectedMap []map[string]interface{}
				err = json.Unmarshal(data, &expectedMap)
				require.NoError(t, err)

				data, err = json.Marshal(state.Resources)
				require.NoError(t, err)
				var resourcesMap []map[string]interface{}
				err = json.Unmarshal(data, &resourcesMap)
				require.NoError(t, err)
				if diff := cmp.Diff(expectedMap, resourcesMap); diff != "" {
					require.Failf(t, "unexpected resources", "diff (-want +got):\n%s", diff)
				}
				require.ElementsMatch(t, expected.externalAuthProviders, state.ExternalAuthProviders)

				require.ElementsMatch(t, expected.Presets, state.Presets)
			})
		})
	}
}

func TestInvalidTerraformAddress(t *testing.T) {
	t.Parallel()
	ctx, logger := context.Background(), slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	state, err := terraform.ConvertState(ctx, []*tfjson.StateModule{{
		Resources: []*tfjson.StateResource{{
			Address:         "invalid",
			Type:            "invalid",
			Name:            "invalid",
			Mode:            tfjson.ManagedResourceMode,
			AttributeValues: map[string]interface{}{},
		}},
	}}, `digraph {}`, logger)
	require.Nil(t, err)
	require.Len(t, state.Resources, 1)
	require.Equal(t, state.Resources[0].Name, "invalid")
	require.Equal(t, state.Resources[0].ModulePath, "invalid terraform address")
}

//nolint:tparallel
func TestAppSlugValidation(t *testing.T) {
	t.Parallel()
	ctx, logger := ctxAndLogger(t)

	// nolint:dogsled
	_, filename, _, _ := runtime.Caller(0)

	// Load the multiple-apps state file and edit it.
	dir := filepath.Join(filepath.Dir(filename), "testdata", "multiple-apps")
	tfPlanRaw, err := os.ReadFile(filepath.Join(dir, "multiple-apps.tfplan.json"))
	require.NoError(t, err)
	var tfPlan tfjson.Plan
	err = json.Unmarshal(tfPlanRaw, &tfPlan)
	require.NoError(t, err)
	tfPlanGraph, err := os.ReadFile(filepath.Join(dir, "multiple-apps.tfplan.dot"))
	require.NoError(t, err)

	cases := []struct {
		slug        string
		errContains string
	}{
		{slug: "$$$ invalid slug $$$", errContains: "does not match regex"},
		{slug: "invalid--slug", errContains: "does not match regex"},
		{slug: "invalid_slug", errContains: "does not match regex"},
		{slug: "Invalid-slug", errContains: "does not match regex"},
		{slug: "valid", errContains: ""},
	}

	//nolint:paralleltest
	for i, c := range cases {
		c := c
		t.Run(fmt.Sprintf("case-%d", i), func(t *testing.T) {
			// Change the first app slug to match the current case.
			for _, resource := range tfPlan.PlannedValues.RootModule.Resources {
				if resource.Type == "coder_app" {
					resource.AttributeValues["slug"] = c.slug
					break
				}
			}

			_, err := terraform.ConvertState(ctx, []*tfjson.StateModule{tfPlan.PlannedValues.RootModule}, string(tfPlanGraph), logger)
			if c.errContains != "" {
				require.ErrorContains(t, err, c.errContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestAppSlugDuplicate(t *testing.T) {
	t.Parallel()
	ctx, logger := ctxAndLogger(t)

	// nolint:dogsled
	_, filename, _, _ := runtime.Caller(0)

	dir := filepath.Join(filepath.Dir(filename), "testdata", "multiple-apps")
	tfPlanRaw, err := os.ReadFile(filepath.Join(dir, "multiple-apps.tfplan.json"))
	require.NoError(t, err)
	var tfPlan tfjson.Plan
	err = json.Unmarshal(tfPlanRaw, &tfPlan)
	require.NoError(t, err)
	tfPlanGraph, err := os.ReadFile(filepath.Join(dir, "multiple-apps.tfplan.dot"))
	require.NoError(t, err)

	for _, resource := range tfPlan.PlannedValues.RootModule.Resources {
		if resource.Type == "coder_app" {
			resource.AttributeValues["slug"] = "dev"
		}
	}

	_, err = terraform.ConvertState(ctx, []*tfjson.StateModule{tfPlan.PlannedValues.RootModule}, string(tfPlanGraph), logger)
	require.Error(t, err)
	require.ErrorContains(t, err, "duplicate app slug")
}

//nolint:tparallel
func TestAgentNameInvalid(t *testing.T) {
	t.Parallel()
	ctx, logger := ctxAndLogger(t)

	// nolint:dogsled
	_, filename, _, _ := runtime.Caller(0)

	dir := filepath.Join(filepath.Dir(filename), "testdata", "multiple-agents")
	tfPlanRaw, err := os.ReadFile(filepath.Join(dir, "multiple-agents.tfplan.json"))
	require.NoError(t, err)
	var tfPlan tfjson.Plan
	err = json.Unmarshal(tfPlanRaw, &tfPlan)
	require.NoError(t, err)
	tfPlanGraph, err := os.ReadFile(filepath.Join(dir, "multiple-agents.tfplan.dot"))
	require.NoError(t, err)

	cases := []struct {
		name        string
		errContains string
	}{
		{name: "bad--name", errContains: "does not match regex"},
		{name: "bad_name", errContains: "contains underscores"}, // custom error for underscores
		{name: "valid-name-123", errContains: ""},
		{name: "valid", errContains: ""},
		{name: "UppercaseValid", errContains: ""},
	}

	//nolint:paralleltest
	for i, c := range cases {
		c := c
		t.Run(fmt.Sprintf("case-%d", i), func(t *testing.T) {
			// Change the first agent name to match the current case.
			for _, resource := range tfPlan.PlannedValues.RootModule.Resources {
				if resource.Type == "coder_agent" {
					resource.Name = c.name
					break
				}
			}

			_, err := terraform.ConvertState(ctx, []*tfjson.StateModule{tfPlan.PlannedValues.RootModule}, string(tfPlanGraph), logger)
			if c.errContains != "" {
				require.ErrorContains(t, err, c.errContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestAgentNameDuplicate(t *testing.T) {
	t.Parallel()
	ctx, logger := ctxAndLogger(t)

	// nolint:dogsled
	_, filename, _, _ := runtime.Caller(0)

	dir := filepath.Join(filepath.Dir(filename), "testdata", "multiple-agents")
	tfPlanRaw, err := os.ReadFile(filepath.Join(dir, "multiple-agents.tfplan.json"))
	require.NoError(t, err)
	var tfPlan tfjson.Plan
	err = json.Unmarshal(tfPlanRaw, &tfPlan)
	require.NoError(t, err)
	tfPlanGraph, err := os.ReadFile(filepath.Join(dir, "multiple-agents.tfplan.dot"))
	require.NoError(t, err)

	for _, resource := range tfPlan.PlannedValues.RootModule.Resources {
		if resource.Type == "coder_agent" {
			switch resource.Name {
			case "dev1":
				resource.Name = "dev"
			case "dev2":
				resource.Name = "Dev"
			}
		}
	}

	state, err := terraform.ConvertState(ctx, []*tfjson.StateModule{tfPlan.PlannedValues.RootModule}, string(tfPlanGraph), logger)
	require.Nil(t, state)
	require.Error(t, err)
	require.ErrorContains(t, err, "duplicate agent name")
}

func TestMetadataResourceDuplicate(t *testing.T) {
	t.Parallel()
	ctx, logger := ctxAndLogger(t)

	// Load the multiple-apps state file and edit it.
	dir := filepath.Join("testdata", "resource-metadata-duplicate")
	tfPlanRaw, err := os.ReadFile(filepath.Join(dir, "resource-metadata-duplicate.tfplan.json"))
	require.NoError(t, err)
	var tfPlan tfjson.Plan
	err = json.Unmarshal(tfPlanRaw, &tfPlan)
	require.NoError(t, err)
	tfPlanGraph, err := os.ReadFile(filepath.Join(dir, "resource-metadata-duplicate.tfplan.dot"))
	require.NoError(t, err)

	state, err := terraform.ConvertState(ctx, []*tfjson.StateModule{tfPlan.PlannedValues.RootModule}, string(tfPlanGraph), logger)
	require.Nil(t, state)
	require.Error(t, err)
	require.ErrorContains(t, err, "duplicate metadata resource: null_resource.about")
}

func TestParameterValidation(t *testing.T) {
	t.Parallel()
	ctx, logger := ctxAndLogger(t)

	// nolint:dogsled
	_, filename, _, _ := runtime.Caller(0)

	// Load the rich-parameters state file and edit it.
	dir := filepath.Join(filepath.Dir(filename), "testdata", "rich-parameters")
	tfPlanRaw, err := os.ReadFile(filepath.Join(dir, "rich-parameters.tfplan.json"))
	require.NoError(t, err)
	var tfPlan tfjson.Plan
	err = json.Unmarshal(tfPlanRaw, &tfPlan)
	require.NoError(t, err)
	tfPlanGraph, err := os.ReadFile(filepath.Join(dir, "rich-parameters.tfplan.dot"))
	require.NoError(t, err)

	// Change all names to be identical.
	var names []string
	for _, resource := range tfPlan.PriorState.Values.RootModule.Resources {
		if resource.Type == "coder_parameter" {
			resource.AttributeValues["name"] = "identical"
			names = append(names, resource.Name)
		}
	}

	state, err := terraform.ConvertState(ctx, []*tfjson.StateModule{tfPlan.PriorState.Values.RootModule}, string(tfPlanGraph), logger)
	require.Nil(t, state)
	require.Error(t, err)
	require.ErrorContains(t, err, "coder_parameter names must be unique but \"identical\" appears multiple times")

	// Make two sets of identical names.
	count := 0
	names = nil
	for _, resource := range tfPlan.PriorState.Values.RootModule.Resources {
		if resource.Type == "coder_parameter" {
			resource.AttributeValues["name"] = fmt.Sprintf("identical-%d", count%2)
			names = append(names, resource.Name)
			count++
		}
	}

	state, err = terraform.ConvertState(ctx, []*tfjson.StateModule{tfPlan.PriorState.Values.RootModule}, string(tfPlanGraph), logger)
	require.Nil(t, state)
	require.Error(t, err)
	require.ErrorContains(t, err, "coder_parameter names must be unique but \"identical-0\" and \"identical-1\" appear multiple times")

	// Once more with three sets.
	count = 0
	names = nil
	for _, resource := range tfPlan.PriorState.Values.RootModule.Resources {
		if resource.Type == "coder_parameter" {
			resource.AttributeValues["name"] = fmt.Sprintf("identical-%d", count%3)
			names = append(names, resource.Name)
			count++
		}
	}

	state, err = terraform.ConvertState(ctx, []*tfjson.StateModule{tfPlan.PriorState.Values.RootModule}, string(tfPlanGraph), logger)
	require.Nil(t, state)
	require.Error(t, err)
	require.ErrorContains(t, err, "coder_parameter names must be unique but \"identical-0\", \"identical-1\" and \"identical-2\" appear multiple times")
}

func TestInstanceTypeAssociation(t *testing.T) {
	t.Parallel()
	type tc struct {
		ResourceType    string
		InstanceTypeKey string
	}
	for _, tc := range []tc{{
		ResourceType:    "google_compute_instance",
		InstanceTypeKey: "machine_type",
	}, {
		ResourceType:    "aws_instance",
		InstanceTypeKey: "instance_type",
	}, {
		ResourceType:    "aws_spot_instance_request",
		InstanceTypeKey: "instance_type",
	}, {
		ResourceType:    "azurerm_linux_virtual_machine",
		InstanceTypeKey: "size",
	}, {
		ResourceType:    "azurerm_windows_virtual_machine",
		InstanceTypeKey: "size",
	}} {
		tc := tc
		t.Run(tc.ResourceType, func(t *testing.T) {
			t.Parallel()
			ctx, logger := ctxAndLogger(t)
			instanceType, err := cryptorand.String(12)
			require.NoError(t, err)
			state, err := terraform.ConvertState(ctx, []*tfjson.StateModule{{
				Resources: []*tfjson.StateResource{{
					Address: tc.ResourceType + ".dev",
					Type:    tc.ResourceType,
					Name:    "dev",
					Mode:    tfjson.ManagedResourceMode,
					AttributeValues: map[string]interface{}{
						tc.InstanceTypeKey: instanceType,
					},
				}},
				// This is manually created to join the edges.
			}}, `digraph {
	compound = "true"
	newrank = "true"
	subgraph "root" {
		"[root] `+tc.ResourceType+`.dev" [label = "`+tc.ResourceType+`.dev", shape = "box"]
	}
}`, logger)
			require.NoError(t, err)
			require.Len(t, state.Resources, 1)
			require.Equal(t, state.Resources[0].GetInstanceType(), instanceType)
		})
	}
}

func TestInstanceIDAssociation(t *testing.T) {
	t.Parallel()
	type tc struct {
		Auth          string
		ResourceType  string
		InstanceIDKey string
	}
	for _, tc := range []tc{{
		Auth:          "google-instance-identity",
		ResourceType:  "google_compute_instance",
		InstanceIDKey: "instance_id",
	}, {
		Auth:          "aws-instance-identity",
		ResourceType:  "aws_instance",
		InstanceIDKey: "id",
	}, {
		Auth:          "aws-instance-identity",
		ResourceType:  "aws_spot_instance_request",
		InstanceIDKey: "spot_instance_id",
	}, {
		Auth:          "azure-instance-identity",
		ResourceType:  "azurerm_linux_virtual_machine",
		InstanceIDKey: "virtual_machine_id",
	}, {
		Auth:          "azure-instance-identity",
		ResourceType:  "azurerm_windows_virtual_machine",
		InstanceIDKey: "virtual_machine_id",
	}} {
		tc := tc
		t.Run(tc.ResourceType, func(t *testing.T) {
			t.Parallel()
			ctx, logger := ctxAndLogger(t)
			instanceID, err := cryptorand.String(12)
			require.NoError(t, err)
			state, err := terraform.ConvertState(ctx, []*tfjson.StateModule{{
				Resources: []*tfjson.StateResource{{
					Address: "coder_agent.dev",
					Type:    "coder_agent",
					Name:    "dev",
					Mode:    tfjson.ManagedResourceMode,
					AttributeValues: map[string]interface{}{
						"arch": "amd64",
						"auth": tc.Auth,
					},
				}, {
					Address:   tc.ResourceType + ".dev",
					Type:      tc.ResourceType,
					Name:      "dev",
					Mode:      tfjson.ManagedResourceMode,
					DependsOn: []string{"coder_agent.dev"},
					AttributeValues: map[string]interface{}{
						tc.InstanceIDKey: instanceID,
					},
				}},
				// This is manually created to join the edges.
			}}, `digraph {
	compound = "true"
	newrank = "true"
	subgraph "root" {
		"[root] coder_agent.dev" [label = "coder_agent.dev", shape = "box"]
		"[root] `+tc.ResourceType+`.dev" [label = "`+tc.ResourceType+`.dev", shape = "box"]
		"[root] `+tc.ResourceType+`.dev" -> "[root] coder_agent.dev"
	}
}
`, logger)
			require.NoError(t, err)
			require.Len(t, state.Resources, 1)
			require.Len(t, state.Resources[0].Agents, 1)
			require.Equal(t, state.Resources[0].Agents[0].GetInstanceId(), instanceID)
		})
	}
}

// sortResource ensures resources appear in a consistent ordering
// to prevent tests from flaking.
func sortResources(resources []*proto.Resource) {
	sort.Slice(resources, func(i, j int) bool {
		if resources[i].Name != resources[j].Name {
			return resources[i].Name < resources[j].Name
		}
		return resources[i].Type < resources[j].Type
	})
	for _, resource := range resources {
		for _, agent := range resource.Agents {
			sort.Slice(agent.Apps, func(i, j int) bool {
				return agent.Apps[i].Slug < agent.Apps[j].Slug
			})
			sort.Slice(agent.ExtraEnvs, func(i, j int) bool {
				return agent.ExtraEnvs[i].Name < agent.ExtraEnvs[j].Name
			})
			sort.Slice(agent.Scripts, func(i, j int) bool {
				return agent.Scripts[i].DisplayName < agent.Scripts[j].DisplayName
			})
		}
		sort.Slice(resource.Agents, func(i, j int) bool {
			return resource.Agents[i].Name < resource.Agents[j].Name
		})
	}
}

func sortExternalAuthProviders(providers []*proto.ExternalAuthProviderResource) {
	sort.Slice(providers, func(i, j int) bool {
		return strings.Compare(providers[i].Id, providers[j].Id) == -1
	})
}
