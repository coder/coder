package pulumi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/testing/protocmp"

	"github.com/coder/coder/v2/provisioner/pulumi"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

const (
	stackURN        = "urn:pulumi:coder::project::pulumi:pulumi:Stack::project"
	providerURN     = "urn:pulumi:coder::project::pulumi:providers:coder::default"
	containerURN    = "urn:pulumi:coder::project::docker:index/container:Container::workspace"
	agentURN        = "urn:pulumi:coder::project::coder:index/agent:Agent::dev"
	appURN          = "urn:pulumi:coder::project::coder:index/app:App::editor"
	scriptURN       = "urn:pulumi:coder::project::coder:index/script:Script::startup"
	envURN          = "urn:pulumi:coder::project::coder:index/env:Env::workspace-env"
	metadataURN     = "urn:pulumi:coder::project::coder:index/metadata:Metadata::workspace-metadata"
	parameterURN    = "urn:pulumi:coder::project::coder:index/parameter:Parameter::region"
	presetURN       = "urn:pulumi:coder::project::coder:index/workspacePreset:WorkspacePreset::fast-start"
	externalAuthURN = "urn:pulumi:coder::project::coder:index/externalAuth:ExternalAuth::github"
	aiTaskURN       = "urn:pulumi:coder::project::coder:index/aiTask:AiTask::sidebar-task"
)

type keyStyle string

const (
	camelKeyStyle keyStyle = "camel"
	snakeKeyStyle keyStyle = "snake"
)

type comparableState struct {
	Resources             []*proto.Resource
	Parameters            []*proto.RichParameter
	Presets               []*proto.Preset
	ExternalAuthProviders []*proto.ExternalAuthProviderResource
	AITasks               []*proto.AITask
	HasAITasks            bool
	HasExternalAgents     bool
}

