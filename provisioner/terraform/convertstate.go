package terraform

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/awalterschulze/gographviz"
	"github.com/google/uuid"
	tfjson "github.com/hashicorp/terraform-json"
	"github.com/mitchellh/mapstructure"
	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/terraform-provider-coder/v2/provider"

	"github.com/coder/coder/v2/coderd/util/slice"
	stringutil "github.com/coder/coder/v2/coderd/util/strings"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisioner"
	"github.com/coder/coder/v2/provisionersdk"
	"github.com/coder/coder/v2/provisionersdk/proto"
)

// stateConverter holds intermediate state for converting Terraform state to Coder resources.
type stateConverter struct {
	ctx    context.Context
	logger slog.Logger
	graph  *gographviz.Graph

	// Indexed Terraform resources by their label (what "terraform graph" uses to reference nodes).
	tfResourcesByLabel map[string]map[string]*tfjson.StateResource

	// Categorized resources for processing.
	tfResourcesRichParameters []*tfjson.StateResource
	tfResourcesPresets        []*tfjson.StateResource
	tfResourcesAITasks        []*tfjson.StateResource

	// Output state.
	resources      []*proto.Resource
	resourceAgents map[string][]*proto.Agent

	// Resource metadata collected during processing.
	resourceMetadata map[string][]*proto.Resource_Metadata
	resourceHidden   map[string]bool
	resourceIcon     map[string]string
	resourceCost     map[string]int32
}

// ConvertState consumes Terraform state and a GraphViz representation
// produced by `terraform graph` to produce resources consumable by Coder.
func ConvertState(ctx context.Context, modules []*tfjson.StateModule, rawGraph string, logger slog.Logger) (*State, error) {
	parsedGraph, err := gographviz.ParseString(rawGraph)
	if err != nil {
		return nil, xerrors.Errorf("parse graph: %w", err)
	}
	graph, err := gographviz.NewAnalysedGraph(parsedGraph)
	if err != nil {
		return nil, xerrors.Errorf("analyze graph: %w", err)
	}

	converter := &stateConverter{
		ctx:                       ctx,
		logger:                    logger,
		graph:                     graph,
		tfResourcesByLabel:        make(map[string]map[string]*tfjson.StateResource),
		tfResourcesRichParameters: make([]*tfjson.StateResource, 0),
		tfResourcesPresets:        make([]*tfjson.StateResource, 0),
		tfResourcesAITasks:        make([]*tfjson.StateResource, 0),
		resources:                 make([]*proto.Resource, 0),
		resourceAgents:            make(map[string][]*proto.Agent),
	}

	// Index all resources from the state.
	converter.indexTerraformResources(modules)

	// Process agents first, as they are referenced by other resources.
	if err := converter.processAgents(); err != nil {
		return nil, err
	}

	// Associate agent instance IDs.
	converter.processAgentInstances()

	// Associate agent-related resources.
	if err := converter.processAgentApps(); err != nil {
		return nil, err
	}
	if err := converter.processAgentEnvs(); err != nil {
		return nil, err
	}
	if err := converter.processAgentScripts(); err != nil {
		return nil, err
	}
	if err := converter.processAgentDevcontainers(); err != nil {
		return nil, err
	}

	// Process resource metadata.
	if err := converter.processResourceMetadata(); err != nil {
		return nil, err
	}

	// Build the final resource list.
	converter.buildResources()

	// Process parameters and presets.
	parameters, err := converter.processParameters()
	if err != nil {
		return nil, err
	}
	presets, err := converter.processPresets(parameters)
	if err != nil {
		return nil, err
	}

	// Process AI tasks.
	aiTasks, err := converter.processAITasks()
	if err != nil {
		return nil, err
	}

	// Process external auth providers.
	externalAuthProviders, err := converter.processExternalAuthProviders()
	if err != nil {
		return nil, err
	}

	return &State{
		Resources:             converter.resources,
		Parameters:            parameters,
		Presets:               presets,
		ExternalAuthProviders: externalAuthProviders,
		HasAITasks:            hasAITaskResources(converter.graph),
		AITasks:               aiTasks,
		HasExternalAgents:     hasExternalAgentResources(converter.graph),
	}, nil
}

// indexTerraformResources recursively indexes all Terraform resources by label and categorizes special types.
func (c *stateConverter) indexTerraformResources(modules []*tfjson.StateModule) {
	var indexModule func(mod *tfjson.StateModule)
	indexModule = func(mod *tfjson.StateModule) {
		for _, module := range mod.ChildModules {
			indexModule(module)
		}
		for _, resource := range mod.Resources {
			// Categorize special resource types.
			switch resource.Type {
			case "coder_parameter":
				c.tfResourcesRichParameters = append(c.tfResourcesRichParameters, resource)
			case "coder_workspace_preset":
				c.tfResourcesPresets = append(c.tfResourcesPresets, resource)
			case "coder_ai_task":
				c.tfResourcesAITasks = append(c.tfResourcesAITasks, resource)
			}

			// Index by label for graph lookups.
			label := convertAddressToLabel(resource.Address)
			if c.tfResourcesByLabel[label] == nil {
				c.tfResourcesByLabel[label] = make(map[string]*tfjson.StateResource)
			}
			c.tfResourcesByLabel[label][resource.Address] = resource
		}
	}

	for _, module := range modules {
		indexModule(module)
	}
}

