package terraform

import (
	"context"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/mitchellh/mapstructure"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/provisioner/terraform/scriptorder"
	"github.com/coder/coder/v2/provisionersdk/proto"
)

type scriptOrderRuntimeBindingInput struct {
	program *scriptorder.Program
}

type scriptOrderConversionResult struct {
	prepared       *scriptorder.Prepared
	scripts        map[string]scriptorder.Script
	scriptRecords  map[string]scriptOrderScriptRecord
	runtimeTargets map[string]scriptOrderRuntimeTarget
}

type scriptOrderScriptRecord struct {
	resource   *tfjson.StateResource
	attributes agentScriptAttributes
	selected   bool
}

type scriptOrderRuntimeTarget struct {
	workspaceAgent *proto.Agent
	devcontainer   *proto.Devcontainer
}

// scriptOrderRuntimeBinding records the Terraform and conversion objects used
// to associate selected scripts with agent runtimes. It is request-local.
type scriptOrderRuntimeBinding struct {
	prepared       *scriptorder.Prepared
	scripts        map[string]scriptorder.Script
	scriptRecords  map[string]scriptOrderScriptRecord
	runtimeTargets map[string]scriptOrderRuntimeTarget
}

func newScriptOrderRuntimeBinding(
	ctx context.Context,
	scriptResources []*tfjson.StateResource,
	input scriptOrderRuntimeBindingInput,
) (*scriptOrderRuntimeBinding, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if input.program == nil {
		return nil, xerrors.New("script order program is required")
	}

	scripts := make(map[string]scriptorder.Script, len(scriptResources))
	records := make(map[string]scriptOrderScriptRecord, len(scriptResources))
	for index, resource := range scriptResources {
		if index%256 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		if resource == nil || resource.Mode != tfjson.ManagedResourceMode {
			continue
		}
		var attributes agentScriptAttributes
		if err := mapstructure.Decode(
			resource.AttributeValues, &attributes,
		); err != nil {
			return nil, xerrors.Errorf(
				"decode script %q attributes: %w",
				resource.Address, err,
			)
		}
		records[resource.Address] = scriptOrderScriptRecord{
			resource: resource, attributes: attributes,
		}
		scripts[resource.Address] = scriptorder.Script{
			RunOnStart: attributes.RunOnStart,
			RunOnStop:  attributes.RunOnStop,
			Cron:       attributes.Cron,
		}
	}
	prepared, err := input.program.Prepare(scripts)
	if err != nil {
		return nil, err
	}
	for _, address := range prepared.SelectedScriptAddresses() {
		record := records[address]
		record.selected = true
		records[address] = record
	}

	return &scriptOrderRuntimeBinding{
		prepared:       prepared,
		scripts:        scripts,
		scriptRecords:  records,
		runtimeTargets: map[string]scriptOrderRuntimeTarget{},
	}, nil
}

func (b *scriptOrderRuntimeBinding) recordWorkspaceAgent(
	address string,
	agent *proto.Agent,
) {
	b.runtimeTargets[address] = scriptOrderRuntimeTarget{
		workspaceAgent: agent,
	}
}

func (b *scriptOrderRuntimeBinding) recordDevcontainer(
	address string,
	devcontainer *proto.Devcontainer,
) {
	b.runtimeTargets[address] = scriptOrderRuntimeTarget{
		devcontainer: devcontainer,
	}
}

func (b *scriptOrderRuntimeBinding) result() *scriptOrderConversionResult {
	return &scriptOrderConversionResult{
		prepared:       b.prepared,
		scripts:        b.scripts,
		scriptRecords:  b.scriptRecords,
		runtimeTargets: b.runtimeTargets,
	}
}