func TestConvertState(t *testing.T) {
	t.Parallel()

	t.Run("HappyPathMixedExport", func(t *testing.T) {
		t.Parallel()

		state := convertState(t, makeStackExport(
			mustJSON(t, map[string]any{
				"urn":    stackURN,
				"type":   "pulumi:pulumi:Stack",
				"custom": false,
			}),
			mustJSON(t, map[string]any{
				"urn":    providerURN,
				"type":   "pulumi:providers:coder",
				"custom": true,
			}),
			mustJSON(t, map[string]any{
				"urn":    containerURN,
				"type":   "docker:index/container:Container",
				"custom": true,
				"outputs": map[string]any{
					"id": "container-1",
				},
			}),
			mustJSON(t, map[string]any{
				"urn":          agentURN,
				"type":         "coder:index/agent:Agent",
				"custom":       true,
				"dependencies": []string{containerURN},
				"outputs": map[string]any{
					"id":                "agent-1",
					"name":              "dev",
					"auth":              "token",
					"token":             "agent-token",
					"os":                "linux",
					"arch":              "amd64",
					"dir":               "/workspace",
					"connectionTimeout": 42,
					"motdFile":          "/etc/motd",
					"apiKeyScope":       "all",
					"order":             7,
					"env": map[string]any{
						"HOME": "/workspace",
					},
					"metadata": []map[string]any{{
						"key":         "cpu",
						"displayName": "CPU",
						"script":      "echo 1",
						"interval":    10,
						"timeout":     2,
						"order":       1,
					}},
					"displayApps": map[string]any{
						"vscode":               true,
						"vscodeInsiders":       true,
						"webTerminal":          false,
						"sshHelper":            true,
						"portForwardingHelper": false,
					},
					"resourcesMonitoring": map[string]any{
						"memory": map[string]any{
							"enabled":   true,
							"threshold": 80,
						},
						"volumes": []map[string]any{{
							"path":      "/workspace",
							"enabled":   true,
							"threshold": 90,
						}},
					},
				},
			}),
			mustJSON(t, map[string]any{
				"urn":    appURN,
				"type":   "coder:index/app:App",
				"custom": true,
				"outputs": map[string]any{
					"id":          "app-1",
					"agentId":     "agent-1",
					"slug":        "editor",
					"displayName": "Editor",
					"url":         "https://editor.example.com",
					"icon":        "code",
					"command":     "code-server",
					"share":       "authenticated",
					"subdomain":   true,
					"order":       2,
					"group":       "tools",
					"hidden":      true,
					"openIn":      "tab",
					"healthcheck": map[string]any{
						"url":       "https://editor.example.com/healthz",
						"interval":  5,
						"threshold": 3,
					},
				},
			}),
			mustJSON(t, map[string]any{
				"urn":    scriptURN,
				"type":   "coder:index/script:Script",
				"custom": true,
				"outputs": map[string]any{
					"agentId":          "agent-1",
					"displayName":      "Startup",
					"icon":             "terminal",
					"script":           "echo ready",
					"cron":             "0 0 * * *",
					"startBlocksLogin": true,
					"runOnStart":       true,
					"runOnStop":        false,
					"timeout":          15,
					"logPath":          "/tmp/startup.log",
				},
			}),
			mustJSON(t, map[string]any{
				"urn":    envURN,
				"type":   "coder:index/env:Env",
				"custom": true,
				"outputs": map[string]any{
					"agentId":       "agent-1",
					"name":          "WORKSPACE_NAME",
					"value":         "coder",
					"mergeStrategy": "append",
				},
			}),
			mustJSON(t, map[string]any{
				"urn":    metadataURN,
				"type":   "coder:index/metadata:Metadata",
				"custom": true,
				"outputs": map[string]any{
					"resourceId": "container-1",
					"hide":       true,
					"icon":       "docker",
					"dailyCost":  99,
					"items": []map[string]any{{
						"key":   "image",
						"value": "coder/workspace:latest",
					}, {
						"key":       "secret",
						"value":     "hidden",
						"sensitive": true,
						"isNull":    true,
					}},
				},
			}),
			mustJSON(t, map[string]any{
				"urn":    parameterURN,
				"type":   "coder:index/parameter:Parameter",
				"custom": true,
				"outputs": map[string]any{
					"name":        "region",
					"displayName": "Region",
					"description": "Cloud region",
					"type":        "string",
					"default":     "us-east-1",
					"formType":    "dropdown",
					"options": []map[string]any{{
						"name":  "US East 1",
						"value": "us-east-1",
					}, {
						"name":  "US West 2",
						"value": "us-west-2",
					}},
				},
			}),
			mustJSON(t, map[string]any{
				"urn":    presetURN,
				"type":   "coder:index/workspacePreset:WorkspacePreset",
				"custom": true,
				"outputs": map[string]any{
					"name":        "fast-start",
					"description": "Warm workspace",
					"parameters": map[string]any{
						"region": "us-east-1",
					},
					"prebuilds": []map[string]any{{
						"instances": 2,
						"expirationPolicy": map[string]any{
							"ttl": 3600,
						},
					}},
				},
			}),
			mustJSON(t, map[string]any{
				"urn":    externalAuthURN,
				"type":   "coder:index/externalAuth:ExternalAuth",
				"custom": true,
				"outputs": map[string]any{
					"id":       "github",
					"optional": true,
				},
			}),
			mustJSON(t, map[string]any{
				"urn":    aiTaskURN,
				"type":   "coder:index/aiTask:AiTask",
				"custom": true,
				"outputs": map[string]any{
					"id": "task-1",
					"sidebarApp": map[string]any{
						"id": "editor",
					},
				},
			}),
		))

		require.Len(t, state.Resources, 1)
		resource := state.Resources[0]
		require.Equal(t, "workspace", resource.Name)
		require.Equal(t, "docker:index/container:Container", resource.Type)
		require.True(t, resource.Hide)
		require.Equal(t, "docker", resource.Icon)
		require.EqualValues(t, 99, resource.DailyCost)
		require.Len(t, resource.Metadata, 2)
		require.Equal(t, "image", resource.Metadata[0].Key)
		require.Equal(t, "coder/workspace:latest", resource.Metadata[0].Value)
		require.Equal(t, "secret", resource.Metadata[1].Key)
		require.True(t, resource.Metadata[1].Sensitive)
		require.True(t, resource.Metadata[1].IsNull)

		require.Len(t, resource.Agents, 1)
		agent := resource.Agents[0]
		require.Equal(t, "agent-1", agent.Id)
		require.Equal(t, "dev", agent.Name)
		require.Equal(t, "linux", agent.OperatingSystem)
		require.Equal(t, "amd64", agent.Architecture)
		require.Equal(t, "/workspace", agent.Directory)
		require.Equal(t, "agent-token", agent.GetToken())
		require.EqualValues(t, 42, agent.ConnectionTimeoutSeconds)
		require.Equal(t, "/etc/motd", agent.MotdFile)
		require.Equal(t, "all", agent.ApiKeyScope)
		require.EqualValues(t, 7, agent.Order)
		require.Equal(t, map[string]string{"HOME": "/workspace"}, agent.Env)
		require.Len(t, agent.Metadata, 1)
		require.Equal(t, "CPU", agent.Metadata[0].DisplayName)
		require.NotNil(t, agent.DisplayApps)
		require.True(t, agent.DisplayApps.Vscode)
		require.True(t, agent.DisplayApps.VscodeInsiders)
		require.False(t, agent.DisplayApps.WebTerminal)
		require.True(t, agent.DisplayApps.SshHelper)
		require.False(t, agent.DisplayApps.PortForwardingHelper)
		require.NotNil(t, agent.ResourcesMonitoring)
		require.NotNil(t, agent.ResourcesMonitoring.Memory)
		require.True(t, agent.ResourcesMonitoring.Memory.Enabled)
		require.EqualValues(t, 80, agent.ResourcesMonitoring.Memory.Threshold)
		require.Len(t, agent.ResourcesMonitoring.Volumes, 1)
		require.Equal(t, "/workspace", agent.ResourcesMonitoring.Volumes[0].Path)

		require.Len(t, agent.Apps, 1)
		app := agent.Apps[0]
		require.Equal(t, "editor", app.Slug)
		require.Equal(t, "Editor", app.DisplayName)
		require.Equal(t, "https://editor.example.com", app.Url)
		require.Equal(t, proto.AppSharingLevel_AUTHENTICATED, app.SharingLevel)
		require.Equal(t, proto.AppOpenIn_TAB, app.OpenIn)
		require.True(t, app.Hidden)
		require.True(t, app.Subdomain)
		require.NotNil(t, app.Healthcheck)
		require.Equal(t, "https://editor.example.com/healthz", app.Healthcheck.Url)

		require.Len(t, agent.ExtraEnvs, 1)
		require.Equal(t, "WORKSPACE_NAME", agent.ExtraEnvs[0].Name)
		require.Equal(t, "append", agent.ExtraEnvs[0].MergeStrategy)

		require.Len(t, agent.Scripts, 1)
		require.Equal(t, "Startup", agent.Scripts[0].DisplayName)
		require.EqualValues(t, 15, agent.Scripts[0].TimeoutSeconds)

		require.Len(t, state.Parameters, 1)
		parameter := state.Parameters[0]
		require.Equal(t, "region", parameter.Name)
		require.Equal(t, "Region", parameter.DisplayName)
		require.Equal(t, "Cloud region", parameter.Description)
		require.Equal(t, "string", parameter.Type)
		require.Equal(t, "us-east-1", parameter.DefaultValue)
		require.Equal(t, proto.ParameterFormType_DROPDOWN, parameter.FormType)
		require.Len(t, parameter.Options, 2)
		require.Equal(t, "US East 1", parameter.Options[0].Name)

		require.Len(t, state.Presets, 1)
		preset := state.Presets[0]
		require.Equal(t, "fast-start", preset.Name)
		require.Equal(t, "Warm workspace", preset.Description)
		require.Len(t, preset.Parameters, 1)
		require.Equal(t, "region", preset.Parameters[0].Name)
		require.Equal(t, "us-east-1", preset.Parameters[0].Value)
		require.NotNil(t, preset.Prebuild)
		require.EqualValues(t, 2, preset.Prebuild.Instances)
		require.NotNil(t, preset.Prebuild.ExpirationPolicy)
		require.EqualValues(t, 3600, preset.Prebuild.ExpirationPolicy.Ttl)

		require.Len(t, state.ExternalAuthProviders, 1)
		require.Equal(t, "github", state.ExternalAuthProviders[0].Id)
		require.True(t, state.ExternalAuthProviders[0].Optional)

		require.Len(t, state.AITasks, 1)
		require.Equal(t, "task-1", state.AITasks[0].Id)
		require.Equal(t, "editor", state.AITasks[0].AppId)
		require.NotNil(t, state.AITasks[0].SidebarApp)
		require.Equal(t, "editor", state.AITasks[0].SidebarApp.Id)
		require.True(t, state.HasAITasks)
		require.False(t, state.HasExternalAgents)
	})

	t.Run("EmptyExport", func(t *testing.T) {
		t.Parallel()

		state := convertState(t, []byte(`{"version":3,"deployment":{"manifest":{"time":"2024-01-01T00:00:00Z","magic":"abc","version":"v3.100.0"},"resources":[]}}`))
		require.Empty(t, state.Resources)
		require.Empty(t, state.Parameters)
		require.Empty(t, state.Presets)
		require.Empty(t, state.ExternalAuthProviders)
		require.Empty(t, state.AITasks)
		require.False(t, state.HasAITasks)
		require.False(t, state.HasExternalAgents)
	})

	t.Run("MissingOptionalFields", func(t *testing.T) {
		t.Parallel()

		state := convertState(t, makeStackExport(
			mustJSON(t, map[string]any{
				"urn":    containerURN,
				"type":   "docker:index/container:Container",
				"custom": true,
				"outputs": map[string]any{
					"id": "container-1",
				},
			}),
			mustJSON(t, map[string]any{
				"urn":          agentURN,
				"type":         "coder:index/agent:Agent",
				"custom":       true,
				"dependencies": []string{containerURN},
				"outputs": map[string]any{
					"id":   "agent-1",
					"auth": "token",
				},
			}),
			mustJSON(t, map[string]any{
				"urn":    appURN,
				"type":   "coder:index/app:App",
				"custom": true,
				"outputs": map[string]any{
					"id":      "app-1",
					"agentId": "agent-1",
					"slug":    "editor",
				},
			}),
		))

		require.Len(t, state.Resources, 1)
		require.Len(t, state.Resources[0].Agents, 1)
		agent := state.Resources[0].Agents[0]
		require.Equal(t, "dev", agent.Name)
		require.Equal(t, "", agent.OperatingSystem)
		require.Equal(t, "", agent.Architecture)
		require.Equal(t, "", agent.Directory)
		require.Equal(t, "", agent.GetToken())
		require.Empty(t, agent.Metadata)
		require.NotNil(t, agent.DisplayApps)
		require.True(t, agent.DisplayApps.Vscode)
		require.True(t, agent.DisplayApps.WebTerminal)
		require.True(t, agent.DisplayApps.SshHelper)
		require.True(t, agent.DisplayApps.PortForwardingHelper)
		require.NotNil(t, agent.ResourcesMonitoring)
		require.Empty(t, agent.ResourcesMonitoring.Volumes)
		require.Len(t, agent.Apps, 1)
		require.Equal(t, "editor", agent.Apps[0].Slug)
		require.Equal(t, "", agent.Apps[0].DisplayName)
		require.Equal(t, proto.AppOpenIn_SLIM_WINDOW, agent.Apps[0].OpenIn)
	})

	t.Run("MalformedExport", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name            string
			input           []byte
			wantErrContains string
			wantEmptyState  bool
		}{
			{
				name:            "EmptyInput",
				input:           []byte(""),
				wantErrContains: "pulumi state must not be empty",
			},
			{
				name:            "InvalidJSON",
				input:           []byte("{invalid"),
				wantErrContains: "decode pulumi stack export",
			},
			{
				name:           "MissingResourcesKey",
				input:          []byte(`{"version":3,"deployment":{"manifest":{"time":"2024-01-01T00:00:00Z","magic":"abc","version":"v3.100.0"}}}`),
				wantEmptyState: true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				state, err := pulumi.ConvertState(context.Background(), tt.input, testutil.Logger(t))
				if tt.wantErrContains != "" {
					require.Error(t, err)
					require.Contains(t, err.Error(), tt.wantErrContains)
					return
				}
				require.NoError(t, err)
				if tt.wantEmptyState {
					require.Empty(t, state.Resources)
					require.Empty(t, state.Parameters)
					require.Empty(t, state.Presets)
					require.Empty(t, state.ExternalAuthProviders)
					require.Empty(t, state.AITasks)
					require.False(t, state.HasAITasks)
				}
			})
		}
	})

	t.Run("KeyStyleResilience", func(t *testing.T) {
		t.Parallel()

		var camelState *pulumi.State
		var snakeState *pulumi.State

		t.Run("CamelCaseKeys", func(t *testing.T) {
			camelState = convertState(t, keyStyleExport(t, camelKeyStyle))
			assertKeyStyleState(t, camelState)
		})

		t.Run("SnakeCaseKeys", func(t *testing.T) {
			snakeState = convertState(t, keyStyleExport(t, snakeKeyStyle))
			assertKeyStyleState(t, snakeState)
		})

		assertStatesEqual(t, camelState, snakeState)
	})

	t.Run("Determinism", func(t *testing.T) {
		t.Parallel()

		input := keyStyleExport(t, camelKeyStyle)
		baseline := convertState(t, input)
		for i := range 10 {
			current := convertState(t, input)
			assertStatesEqual(t, baseline, current)
			_ = i
		}
	})

	t.Run("FallbackToInputsWhenOutputsMissing", func(t *testing.T) {
		t.Parallel()

		state := convertState(t, makeStackExport(
			mustJSON(t, map[string]any{
				"urn":    containerURN,
				"type":   "docker:index/container:Container",
				"custom": true,
				"inputs": map[string]any{
					"id": "container-input",
				},
			}),
			mustJSON(t, map[string]any{
				"urn":          agentURN,
				"type":         "coder:index/agent:Agent",
				"custom":       true,
				"dependencies": []string{containerURN},
				"inputs": map[string]any{
					"id":   "agent-input",
					"auth": "token",
					"name": "dev",
				},
			}),
			mustJSON(t, map[string]any{
				"urn":    appURN,
				"type":   "coder:index/app:App",
				"custom": true,
				"inputs": map[string]any{
					"id":          "app-input",
					"agentId":     "agent-input",
					"slug":        "editor",
					"displayName": "Editor",
				},
			}),
			mustJSON(t, map[string]any{
				"urn":    metadataURN,
				"type":   "coder:index/metadata:Metadata",
				"custom": true,
				"inputs": map[string]any{
					"resourceId": "container-input",
					"items": []map[string]any{{
						"key":   "source",
						"value": "inputs",
					}},
				},
			}),
		))

		require.Len(t, state.Resources, 1)
		resource := state.Resources[0]
		require.Equal(t, "workspace", resource.Name)
		require.Len(t, resource.Metadata, 1)
		require.Equal(t, "source", resource.Metadata[0].Key)
		require.Equal(t, "inputs", resource.Metadata[0].Value)
		require.Len(t, resource.Agents, 1)
		require.Equal(t, "agent-input", resource.Agents[0].Id)
		require.Len(t, resource.Agents[0].Apps, 1)
		require.Equal(t, "app-input", resource.Agents[0].Apps[0].Id)
		require.Equal(t, "editor", resource.Agents[0].Apps[0].Slug)
		require.Equal(t, "Editor", resource.Agents[0].Apps[0].DisplayName)
	})
}

