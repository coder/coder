package terraform

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/awalterschulze/gographviz"
	tfjson "github.com/hashicorp/terraform-json"
	"github.com/mitchellh/mapstructure"
	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/terraform-provider-coder/v2/provider"

	tfaddr "github.com/hashicorp/go-terraform-address"

	"github.com/coder/coder/v2/coderd/util/slice"
	stringutil "github.com/coder/coder/v2/coderd/util/strings"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisioner"
	"github.com/coder/coder/v2/provisionersdk"
	"github.com/coder/coder/v2/provisionersdk/proto"
)

type agentMetadata struct {
	Key         string `mapstructure:"key"`
	DisplayName string `mapstructure:"display_name"`
	Script      string `mapstructure:"script"`
	Interval    int64  `mapstructure:"interval"`
	Timeout     int64  `mapstructure:"timeout"`
	Order       int64  `mapstructure:"order"`
}

// A mapping of attributes on the "coder_agent" resource.
type agentAttributes struct {
	Auth            string            `mapstructure:"auth"`
	OperatingSystem string            `mapstructure:"os"`
	Architecture    string            `mapstructure:"arch"`
	Directory       string            `mapstructure:"dir"`
	ID              string            `mapstructure:"id"`
	Token           string            `mapstructure:"token"`
	Env             map[string]string `mapstructure:"env"`
	// Deprecated: but remains here for backwards compatibility.
	StartupScript                string `mapstructure:"startup_script"`
	StartupScriptBehavior        string `mapstructure:"startup_script_behavior"`
	StartupScriptTimeoutSeconds  int32  `mapstructure:"startup_script_timeout"`
	LoginBeforeReady             bool   `mapstructure:"login_before_ready"`
	ShutdownScript               string `mapstructure:"shutdown_script"`
	ShutdownScriptTimeoutSeconds int32  `mapstructure:"shutdown_script_timeout"`

	ConnectionTimeoutSeconds int32                        `mapstructure:"connection_timeout"`
	TroubleshootingURL       string                       `mapstructure:"troubleshooting_url"`
	MOTDFile                 string                       `mapstructure:"motd_file"`
	Metadata                 []agentMetadata              `mapstructure:"metadata"`
	DisplayApps              []agentDisplayAppsAttributes `mapstructure:"display_apps"`
	Order                    int64                        `mapstructure:"order"`
	ResourcesMonitoring      []agentResourcesMonitoring   `mapstructure:"resources_monitoring"`
}

type agentDevcontainerAttributes struct {
	AgentID         string `mapstructure:"agent_id"`
	WorkspaceFolder string `mapstructure:"workspace_folder"`
	ConfigPath      string `mapstructure:"config_path"`
}

type agentResourcesMonitoring struct {
	Memory  []agentMemoryResourceMonitor `mapstructure:"memory"`
	Volumes []agentVolumeResourceMonitor `mapstructure:"volume"`
}

type agentMemoryResourceMonitor struct {
	Enabled   bool  `mapstructure:"enabled"`
	Threshold int32 `mapstructure:"threshold"`
}

type agentVolumeResourceMonitor struct {
	Path      string `mapstructure:"path"`
	Enabled   bool   `mapstructure:"enabled"`
	Threshold int32  `mapstructure:"threshold"`
}

type agentDisplayAppsAttributes struct {
	VSCode               bool `mapstructure:"vscode"`
	VSCodeInsiders       bool `mapstructure:"vscode_insiders"`
	WebTerminal          bool `mapstructure:"web_terminal"`
	SSHHelper            bool `mapstructure:"ssh_helper"`
	PortForwardingHelper bool `mapstructure:"port_forwarding_helper"`
}

// A mapping of attributes on the "coder_app" resource.
type agentAppAttributes struct {
	AgentID string `mapstructure:"agent_id"`
	// Slug is required in terraform, but to avoid breaking existing users we
	// will default to the resource name if it is not specified.
	Slug        string `mapstructure:"slug"`
	DisplayName string `mapstructure:"display_name"`
	// Name is deprecated in favor of DisplayName.
	Name        string                     `mapstructure:"name"`
	Icon        string                     `mapstructure:"icon"`
	URL         string                     `mapstructure:"url"`
	External    bool                       `mapstructure:"external"`
	Command     string                     `mapstructure:"command"`
	Share       string                     `mapstructure:"share"`
	Subdomain   bool                       `mapstructure:"subdomain"`
	Healthcheck []appHealthcheckAttributes `mapstructure:"healthcheck"`
	Order       int64                      `mapstructure:"order"`
	Hidden      bool                       `mapstructure:"hidden"`
	OpenIn      string                     `mapstructure:"open_in"`
}

type agentEnvAttributes struct {
	AgentID string `mapstructure:"agent_id"`
	Name    string `mapstructure:"name"`
	Value   string `mapstructure:"value"`
}

