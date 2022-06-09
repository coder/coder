package terraform

import (
	"strings"

	"github.com/awalterschulze/gographviz"
	tfjson "github.com/hashicorp/terraform-json"
	"github.com/mitchellh/mapstructure"
	"golang.org/x/xerrors"

	"github.com/coder/coder/provisionersdk/proto"
)

// ConvertResources consumes Terraform state and a GraphViz representation produced by
// `terraform graph` to produce resources consumable by Coder.
func ConvertResources(module *tfjson.StateModule, rawGraph string) ([]*proto.Resource, error) {
	parsedGraph, err := gographviz.ParseString(rawGraph)
	if err != nil {
		return nil, xerrors.Errorf("parse graph: %w", err)
	}
	graph, err := gographviz.NewAnalysedGraph(parsedGraph)
	if err != nil {
		return nil, xerrors.Errorf("analyze graph: %w", err)
	}
	resourceDependencies := map[string][]string{}
	for _, node := range graph.Nodes.Nodes {
		label, exists := node.Attrs["label"]
		if !exists {
			continue
		}
		label = strings.Trim(label, `"`)
		resourceDependencies[label] = findDependenciesWithLabels(graph, node.Name)
	}

	resources := make([]*proto.Resource, 0)
	agents := map[string]*proto.Agent{}

	tfResources := make([]*tfjson.StateResource, 0)
	var appendResources func(mod *tfjson.StateModule)
	appendResources = func(mod *tfjson.StateModule) {
		for _, module := range mod.ChildModules {
			appendResources(module)
		}
		tfResources = append(tfResources, mod.Resources...)
	}
	appendResources(module)

	type agentAttributes struct {
		Auth            string            `mapstructure:"auth"`
		OperatingSystem string            `mapstructure:"os"`
		Architecture    string            `mapstructure:"arch"`
		Directory       string            `mapstructure:"dir"`
		ID              string            `mapstructure:"id"`
		Token           string            `mapstructure:"token"`
		Env             map[string]string `mapstructure:"env"`
		StartupScript   string            `mapstructure:"startup_script"`
	}

	// Store all agents inside the maps!
	for _, resource := range tfResources {
		if resource.Type != "coder_agent" {
			continue
		}
		var attrs agentAttributes
		err = mapstructure.Decode(resource.AttributeValues, &attrs)
		if err != nil {
			return nil, xerrors.Errorf("decode agent attributes: %w", err)
		}
		agent := &proto.Agent{
			Name:            resource.Name,
			Id:              attrs.ID,
			Env:             attrs.Env,
			StartupScript:   attrs.StartupScript,
			OperatingSystem: attrs.OperatingSystem,
			Architecture:    attrs.Architecture,
			Directory:       attrs.Directory,
		}
		switch attrs.Auth {
		case "token":
			agent.Auth = &proto.Agent_Token{
				Token: attrs.Token,
			}
		default:
			agent.Auth = &proto.Agent_InstanceId{}
		}

		agents[convertAddressToLabel(resource.Address)] = agent
	}

	// Manually associate agents with instance IDs.
	for _, resource := range tfResources {
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

		for _, agent := range agents {
			if agent.Id != agentID {
				continue
			}
			agent.Auth = &proto.Agent_InstanceId{
				InstanceId: instanceID,
			}
			break
		}
	}

	for _, resource := range tfResources {
		if resource.Mode == tfjson.DataResourceMode {
			continue
		}
		if resource.Type == "coder_agent" || resource.Type == "coder_agent_instance" || resource.Type == "coder_app" {
			continue
		}
		resources = append(resources, &proto.Resource{
			Name:   resource.Name,
			Type:   resource.Type,
			Agents: findAgents(resourceDependencies, agents, convertAddressToLabel(resource.Address)),
		})
	}

	return resources, nil
}

// convertAddressToLabel returns the Terraform address without the count
// specifier. eg. "module.ec2_dev.ec2_instance.dev[0]" becomes "module.ec2_dev.ec2_instance.dev"
func convertAddressToLabel(address string) string {
	return strings.Split(address, "[")[0]
}

// findAgents recursively searches through resource dependencies
// to find associated agents. Nested is required for indirect
// dependency matching.
func findAgents(resourceDependencies map[string][]string, agents map[string]*proto.Agent, resourceLabel string) []*proto.Agent {
	resourceNode, exists := resourceDependencies[resourceLabel]
	if !exists {
		return []*proto.Agent{}
	}
	// Associate resources that depend on an agent.
	resourceAgents := make([]*proto.Agent, 0)
	for _, dep := range resourceNode {
		var has bool
		agent, has := agents[dep]
		if !has {
			resourceAgents = append(resourceAgents, findAgents(resourceDependencies, agents, dep)...)
			continue
		}
		// An agent must be deleted after being assigned so it isn't referenced twice.
		delete(agents, dep)
		resourceAgents = append(resourceAgents, agent)
	}
	return resourceAgents
}

// findDependenciesWithLabels recursively finds nodes with labels (resource and data nodes)
// to build a dependency tree.
func findDependenciesWithLabels(graph *gographviz.Graph, nodeName string) []string {
	dependencies := make([]string, 0)
	for destination := range graph.Edges.SrcToDsts[nodeName] {
		dependencyNode, exists := graph.Nodes.Lookup[destination]
		if !exists {
			continue
		}
		label, exists := dependencyNode.Attrs["label"]
		if !exists {
			dependencies = append(dependencies, findDependenciesWithLabels(graph, dependencyNode.Name)...)
			continue
		}
		label = strings.Trim(label, `"`)
		dependencies = append(dependencies, label)
	}
	return dependencies
}