// processAgents finds all coder_agent resources, validates them, and associates them with their parent resources via the graph.
func (c *stateConverter) processAgents() error {
	agentNames := make(map[string]struct{})
	for _, tfResources := range c.tfResourcesByLabel {
		for _, tfResource := range tfResources {
			if tfResource.Type != "coder_agent" {
				continue
			}
			var attrs agentAttributes
			err := mapstructure.Decode(tfResource.AttributeValues, &attrs)
			if err != nil {
				return xerrors.Errorf("decode agent attributes: %w", err)
			}

			// Similar logic is duplicated in terraform/resources.go.
			if tfResource.Name == "" {
				return xerrors.Errorf("agent name cannot be empty")
			}
			// In 2025-02 we removed support for underscores in agent names. To
			// provide a nicer error message, we check the regex first and check
			// for underscores if it fails.
			if !provisioner.AgentNameRegex.MatchString(tfResource.Name) {
				if strings.Contains(tfResource.Name, "_") {
					return xerrors.Errorf("agent name %q contains underscores which are no longer supported, please use hyphens instead (regex: %q)", tfResource.Name, provisioner.AgentNameRegex.String())
				}
				return xerrors.Errorf("agent name %q does not match regex %q", tfResource.Name, provisioner.AgentNameRegex.String())
			}
			// Agent names must be case-insensitive-unique, to be unambiguous in
			// `coder_app`s and CoderVPN DNS names.
			if _, ok := agentNames[strings.ToLower(tfResource.Name)]; ok {
				return xerrors.Errorf("duplicate agent name: %s", tfResource.Name)
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
				ApiKeyScope:              attrs.APIKeyScope,
			}
			// Support the legacy script attributes in the agent!
			if attrs.StartupScript != "" {
				agent.Scripts = append(agent.Scripts, &proto.Script{
					// This is ▶️
					Icon:             "/emojis/25b6-fe0f.png",
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
			for _, node := range c.graph.Nodes.Lookup {
				// The node attributes surround the label with quotes.
				if strings.Trim(node.Attrs["label"], `"`) != agentLabel {
					continue
				}
				agentNode = node
				break
			}
			if agentNode == nil {
				return xerrors.Errorf("couldn't find node on graph: %q", agentLabel)
			}

			var agentResource *graphResource
			for _, resource := range findResourcesInGraph(c.graph, c.tfResourcesByLabel, agentNode.Name, 0, true) {
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

			agents, exists := c.resourceAgents[agentResource.Label]
			if !exists {
				agents = make([]*proto.Agent, 0, 1)
			}
			agents = append(agents, agent)
			c.resourceAgents[agentResource.Label] = agents
		}
	}
	return nil
}

// processAgentInstances associates instance IDs with agents that use instance-based authentication.
func (c *stateConverter) processAgentInstances() {
	for _, resources := range c.tfResourcesByLabel {
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

			for _, agents := range c.resourceAgents {
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
}

// processAgentApps associates coder_app resources with their agents.
func (c *stateConverter) processAgentApps() error {
	appSlugs := make(map[string]struct{})
	for _, resources := range c.tfResourcesByLabel {
		for _, resource := range resources {
			if resource.Type != "coder_app" {
				continue
			}

			var attrs agentAppAttributes
			err := mapstructure.Decode(resource.AttributeValues, &attrs)
			if err != nil {
				return xerrors.Errorf("decode app attributes: %w", err)
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
				return xerrors.Errorf("app slug %q does not match regex %q", attrs.Slug, provisioner.AppSlugRegex.String())
			}

			if _, exists := appSlugs[attrs.Slug]; exists {
				return xerrors.Errorf("duplicate app slug, they must be unique per template: %q", attrs.Slug)
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

			for _, agents := range c.resourceAgents {
				for _, agent := range agents {
					// Find agents with the matching ID and associate them!

					if !dependsOnAgent(c.graph, agent, attrs.AgentID, resource) {
						continue
					}

					id := attrs.ID
					if id == "" {
						// This should never happen since the "id" attribute is set on creation:
						// https://github.com/coder/terraform-provider-coder/blob/cfa101df4635e405e66094fa7779f9a89d92f400/provider/app.go#L37
						c.logger.Warn(c.ctx, "coder_app's id was unexpectedly empty", slog.F("name", attrs.Name))

						id = uuid.NewString()
					}

					agent.Apps = append(agent.Apps, &proto.App{
						Id:           id,
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
						Group:        attrs.Group,
						Hidden:       attrs.Hidden,
						OpenIn:       openIn,
						Tooltip:      attrs.Tooltip,
					})
				}
			}
		}
	}
	return nil
}

// processAgentEnvs associates coder_env resources with their agents.
func (c *stateConverter) processAgentEnvs() error {
	for _, resources := range c.tfResourcesByLabel {
		for _, resource := range resources {
			if resource.Type != "coder_env" {
				continue
			}
			var attrs agentEnvAttributes
			err := mapstructure.Decode(resource.AttributeValues, &attrs)
			if err != nil {
				return xerrors.Errorf("decode env attributes: %w", err)
			}
			for _, agents := range c.resourceAgents {
				for _, agent := range agents {
					// Find agents with the matching ID and associate them!
					if !dependsOnAgent(c.graph, agent, attrs.AgentID, resource) {
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
	return nil
}

// processAgentScripts associates coder_script resources with their agents.
func (c *stateConverter) processAgentScripts() error {
	for _, resources := range c.tfResourcesByLabel {
		for _, resource := range resources {
			if resource.Type != "coder_script" {
				continue
			}
			var attrs agentScriptAttributes
			err := mapstructure.Decode(resource.AttributeValues, &attrs)
			if err != nil {
				return xerrors.Errorf("decode script attributes: %w", err)
			}
			for _, agents := range c.resourceAgents {
				for _, agent := range agents {
					// Find agents with the matching ID and associate them!
					if !dependsOnAgent(c.graph, agent, attrs.AgentID, resource) {
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
	return nil
}

// processAgentDevcontainers associates coder_devcontainer resources with their agents.
func (c *stateConverter) processAgentDevcontainers() error {
	for _, resources := range c.tfResourcesByLabel {
		for _, resource := range resources {
			if resource.Type != "coder_devcontainer" {
				continue
			}
			var attrs agentDevcontainerAttributes
			err := mapstructure.Decode(resource.AttributeValues, &attrs)
			if err != nil {
				return xerrors.Errorf("decode script attributes: %w", err)
			}
			for _, agents := range c.resourceAgents {
				for _, agent := range agents {
					// Find agents with the matching ID and associate them!
					if !dependsOnAgent(c.graph, agent, attrs.AgentID, resource) {
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
	return nil
}

// processResourceMetadata associates coder_metadata resources with their target resources and collects metadata.
func (c *stateConverter) processResourceMetadata() error {
	resourceMetadata := make(map[string][]*proto.Resource_Metadata)
	resourceHidden := make(map[string]bool)
	resourceIcon := make(map[string]string)
	resourceCost := make(map[string]int32)

	metadataTargetLabels := make(map[string]bool)
	for _, resources := range c.tfResourcesByLabel {
		for _, resource := range resources {
			if resource.Type != "coder_metadata" {
				continue
			}

			var attrs resourceMetadataAttributes
			err := mapstructure.Decode(resource.AttributeValues, &attrs)
			if err != nil {
				return xerrors.Errorf("decode metadata attributes: %w", err)
			}
			resourceLabel := convertAddressToLabel(resource.Address)

			var attachedNode *gographviz.Node
			for _, node := range c.graph.Nodes.Lookup {
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
			for _, resource := range findResourcesInGraph(c.graph, c.tfResourcesByLabel, attachedNode.Name, 0, false) {
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
				return xerrors.Errorf("duplicate metadata resource: %s", targetLabel)
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

	// Store metadata for use in buildResources.
	c.resourceMetadata = resourceMetadata
	c.resourceHidden = resourceHidden
	c.resourceIcon = resourceIcon
	c.resourceCost = resourceCost
	return nil
}

// buildResources creates the final proto.Resource list from indexed resources and associated agents/metadata.
func (c *stateConverter) buildResources() {
	for _, tfResources := range c.tfResourcesByLabel {
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
				c.logger.Error(c.ctx, "failed to parse Terraform address", slog.F("address", resource.Address))
			}

			agents, exists := c.resourceAgents[label]
			if exists {
				applyAutomaticInstanceID(resource, agents)
			}

			c.resources = append(c.resources, &proto.Resource{
				Name:         resource.Name,
				Type:         resource.Type,
				Agents:       agents,
				Metadata:     c.resourceMetadata[label],
				Hide:         c.resourceHidden[label],
				Icon:         c.resourceIcon[label],
				DailyCost:    c.resourceCost[label],
				InstanceType: applyInstanceType(resource),
				ModulePath:   modulePath,
			})
		}
	}
}

// processParameters converts coder_parameter resources to proto.RichParameter.
func (c *stateConverter) processParameters() ([]*proto.RichParameter, error) {
	var duplicatedParamNames []string
	parameters := make([]*proto.RichParameter, 0)
	for _, resource := range c.tfResourcesRichParameters {
		var param provider.Parameter
		err := mapstructure.Decode(resource.AttributeValues, &param)
		if err != nil {
			return nil, xerrors.Errorf("decode map values for coder_parameter.%s: %w", resource.Name, err)
		}
		var defaultVal string
		if param.Default != nil {
			defaultVal = *param.Default
		}

		pft, err := proto.FormType(param.FormType)
		if err != nil {
			return nil, xerrors.Errorf("decode form_type for coder_parameter.%s: %w", resource.Name, err)
		}

		protoParam := &proto.RichParameter{
			Name:         param.Name,
			DisplayName:  param.DisplayName,
			Description:  param.Description,
			FormType:     pft,
			Type:         param.Type,
			Mutable:      param.Mutable,
			DefaultValue: defaultVal,
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

	return parameters, nil
}

// processPresets converts coder_workspace_preset resources to proto.Preset.
func (c *stateConverter) processPresets(parameters []*proto.RichParameter) ([]*proto.Preset, error) {
	var duplicatedPresetNames []string
	presets := make([]*proto.Preset, 0)
	for _, resource := range c.tfResourcesPresets {
		var preset provider.WorkspacePreset
		err := mapstructure.Decode(resource.AttributeValues, &preset)
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
			c.logger.Warn(
				c.ctx,
				"coder_workspace_preset defines preset values for at least one parameter that is not defined by the template",
				slog.F("parameters", stringutil.JoinWithConjunction(nonExistentParameters)),
			)
		}

		if len(preset.Prebuilds) != 1 {
			c.logger.Warn(
				c.ctx,
				"coder_workspace_preset must have exactly one prebuild block",
			)
		}
		var prebuildInstances int32
		var expirationPolicy *proto.ExpirationPolicy
		var scheduling *proto.Scheduling
		if len(preset.Prebuilds) > 0 {
			prebuildInstances = int32(math.Min(math.MaxInt32, float64(preset.Prebuilds[0].Instances)))
			if len(preset.Prebuilds[0].ExpirationPolicy) > 0 {
				expirationPolicy = &proto.ExpirationPolicy{
					Ttl: int32(math.Min(math.MaxInt32, float64(preset.Prebuilds[0].ExpirationPolicy[0].TTL))),
				}
			}
			if len(preset.Prebuilds[0].Scheduling) > 0 {
				scheduling = convertScheduling(preset.Prebuilds[0].Scheduling[0])
			}
		}
		protoPreset := &proto.Preset{
			Name:       preset.Name,
			Parameters: presetParameters,
			Prebuild: &proto.Prebuild{
				Instances:        prebuildInstances,
				ExpirationPolicy: expirationPolicy,
				Scheduling:       scheduling,
			},
			Default:     preset.Default,
			Description: preset.Description,
			Icon:        preset.Icon,
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

	// Validate that only one preset is marked as default.
	var defaultPresets int
	for _, preset := range presets {
		if preset.Default {
			defaultPresets++
		}
	}
	if defaultPresets > 1 {
		return nil, xerrors.Errorf("a maximum of 1 coder_workspace_preset can be marked as default, but %d are set", defaultPresets)
	}

	return presets, nil
}

// processAITasks converts coder_ai_task resources to proto.AITask.
func (c *stateConverter) processAITasks() ([]*proto.AITask, error) {
	// This will only pick up resources which will actually be created.
	aiTasks := make([]*proto.AITask, 0, len(c.tfResourcesAITasks))
	for _, resource := range c.tfResourcesAITasks {
		var task provider.AITask
		err := mapstructure.Decode(resource.AttributeValues, &task)
		if err != nil {
			return nil, xerrors.Errorf("decode coder_ai_task attributes: %w", err)
		}

		appID := task.AppID
		if appID == "" && len(task.SidebarApp) > 0 {
			appID = task.SidebarApp[0].ID
		}

		aiTasks = append(aiTasks, &proto.AITask{
			Id:    task.ID,
			AppId: appID,
			SidebarApp: &proto.AITaskSidebarApp{
				Id: appID,
			},
		})
	}

	return aiTasks, nil
}

// processExternalAuthProviders collects coder_external_auth resources.
func (c *stateConverter) processExternalAuthProviders() ([]*proto.ExternalAuthProviderResource, error) {
	// A map is used to ensure we don't have duplicates!
	externalAuthProvidersMap := make(map[string]*proto.ExternalAuthProviderResource)
	for _, tfResources := range c.tfResourcesByLabel {
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

	return externalAuthProviders, nil
}