type agentScriptAttributes struct {
	AgentID          string `mapstructure:"agent_id"`
	DisplayName      string `mapstructure:"display_name"`
	Icon             string `mapstructure:"icon"`
	Script           string `mapstructure:"script"`
	Cron             string `mapstructure:"cron"`
	LogPath          string `mapstructure:"log_path"`
	StartBlocksLogin bool   `mapstructure:"start_blocks_login"`
	RunOnStart       bool   `mapstructure:"run_on_start"`
	RunOnStop        bool   `mapstructure:"run_on_stop"`
	TimeoutSeconds   int32  `mapstructure:"timeout"`
}

// A mapping of attributes on the "healthcheck" resource.
type appHealthcheckAttributes struct {
	URL       string `mapstructure:"url"`
	Interval  int32  `mapstructure:"interval"`
	Threshold int32  `mapstructure:"threshold"`
}

// A mapping of attributes on the "coder_metadata" resource.
type resourceMetadataAttributes struct {
	ResourceID string                 `mapstructure:"resource_id"`
	Hide       bool                   `mapstructure:"hide"`
	Icon       string                 `mapstructure:"icon"`
	DailyCost  int32                  `mapstructure:"daily_cost"`
	Items      []resourceMetadataItem `mapstructure:"item"`
}

type resourceMetadataItem struct {
	Key       string `mapstructure:"key"`
	Value     string `mapstructure:"value"`
	Sensitive bool   `mapstructure:"sensitive"`
	IsNull    bool   `mapstructure:"is_null"`
}

type State struct {
	Resources             []*proto.Resource
	Parameters            []*proto.RichParameter
	Presets               []*proto.Preset
	ExternalAuthProviders []*proto.ExternalAuthProviderResource
}

var ErrInvalidTerraformAddr = xerrors.New("invalid terraform address")