func convertState(t *testing.T, rawJSON []byte) *pulumi.State {
	t.Helper()

	state, err := pulumi.ConvertState(context.Background(), rawJSON, testutil.Logger(t))
	require.NoError(t, err)
	return state
}

func assertKeyStyleState(t *testing.T, state *pulumi.State) {
	t.Helper()

	require.Len(t, state.Resources, 1)
	require.Len(t, state.Resources[0].Agents, 1)
	agent := state.Resources[0].Agents[0]
	require.Equal(t, "dev", agent.Name)
	require.EqualValues(t, 45, agent.ConnectionTimeoutSeconds)
	require.Equal(t, "/etc/motd", agent.MotdFile)
	require.Equal(t, "all", agent.ApiKeyScope)
	require.NotNil(t, agent.DisplayApps)
	require.False(t, agent.DisplayApps.Vscode)
	require.True(t, agent.DisplayApps.VscodeInsiders)
	require.False(t, agent.DisplayApps.WebTerminal)
	require.False(t, agent.DisplayApps.SshHelper)
	require.True(t, agent.DisplayApps.PortForwardingHelper)
	require.Len(t, agent.Apps, 1)
	require.Equal(t, "editor", agent.Apps[0].Slug)
	require.Equal(t, "Editor", agent.Apps[0].DisplayName)
	require.Equal(t, proto.AppOpenIn_TAB, agent.Apps[0].OpenIn)
	require.Len(t, agent.Scripts, 1)
	require.Equal(t, "Startup", agent.Scripts[0].DisplayName)
	require.Len(t, agent.ExtraEnvs, 1)
	require.Equal(t, "replace", agent.ExtraEnvs[0].MergeStrategy)
	require.Len(t, state.Resources[0].Metadata, 1)
	require.Equal(t, "snake-or-camel", state.Resources[0].Metadata[0].Value)
	require.Len(t, state.Parameters, 1)
	require.Equal(t, "value-from-keys", state.Parameters[0].DefaultValue)
	require.Len(t, state.AITasks, 1)
	require.Equal(t, "editor", state.AITasks[0].SidebarApp.Id)
}

