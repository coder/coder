package terraform

import (
	"context"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/mitchellh/mapstructure"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/provisioner/terraform/scriptorder"
)

// scriptOrderAgentCatalog maps applied agent and subagent IDs to their
// Terraform resource addresses. Devcontainer subagents use coder_devcontainer
// addresses because they do not have separate agent resources. Applied state
// should contain unique IDs. A missing entry means no resource matched; an
// empty address defensively marks multiple matches so binding fails instead of
// selecting one arbitrarily.
type scriptOrderAgentCatalog map[string]string

func (c scriptOrderAgentCatalog) index(id, address string) {
	if c == nil || id == "" {
		return
	}
	indexedAddress, ok := c[id]
	if ok && indexedAddress != address {
		c[id] = ""
		return
	}
	c[id] = address
}

// resolveAppliedAgentBindings associates selected scripts with applied
// workspace agents or devcontainer subagents.
func resolveAppliedAgentBindings(
	ctx context.Context,
	modules []*tfjson.StateModule,
	agentCatalog scriptOrderAgentCatalog,
	selectedAddresses []string,
) (map[string]scriptorder.RuntimeBinding, error) {
	// Check cancellation in batches to limit post-cancellation work without
	// performing a context lookup for every Terraform resource.
	const contextCheckInterval = 256

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	selected := make(map[string]struct{}, len(selectedAddresses))
	for _, address := range selectedAddresses {
		selected[address] = struct{}{}
	}
	if len(selected) == 0 {
		return map[string]scriptorder.RuntimeBinding{}, nil
	}
	agentIDsByScriptAddress := make(map[string]string, len(selected))
	visited := 0
	for _, module := range modules {
		if module == nil {
			continue
		}
		err := forEachResource(module, func(resource *tfjson.StateResource) error {
			visited++
			if visited%contextCheckInterval == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			if resource == nil || resource.Mode != tfjson.ManagedResourceMode {
				return nil
			}

			if resource.Type != "coder_script" {
				return nil
			}
			if _, ok := selected[resource.Address]; !ok {
				return nil
			}
			var attrs agentScriptAttributes
			err := mapstructure.Decode(resource.AttributeValues, &attrs)
			if err != nil {
				return xerrors.Errorf(
					"decode script %q attributes: %w",
					resource.Address, err,
				)
			}
			agentIDsByScriptAddress[resource.Address] = attrs.AgentID
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	bindings := make(map[string]scriptorder.RuntimeBinding, len(selected))
	bindingsByAgentID := map[string]scriptorder.RuntimeBinding{}
	for idx, address := range selectedAddresses {
		if idx%contextCheckInterval == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		agentID, found := agentIDsByScriptAddress[address]
		switch {
		case !found:
			bindings[address] = scriptorder.RuntimeBinding{
				Error: "script was not found in applied Terraform state",
			}
		case agentID == "":
			bindings[address] = scriptorder.RuntimeBinding{
				Error: "agent_id is empty",
			}
		default:
			binding, ok := bindingsByAgentID[agentID]
			if !ok {
				agentAddress, found := agentCatalog[agentID]
				binding = appliedAgentBinding(agentAddress, found)
				bindingsByAgentID[agentID] = binding
			}
			bindings[address] = binding
		}
	}
	return bindings, nil
}

func appliedAgentBinding(
	agentAddress string, found bool,
) scriptorder.RuntimeBinding {
	switch {
	case !found:
		return scriptorder.RuntimeBinding{
			Error: "agent_id does not match any workspace agent or devcontainer subagent",
		}
	case agentAddress == "":
		return scriptorder.RuntimeBinding{
			Error: "agent_id matches multiple workspace agents or devcontainer subagents",
		}
	default:
		return scriptorder.RuntimeBinding{RuntimeAddress: agentAddress}
	}
}