// ConvertState consumes Terraform state and a GraphViz representation
// produced by `terraform graph` to produce resources consumable by Coder.
// nolint:gocognit // This function makes more sense being large for now, until refactored.
func ConvertState(ctx context.Context, modules []*tfjson.StateModule, rawGraph string, logger slog.Logger) (*State, error) {
	parsedGraph, err := gographviz.ParseString(rawGraph)
	if err != nil {
		return nil, xerrors.Errorf("parse graph: %w", err)
	}
	graph, err := gographviz.NewAnalysedGraph(parsedGraph)
	if err != nil {
		return nil, xerrors.Errorf("analyze graph: %w", err)
	}

	resources := make([]*proto.Resource, 0)
	resourceAgents := map[string][]*proto.Agent{}

	// Indexes Terraform resources by their label.
	// The label is what "terraform graph" uses to reference nodes.
	tfResourcesByLabel := map[string]map[string]*tfjson.StateResource{}

	// Extra array to preserve the order of rich parameters.
	tfResourcesRichParameters := make([]*tfjson.StateResource, 0)
	tfResourcesPresets := make([]*tfjson.StateResource, 0)
	var findTerraformResources func(mod *tfjson.StateModule)
	findTerraformResources = func(mod *tfjson.StateModule) {
		for _, module := range mod.ChildModules {
			findTerraformResources(module)
		}
		for _, resource := range mod.Resources {
			if resource.Type == "coder_parameter" {
				tfResourcesRichParameters = append(tfResourcesRichParameters, resource)
			}
			if resource.Type == "coder_workspace_preset" {
				tfResourcesPresets = append(tfResourcesPresets, resource)
			}

			label := convertAddressToLabel(resource.Address)
			if tfResourcesByLabel[label] == nil {
				tfResourcesByLabel[label] = map[string]*tfjson.StateResource{}
			}
			tfResourcesByLabel[label][resource.Address] = resource
		}
	}
	for _, module := range modules {
		findTerraformResources(module)
	}

	// Find all agents!
	agentNames := map[string]struct{}{}
	for _, tfResources := range tfResourcesByLabel {
		for _, tfResource := range tfResources {
			if tfResource.Type != "coder_agent" {
				continue
			}
			var attrs agentAttributes
			err = mapstructure.Decode(tfResource.AttributeValues, &attrs)
			if err != nil {
				return nil, xerrors.Errorf("decode agent attributes: %w", err)
			}

			// Similar logic is duplicated in terraform/resources.go.
			if tfResource.Name == "" {
				return nil, xerrors.Errorf("agent name cannot be empty")
			}
			// In 2025-02 we removed support for underscores in agent names. To
			// provide a nicer error message, we check the regex first and check
			// for underscores if it fails.
			if !provisioner.AgentNameRegex.MatchString(tfResource.Name) {
				if strings.Contains(tfResource.Name, "_") {
					return nil, xerrors.Errorf("agent name %q contains underscores which are no longer supported, please use hyphens instead (regex: %q)", tfResource.Name, provisioner.AgentNameRegex.String())
				}
				return nil, xerrors.Errorf("agent name %q does not match regex %q", tfResource.Name, provisioner.AgentNameRegex.String())
			}
			// Agent names must be case-insensitive-unique, to be unambiguous in
			// `coder_app`s and CoderVPN DNS names.
			if _, ok := agentNames[strings.ToLower(tfResource.Name)]; ok {
				return nil, xerrors.Errorf("duplicate agent name: %s", tfResource.Name)
			}
			agentNames[strings.ToLower(tfResource.Name)] = struct{}{}

			// Handling for deprecated attributes. login_before_ready was replaced
			// by startup_script_behavior, but we still need to support it for
			// backwards compatibility.
			startupScriptBehavior := string(codersdk.WorkspaceAgentStartupScriptBehaviorNonBlocking)
			if attrs.StartupScriptBehavior != "" {
				startupScriptBehavior = attrs.StartupScriptBehavior
			} else {
				// Handling for provider pre-v0.6.10 (because login_before_ready
				// defaulted to true, we must check for its presence).
				if _, ok := tfResource.AttributeValues["login_before_ready"]; ok && !attrs.LoginBeforeReady {
					startupScriptBehavior = string(codersdk.WorkspaceAgentStartupScriptBehaviorBlocking)
				}
			}

			var metadata []*proto.Agent_Metadata
			for _, item := range attrs.Metadata {
				metadata = append(metadata, &proto.Agent_Metadata{
					Key:         item.Key,
					DisplayName: item.DisplayName,
					Script:      item.Script,
					Interval:    item.Interval,
					Timeout:     item.Timeout,
					Order:       item.Order,
				})
			}

			// If a user doesn't specify 'display_apps' then they default
			// into all apps except VSCode Insiders.
			displayApps := provisionersdk.DefaultDisplayApps()

			if len(attrs.DisplayApps) != 0 {
				displayApps = &proto.DisplayApps{
					Vscode:               attrs.DisplayApps[0].VSCode,
					VscodeInsiders:       attrs.DisplayApps[0].VSCodeInsiders,
					WebTerminal:          attrs.DisplayApps[0].WebTerminal,
					PortForwardingHelper: attrs.DisplayApps[0].PortForwardingHelper,
					SshHelper:            attrs.DisplayApps[0].SSHHelper,
				}
			}

			resourcesMonitoring := &proto.ResourcesMonitoring{
				Volumes: make([]*proto.VolumeResourceMonitor, 0),
			}

			for _, resource := range attrs.ResourcesMonitoring {
				for _, memoryResource := range resource.Memory {
					resourcesMonitoring.Memory = &proto.MemoryResourceMonitor{
						Enabled:   memoryResource.Enabled,
						Threshold: memoryResource.Threshold,
					}
				}
			}

			for _, resource := range attrs.ResourcesMonitoring {
				for _, volume := range resource.Volumes {
					resourcesMonitoring.Volumes = append(resourcesMonitoring.Volumes, &proto.VolumeResourceMonitor{
						Path:      volume.Path,
						Enabled:   volume.Enabled,
						Threshold: volume.Threshold,
					})
				}
			}

			agent := &proto.Agent{
				Name:                     tfResource.Name,
				Id:                       attrs.ID,
				Env:                      attrs.Env,
				OperatingSystem:          attrs.OperatingSystem,
				Architecture:             attrs.Architecture,
				Directory:                attrs.Directory,
				ConnectionTimeoutSeconds: attrs.ConnectionTimeoutSeconds,
				TroubleshootingUrl:       attrs.TroubleshootingURL,
				MotdFile:                 attrs.MOTDFile,
				ResourcesMonitoring:      resourcesMonitoring,
				Metadata:                 metadata,
				DisplayApps:              displayApps,
				Order:                    attrs.Order,
			}
			// Support the legacy script attributes in the agent!
			if attrs.StartupScript != "" {
				agent.Scripts = append(agent.Scripts, &proto.Script{
					// This is ▶️
					Icon:             "/emojis/25b6.png",
					LogPath:          "coder-startup-script.log",
					DisplayName:      "Startup Script",
					Script:           attrs.StartupScript,
					StartBlocksLogin: startupScriptBehavior == string(codersdk.WorkspaceAgentStartupScriptBehaviorBlocking),
					RunOnStart:       true,
				})
			}
			if attrs.ShutdownScript != "" {
				agent.Scripts = append(agent.Scripts, &proto.Script{
					// This is ◀️
					Icon:        "/emojis/25c0.png",
					LogPath:     "coder-shutdown-script.log",
					DisplayName: "Shutdown Script",
					Script:      attrs.ShutdownScript,
					RunOnStop:   true,
				})
			}
			switch attrs.Auth {
			case "token":
				agent.Auth = &proto.Agent_Token{
					Token: attrs.Token,
				}
			default:
				// If token authentication isn't specified,
				// assume instance auth. It's our only other
				// authentication type!
				agent.Auth = &proto.Agent_InstanceId{}
			}

			// The label is used to find the graph node!
			agentLabel := convertAddressToLabel(tfResource.Address)

			var agentNode *gographviz.Node
			for _, node := range graph.Nodes.Lookup {
				// The node attributes surround the label with quotes.
				if strings.Trim(node.Attrs["label"], `"`) != agentLabel {
					continue
				}
				agentNode = node
				break
			}
			if agentNode == nil {
				return nil, xerrors.Errorf("couldn't find node on graph: %q", agentLabel)
			}

			var agentResource *graphResource
			for _, resource := range findResourcesInGraph(graph, tfResourcesByLabel, agentNode.Name, 0, true) {
				if agentResource == nil {
					// Default to the first resource because we have nothing to compare!
					agentResource = resource
					continue
				}
				if resource.Depth < agentResource.Depth {
					// There's a closer resource!
					agentResource = resource
					continue
				}
				if resource.Depth == agentResource.Depth && resource.Label < agentResource.Label {
					agentResource = resource
					continue
				}
			}

			if agentResource == nil {
				continue
			}

			agents, exists := resourceAgents[agentResource.Label]
			if !exists {
				agents = make([]*proto.Agent, 0)
			}
			agents = append(agents, agent)
			resourceAgents[agentResource.Label] = agents
		}
	}

	// Manually associate agents with instance IDs.
	for _, resources := range tfResourcesByLabel {
		for _, resource := range resources {
			if resource.Type != "coder_agent_instance" {
				continue
			}
			agentIDRaw, valid := resource.AttributeValues["agent_id"]
			if !valid {
				continue
			}
			agentID, valid := agentIDRaw.(string)
			if !valid {
				continue
			}
			instanceIDRaw, valid := resource.AttributeValues["instance_id"]
			if !valid {
				continue
			}
			instanceID, valid := instanceIDRaw.(string)
			if !valid {
				continue
			}

			for _, agents := range resourceAgents {
				for _, agent := range agents {
					if agent.Id != agentID {
						continue
					}
					// Only apply the instance ID if the agent authentication
					// type is set to do so. A user ran into a bug where they
					// had the instance ID block, but auth was set to "token". See:
					// https://github.com/coder/coder/issues/4551#issuecomment-1336293468
					switch t := agent.Auth.(type) {
					case *proto.Agent_Token:
						continue
					case *proto.Agent_InstanceId:
						t.InstanceId = instanceID
					}
					break
				}
			}
		}
	}

	// Associate Apps with agents.
	appSlugs := make(map[string]struct{})
	for _, resources := range tfResourcesByLabel {
		for _, resource := range resources {
			if resource.Type != "coder_app" {
				continue
			}

			var attrs agentAppAttributes
			err = mapstructure.Decode(resource.AttributeValues, &attrs)
			if err != nil {
				return nil, xerrors.Errorf("decode app attributes: %w", err)
			}

			// Default to the resource name if none is set!
			if attrs.Slug == "" {
				attrs.Slug = resource.Name
			}
			// Similar logic is duplicated in terraform/resources.go.
			if attrs.DisplayName == "" {
				if attrs.Name != "" {
					// Name is deprecated but still accepted.
					attrs.DisplayName = attrs.Name
				} else {
					attrs.DisplayName = attrs.Slug
				}
			}

			// Contrary to agent names above, app slugs were never permitted to
			// contain uppercase letters or underscores.
			if !provisioner.AppSlugRegex.MatchString(attrs.Slug) {
				return nil, xerrors.Errorf("app slug %q does not match regex %q", attrs.Slug, provisioner.AppSlugRegex.String())
			}

			if _, exists := appSlugs[attrs.Slug]; exists {
				return nil, xerrors.Errorf("duplicate app slug, they must be unique per template: %q", attrs.Slug)
			}
			appSlugs[attrs.Slug] = struct{}{}

			var healthcheck *proto.Healthcheck
			if len(attrs.Healthcheck) != 0 {
				healthcheck = &proto.Healthcheck{
					Url:       attrs.Healthcheck[0].URL,
					Interval:  attrs.Healthcheck[0].Interval,
					Threshold: attrs.Healthcheck[0].Threshold,
				}
			}

			sharingLevel := proto.AppSharingLevel_OWNER
			switch strings.ToLower(attrs.Share) {
			case "owner":
				sharingLevel = proto.AppSharingLevel_OWNER
			case "authenticated":
				sharingLevel = proto.AppSharingLevel_AUTHENTICATED
			case "public":
				sharingLevel = proto.AppSharingLevel_PUBLIC
			}

			openIn := proto.AppOpenIn_SLIM_WINDOW
			switch strings.ToLower(attrs.OpenIn) {
			case "slim-window":
				openIn = proto.AppOpenIn_SLIM_WINDOW
			case "tab":
				openIn = proto.AppOpenIn_TAB
			}

			for _, agents := range resourceAgents {
				for _, agent := range agents {
					// Find agents with the matching ID and associate them!

					if !dependsOnAgent(graph, agent, attrs.AgentID, resource) {
						continue
					}

					agent.Apps = append(agent.Apps, &proto.App{
						Slug:         attrs.Slug,
						DisplayName:  attrs.DisplayName,
						Command:      attrs.Command,
						External:     attrs.External,
						Url:          attrs.URL,
						Icon:         attrs.Icon,
						Subdomain:    attrs.Subdomain,
						SharingLevel: sharingLevel,
						Healthcheck:  healthcheck,
						Order:        attrs.Order,
						Hidden:       attrs.Hidden,
						OpenIn:       openIn,
					})
				}
			}
		}
	}

	// Associate envs with agents.
	for _, resources := range tfResourcesByLabel {
		for _, resource := range resources {
			if resource.Type != "coder_env" {
				continue
			}
			var attrs agentEnvAttributes
			err = mapstructure.Decode(resource.AttributeValues, &attrs)
			if err != nil {
				return nil, xerrors.Errorf("decode env attributes: %w", err)
			}
			for _, agents := range resourceAgents {
				for _, agent := range agents {
					// Find agents with the matching ID and associate them!
					if !dependsOnAgent(graph, agent, attrs.AgentID, resource) {
						continue
					}
					agent.ExtraEnvs = append(agent.ExtraEnvs, &proto.Env{
						Name:  attrs.Name,
						Value: attrs.Value,
					})
				}
			}
		}
	}

	// Associate scripts with agents.
	for _, resources := range tfResourcesByLabel {
		for _, resource := range resources {
			if resource.Type != "coder_script" {
				continue
			}
			var attrs agentScriptAttributes
			err = mapstructure.Decode(resource.AttributeValues, &attrs)
			if err != nil {
				return nil, xerrors.Errorf("decode script attributes: %w", err)
			}
			for _, agents := range resourceAgents {
				for _, agent := range agents {
					// Find agents with the matching ID and associate them!
					if !dependsOnAgent(graph, agent, attrs.AgentID, resource) {
						continue
					}
					agent.Scripts = append(agent.Scripts, &proto.Script{
						DisplayName:      attrs.DisplayName,
						Icon:             attrs.Icon,
						Script:           attrs.Script,
						Cron:             attrs.Cron,
						LogPath:          attrs.LogPath,
						StartBlocksLogin: attrs.StartBlocksLogin,
						RunOnStart:       attrs.RunOnStart,
						RunOnStop:        attrs.RunOnStop,
						TimeoutSeconds:   attrs.TimeoutSeconds,
					})
				}
			}
		}
	}

	// Associate Dev Containers with agents.
	for _, resources := range tfResourcesByLabel {
		for _, resource := range resources {
			if resource.Type != "coder_devcontainer" {
				continue
			}
			var attrs agentDevcontainerAttributes
			err = mapstructure.Decode(resource.AttributeValues, &attrs)
			if err != nil {
				return nil, xerrors.Errorf("decode script attributes: %w", err)
			}
			for _, agents := range resourceAgents {
				for _, agent := range agents {
					// Find agents with the matching ID and associate them!
					if !dependsOnAgent(graph, agent, attrs.AgentID, resource) {
						continue
					}
					agent.Devcontainers = append(agent.Devcontainers, &proto.Devcontainer{
						Name:            resource.Name,
						WorkspaceFolder: attrs.WorkspaceFolder,
						ConfigPath:      attrs.ConfigPath,
					})
				}
			}
		}
	}

	// Associate metadata blocks with resources.
	resourceMetadata := map[string][]*proto.Resource_Metadata{}
	resourceHidden := map[string]bool{}
	resourceIcon := map[string]string{}
	resourceCost := map[string]int32{}

	metadataTargetLabels := map[string]bool{}
	for _, resources := range tfResourcesByLabel {
		for _, resource := range resources {
			if resource.Type != "coder_metadata" {
				continue
			}

			var attrs resourceMetadataAttributes
			err = mapstructure.Decode(resource.AttributeValues, &attrs)
			if err != nil {
				return nil, xerrors.Errorf("decode metadata attributes: %w", err)
			}
			resourceLabel := convertAddressToLabel(resource.Address)

			var attachedNode *gographviz.Node
			for _, node := range graph.Nodes.Lookup {
				// The node attributes surround the label with quotes.
				if strings.Trim(node.Attrs["label"], `"`) != resourceLabel {
					continue
				}
				attachedNode = node
				break
			}
			if attachedNode == nil {
				continue
			}
			var attachedResource *graphResource
			for _, resource := range findResourcesInGraph(graph, tfResourcesByLabel, attachedNode.Name, 0, false) {
				if attachedResource == nil {
					// Default to the first resource because we have nothing to compare!
					attachedResource = resource
					continue
				}
				if resource.Depth < attachedResource.Depth {
					// There's a closer resource!
					attachedResource = resource
					continue
				}
				if resource.Depth == attachedResource.Depth && resource.Label < attachedResource.Label {
					attachedResource = resource
					continue
				}
			}
			if attachedResource == nil {
				continue
			}
			targetLabel := attachedResource.Label

			if metadataTargetLabels[targetLabel] {
				return nil, xerrors.Errorf("duplicate metadata resource: %s", targetLabel)
			}
			metadataTargetLabels[targetLabel] = true

			resourceHidden[targetLabel] = attrs.Hide
			resourceIcon[targetLabel] = attrs.Icon
			resourceCost[targetLabel] = attrs.DailyCost
			for _, item := range attrs.Items {
				resourceMetadata[targetLabel] = append(resourceMetadata[targetLabel],
					&proto.Resource_Metadata{
						Key:       item.Key,
						Value:     item.Value,
						Sensitive: item.Sensitive,
						IsNull:    item.IsNull,
					})
			}
		}
	}

	for _, tfResources := range tfResourcesByLabel {
		for _, resource := range tfResources {
			if resource.Mode == tfjson.DataResourceMode {
				continue
			}
			if resource.Type == "coder_script" || resource.Type == "coder_agent" || resource.Type == "coder_agent_instance" || resource.Type == "coder_app" || resource.Type == "coder_metadata" {
				continue
			}
			label := convertAddressToLabel(resource.Address)
			modulePath, err := convertAddressToModulePath(resource.Address)
			if err != nil {
				// Module path recording was added primarily to keep track of
				// modules in telemetry. We're adding this sentinel value so
				// we can detect if there are any issues with the address
				// parsing.
				//
				// We don't want to set modulePath to null here because, in
				// the database, a null value in WorkspaceResource's ModulePath
				// indicates "this resource was created before module paths
				// were tracked."
				modulePath = fmt.Sprintf("%s", ErrInvalidTerraformAddr)
				logger.Error(ctx, "failed to parse Terraform address", slog.F("address", resource.Address))
			}

			agents, exists := resourceAgents[label]
			if exists {
				applyAutomaticInstanceID(resource, agents)
			}

			resources = append(resources, &proto.Resource{
				Name:         resource.Name,
				Type:         resource.Type,
				Agents:       agents,
				Metadata:     resourceMetadata[label],
				Hide:         resourceHidden[label],
				Icon:         resourceIcon[label],
				DailyCost:    resourceCost[label],
				InstanceType: applyInstanceType(resource),
				ModulePath:   modulePath,
			})
		}
	}

	var duplicatedParamNames []string
	parameters := make([]*proto.RichParameter, 0)
	for _, resource := range tfResourcesRichParameters {
		var param provider.Parameter
		err = mapstructure.Decode(resource.AttributeValues, &param)
		if err != nil {
			return nil, xerrors.Errorf("decode map values for coder_parameter.%s: %w", resource.Name, err)
		}
		protoParam := &proto.RichParameter{
			Name:         param.Name,
			DisplayName:  param.DisplayName,
			Description:  param.Description,
			Type:         param.Type,
			Mutable:      param.Mutable,
			DefaultValue: param.Default,
			Icon:         param.Icon,
			Required:     !param.Optional,
			// #nosec G115 - Safe conversion as parameter order value is expected to be within int32 range
			Order:     int32(param.Order),
			Ephemeral: param.Ephemeral,
		}
		if len(param.Validation) == 1 {
			protoParam.ValidationRegex = param.Validation[0].Regex
			protoParam.ValidationError = param.Validation[0].Error

			validationAttributeValues, ok := resource.AttributeValues["validation"]
			if ok {
				validationAttributeValuesArr, ok := validationAttributeValues.([]interface{})
				if ok {
					validationAttributeValuesMapStr, ok := validationAttributeValuesArr[0].(map[string]interface{})
					if ok {
						// Backward compatibility with terraform-coder-plugin < v0.8.2:
						// * "min_disabled" and "max_disabled" are not available yet
						// * "min" and "max" are required to be specified together
						if _, ok = validationAttributeValuesMapStr["min_disabled"]; !ok {
							if param.Validation[0].Min != 0 || param.Validation[0].Max != 0 {
								param.Validation[0].MinDisabled = false
								param.Validation[0].MaxDisabled = false
							} else {
								param.Validation[0].MinDisabled = true
								param.Validation[0].MaxDisabled = true
							}
						}
					}
				}
			}

			if !param.Validation[0].MaxDisabled {
				protoParam.ValidationMax = PtrInt32(param.Validation[0].Max)
			}
			if !param.Validation[0].MinDisabled {
				protoParam.ValidationMin = PtrInt32(param.Validation[0].Min)
			}
			protoParam.ValidationMonotonic = param.Validation[0].Monotonic
		}
		if len(param.Option) > 0 {
			protoParam.Options = make([]*proto.RichParameterOption, 0, len(param.Option))
			for _, option := range param.Option {
				protoParam.Options = append(protoParam.Options, &proto.RichParameterOption{
					Name:        option.Name,
					Description: option.Description,
					Value:       option.Value,
					Icon:        option.Icon,
				})
			}
		}

		// Check if this parameter duplicates an existing parameter.
		formattedName := fmt.Sprintf("%q", protoParam.Name)
		if !slice.Contains(duplicatedParamNames, formattedName) &&
			slice.ContainsCompare(parameters, protoParam, func(a, b *proto.RichParameter) bool {
				return a.Name == b.Name
			}) {
			duplicatedParamNames = append(duplicatedParamNames, formattedName)
		}

		parameters = append(parameters, protoParam)
	}

	// Enforce that parameters be uniquely named.
	if len(duplicatedParamNames) > 0 {
		s := ""
		if len(duplicatedParamNames) == 1 {
			s = "s"
		}
		return nil, xerrors.Errorf(
			"coder_parameter names must be unique but %s appear%s multiple times",
			stringutil.JoinWithConjunction(duplicatedParamNames), s,
		)
	}

	var duplicatedPresetNames []string
	presets := make([]*proto.Preset, 0)
	for _, resource := range tfResourcesPresets {
		var preset provider.WorkspacePreset
		err = mapstructure.Decode(resource.AttributeValues, &preset)
		if err != nil {
			return nil, xerrors.Errorf("decode preset attributes: %w", err)
		}

		var duplicatedPresetParameterNames []string
		var nonExistentParameters []string
		var presetParameters []*proto.PresetParameter
		for name, value := range preset.Parameters {
			presetParameter := &proto.PresetParameter{
				Name:  name,
				Value: value,
			}

			formattedName := fmt.Sprintf("%q", name)
			if !slice.Contains(duplicatedPresetParameterNames, formattedName) &&
				slice.ContainsCompare(presetParameters, presetParameter, func(a, b *proto.PresetParameter) bool {
					return a.Name == b.Name
				}) {
				duplicatedPresetParameterNames = append(duplicatedPresetParameterNames, formattedName)
			}
			if !slice.ContainsCompare(parameters, &proto.RichParameter{Name: name}, func(a, b *proto.RichParameter) bool {
				return a.Name == b.Name
			}) {
				nonExistentParameters = append(nonExistentParameters, name)
			}

			presetParameters = append(presetParameters, presetParameter)
		}

		if len(duplicatedPresetParameterNames) > 0 {
			s := ""
			if len(duplicatedPresetParameterNames) == 1 {
				s = "s"
			}
			return nil, xerrors.Errorf(
				"coder_workspace_preset parameters must be unique but %s appear%s multiple times", stringutil.JoinWithConjunction(duplicatedPresetParameterNames), s,
			)
		}

		if len(nonExistentParameters) > 0 {
			logger.Warn(
				ctx,
				"coder_workspace_preset defines preset values for at least one parameter that is not defined by the template",
				slog.F("parameters", stringutil.JoinWithConjunction(nonExistentParameters)),
			)
		}

		protoPreset := &proto.Preset{
			Name:       preset.Name,
			Parameters: presetParameters,
		}

		if len(preset.Prebuild) > 1 {
			// The provider template schema should prevent this, but we're being defensive here.
			logger.Info(ctx, "coder_workspace_preset has more than 1 prebuild, only using the first one", slog.F("name", preset.Name))
		}
		if len(preset.Prebuild) == 1 {
			protoPreset.Prebuild = &proto.Prebuild{
				Instances: int32(math.Min(math.MaxInt32, float64(preset.Prebuild[0].Instances))),
			}
		}

		if slice.Contains(duplicatedPresetNames, preset.Name) {
			duplicatedPresetNames = append(duplicatedPresetNames, preset.Name)
		}
		presets = append(presets, protoPreset)
	}
	if len(duplicatedPresetNames) > 0 {
		s := ""
		if len(duplicatedPresetNames) == 1 {
			s = "s"
		}
		return nil, xerrors.Errorf(
			"coder_workspace_preset names must be unique but %s appear%s multiple times",
			stringutil.JoinWithConjunction(duplicatedPresetNames), s,
		)
	}

	// A map is used to ensure we don't have duplicates!
	externalAuthProvidersMap := map[string]*proto.ExternalAuthProviderResource{}
	for _, tfResources := range tfResourcesByLabel {
		for _, resource := range tfResources {
			// Checking for `coder_git_auth` is legacy!
			if resource.Type != "coder_external_auth" && resource.Type != "coder_git_auth" {
				continue
			}

			id, ok := resource.AttributeValues["id"].(string)
			if !ok {
				return nil, xerrors.Errorf("external auth id is not a string")
			}
			optional := false
			optionalAttribute, ok := resource.AttributeValues["optional"].(bool)
			if ok {
				optional = optionalAttribute
			}

			externalAuthProvidersMap[id] = &proto.ExternalAuthProviderResource{
				Id:       id,
				Optional: optional,
			}
		}
	}
	externalAuthProviders := make([]*proto.ExternalAuthProviderResource, 0, len(externalAuthProvidersMap))
	for _, it := range externalAuthProvidersMap {
		externalAuthProviders = append(externalAuthProviders, it)
	}

	return &State{
		Resources:             resources,
		Parameters:            parameters,
		Presets:               presets,
		ExternalAuthProviders: externalAuthProviders,
	}, nil
}