func assertStatesEqual(t *testing.T, want, got *pulumi.State) {
	t.Helper()

	diff := cmp.Diff(toComparableState(want), toComparableState(got), protocmp.Transform())
	require.Empty(t, diff)
}

func toComparableState(state *pulumi.State) comparableState {
	if state == nil {
		return comparableState{}
	}
	return comparableState{
		Resources:             state.Resources,
		Parameters:            state.Parameters,
		Presets:               state.Presets,
		ExternalAuthProviders: state.ExternalAuthProviders,
		AITasks:               state.AITasks,
		HasAITasks:            state.HasAITasks,
		HasExternalAgents:     state.HasExternalAgents,
	}
}

func keyStyleExport(t *testing.T, style keyStyle) []byte {
	t.Helper()

	agentIDKey := "agentId"
	displayNameKey := "displayName"
	openInKey := "openIn"
	mergeStrategyKey := "mergeStrategy"
	resourceIDKey := "resourceId"
	dailyCostKey := "dailyCost"
	isNullKey := "isNull"
	defaultValueKey := "defaultValue"
	formTypeKey := "formType"
	appIDKey := "appId"
	sidebarAppKey := "sidebarApp"
	displayAppsKey := "displayApps"
	resourcesMonitoringKey := "resourcesMonitoring"
	connectionTimeoutKey := "connectionTimeout"
	motdFileKey := "motdFile"
	scopePropertyKey := "apiKeyScope"
	vscodeInsidersKey := "vscodeInsiders"
	webTerminalKey := "webTerminal"
	sshHelperKey := "sshHelper"
	portForwardingHelperKey := "portForwardingHelper"
	if style == snakeKeyStyle {
		agentIDKey = "agent_id"
		displayNameKey = "display_name"
		openInKey = "open_in"
		mergeStrategyKey = "merge_strategy"
		resourceIDKey = "resource_id"
		dailyCostKey = "daily_cost"
		isNullKey = "is_null"
		defaultValueKey = "default_value"
		formTypeKey = "form_type"
		appIDKey = "app_id"
		sidebarAppKey = "sidebar_app"
		displayAppsKey = "display_apps"
		resourcesMonitoringKey = "resources_monitoring"
		connectionTimeoutKey = "connection_timeout"
		motdFileKey = "motd_file"
		scopePropertyKey = strings.Join([]string{"api", "key", "scope"}, "_")
		vscodeInsidersKey = "vscode_insiders"
		webTerminalKey = "web_terminal"
		sshHelperKey = "ssh_helper"
		portForwardingHelperKey = "port_forwarding_helper"
	}

	return makeStackExport(
		mustJSON(t, map[string]any{
			"urn":    containerURN,
			"type":   "docker:index/container:Container",
			"custom": true,
			"outputs": map[string]any{
				"id": "container-1",
			},
		}),
		mustJSON(t, map[string]any{
			"urn":          agentURN,
			"type":         "coder:index/agent:Agent",
			"custom":       true,
			"dependencies": []string{containerURN},
			"outputs": map[string]any{
				"id":                 "agent-1",
				"name":               "dev",
				"auth":               "token",
				connectionTimeoutKey: 45,
				motdFileKey:          "/etc/motd",
				scopePropertyKey:     "all",
				displayAppsKey: map[string]any{
					"vscode":                false,
					vscodeInsidersKey:       true,
					webTerminalKey:          false,
					sshHelperKey:            false,
					portForwardingHelperKey: true,
				},
				resourcesMonitoringKey: map[string]any{
					"memory": map[string]any{
						"enabled":   true,
						"threshold": 75,
					},
				},
			},
		}),
		mustJSON(t, map[string]any{
			"urn":    appURN,
			"type":   "coder:index/app:App",
			"custom": true,
			"outputs": map[string]any{
				"id":           "app-1",
				agentIDKey:     "agent-1",
				"slug":         "editor",
				displayNameKey: "Editor",
				openInKey:      "tab",
				"sharingLevel": "public",
			},
		}),
		mustJSON(t, map[string]any{
			"urn":    scriptURN,
			"type":   "coder:index/script:Script",
			"custom": true,
			"outputs": map[string]any{
				agentIDKey:     "agent-1",
				displayNameKey: "Startup",
				"script":       "echo hi",
			},
		}),
		mustJSON(t, map[string]any{
			"urn":    envURN,
			"type":   "coder:index/env:Env",
			"custom": true,
			"outputs": map[string]any{
				agentIDKey:       "agent-1",
				"name":           "EDITOR_MODE",
				"value":          "remote",
				mergeStrategyKey: "replace",
			},
		}),
		mustJSON(t, map[string]any{
			"urn":    metadataURN,
			"type":   "coder:index/metadata:Metadata",
			"custom": true,
			"outputs": map[string]any{
				resourceIDKey: "container-1",
				dailyCostKey:  13,
				"items": []map[string]any{{
					"key":     "key-style",
					"value":   "snake-or-camel",
					isNullKey: true,
				}},
			},
		}),
		mustJSON(t, map[string]any{
			"urn":    parameterURN,
			"type":   "coder:index/parameter:Parameter",
			"custom": true,
			"outputs": map[string]any{
				"name":          "flavor",
				"type":          "string",
				defaultValueKey: "value-from-keys",
				formTypeKey:     "dropdown",
				"options": []map[string]any{{
					"name":  "One",
					"value": "one",
				}},
			},
		}),
		mustJSON(t, map[string]any{
			"urn":    aiTaskURN,
			"type":   "coder:index/aiTask:AiTask",
			"custom": true,
			"outputs": map[string]any{
				"id":     "task-1",
				appIDKey: "editor",
				sidebarAppKey: map[string]any{
					"id": "editor",
				},
			},
		}),
	)
}

func makeStackExport(resources ...string) []byte {
	return []byte(fmt.Sprintf(`{"version":3,"deployment":{"manifest":{"time":"2024-01-01T00:00:00Z","magic":"abc","version":"v3.100.0"},"resources":[%s]}}`, strings.Join(resources, ",")))
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()

	data, err := json.Marshal(value)
	require.NoError(t, err)
	return string(data)
}