func PtrInt32(number int) *int32 {
	// #nosec G115 - Safe conversion as the number is expected to be within int32 range
	n := int32(number)
	return &n
}

// convertAddressToLabel returns the Terraform address without the count
// specifier.
// eg. "module.ec2_dev.ec2_instance.dev[0]" becomes "module.ec2_dev.ec2_instance.dev"
func convertAddressToLabel(address string) string {
	cut, _, _ := strings.Cut(address, "[")
	return cut
}

// convertAddressToModulePath returns the module path from a Terraform address.
// eg. "module.ec2_dev.ec2_instance.dev[0]" becomes "module.ec2_dev".
// Empty string is returned for the root module.
//
// Module paths are defined in the Terraform spec:
// https://github.com/hashicorp/terraform/blob/ef071f3d0e49ba421ae931c65b263827a8af1adb/website/docs/internals/resource-addressing.html.markdown#module-path
func convertAddressToModulePath(address string) (string, error) {
	addr, err := tfaddr.NewAddress(address)
	if err != nil {
		return "", xerrors.Errorf("parse terraform address: %w", err)
	}
	return addr.ModulePath.String(), nil
}

func dependsOnAgent(graph *gographviz.Graph, agent *proto.Agent, resourceAgentID string, resource *tfjson.StateResource) bool {
	// Plan: we need to find if there is edge between the agent and the resource.
	if agent.Id == "" && resourceAgentID == "" {
		resourceNodeSuffix := fmt.Sprintf(`] %s.%s (expand)"`, resource.Type, resource.Name)
		agentNodeSuffix := fmt.Sprintf(`] coder_agent.%s (expand)"`, agent.Name)

		// Traverse the graph to check if the coder_<resource_type> depends on coder_agent.
		for _, dst := range graph.Edges.SrcToDsts {
			for _, edges := range dst {
				for _, edge := range edges {
					if strings.HasSuffix(edge.Src, resourceNodeSuffix) &&
						strings.HasSuffix(edge.Dst, agentNodeSuffix) {
						return true
					}
				}
			}
		}
		return false
	}

	// Provision: agent ID and child resource ID are present
	return agent.Id == resourceAgentID
}

type graphResource struct {
	Label string
	Depth uint
}

// applyInstanceType sets the instance type on an agent if it matches
// one of the special resource types that we track.
func applyInstanceType(resource *tfjson.StateResource) string {
	key, isValid := map[string]string{
		"google_compute_instance":         "machine_type",
		"aws_instance":                    "instance_type",
		"aws_spot_instance_request":       "instance_type",
		"azurerm_linux_virtual_machine":   "size",
		"azurerm_windows_virtual_machine": "size",
	}[resource.Type]
	if !isValid {
		return ""
	}

	instanceTypeRaw, isValid := resource.AttributeValues[key]
	if !isValid {
		return ""
	}
	instanceType, isValid := instanceTypeRaw.(string)
	if !isValid {
		return ""
	}
	return instanceType
}

// applyAutomaticInstanceID checks if the resource is one of a set of *magical* IDs
// that automatically index their identifier for automatic authentication.
func applyAutomaticInstanceID(resource *tfjson.StateResource, agents []*proto.Agent) {
	// These resource types are for automatically associating an instance ID
	// with an agent for authentication.
	key, isValid := map[string]string{
		"google_compute_instance":         "instance_id",
		"aws_instance":                    "id",
		"aws_spot_instance_request":       "spot_instance_id",
		"azurerm_linux_virtual_machine":   "virtual_machine_id",
		"azurerm_windows_virtual_machine": "virtual_machine_id",
	}[resource.Type]
	if !isValid {
		return
	}

	// The resource type doesn't support
	// automatically setting the instance ID.
	instanceIDRaw, isValid := resource.AttributeValues[key]
	if !isValid {
		return
	}
	instanceID, isValid := instanceIDRaw.(string)
	if !isValid {
		return
	}
	for _, agent := range agents {
		// Didn't use instance identity.
		if agent.GetToken() != "" {
			continue
		}
		if agent.GetInstanceId() != "" {
			// If an instance ID is manually specified, do not override!
			continue
		}

		agent.Auth = &proto.Agent_InstanceId{
			InstanceId: instanceID,
		}
	}
}

// findResourcesInGraph traverses directionally in a graph until a resource is found,
// then it stores the depth it was found at, and continues working up the tree.
// nolint:revive
func findResourcesInGraph(graph *gographviz.Graph, tfResourcesByLabel map[string]map[string]*tfjson.StateResource, nodeName string, currentDepth uint, up bool) []*graphResource {
	graphResources := make([]*graphResource, 0)
	mapping := graph.Edges.DstToSrcs
	if !up {
		mapping = graph.Edges.SrcToDsts
	}
	for destination := range mapping[nodeName] {
		destinationNode := graph.Nodes.Lookup[destination]
		// Work our way up the tree!
		graphResources = append(graphResources, findResourcesInGraph(graph, tfResourcesByLabel, destinationNode.Name, currentDepth+1, up)...)

		destinationLabel, exists := destinationNode.Attrs["label"]
		if !exists {
			continue
		}
		destinationLabel = strings.Trim(destinationLabel, `"`)
		resources, exists := tfResourcesByLabel[destinationLabel]
		if !exists {
			continue
		}
		for _, resource := range resources {
			// Data sources cannot be associated with agents for now!
			if resource.Mode != tfjson.ManagedResourceMode {
				continue
			}
			// Don't associate Coder resources with other Coder resources!
			if strings.HasPrefix(resource.Type, "coder_") {
				continue
			}
			graphResources = append(graphResources, &graphResource{
				Label: destinationLabel,
				Depth: currentDepth,
			})
		}
	}

	return graphResources
}
