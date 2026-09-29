package terraform

import (
	"context"
	"fmt"
	"slices"
	"strings"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/mitchellh/mapstructure"
	"golang.org/x/xerrors"

	stringutil "github.com/coder/coder/v2/coderd/util/strings"
	"github.com/coder/coder/v2/provisioner/terraform/scriptorder"
	"github.com/coder/coder/v2/provisionersdk/proto"
)

type scriptOrderConversionSource int

const (
	_ scriptOrderConversionSource = iota
	scriptOrderConversionSourceState
)

type scriptOrderRuntimeBindingInput struct {
	source  scriptOrderConversionSource
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

	workspaceAddressesByID    map[string][]string
	devcontainerAddressesByID map[string][]string
}

func newScriptOrderRuntimeBinding(
	ctx context.Context,
	scriptResources []*tfjson.StateResource,
	input scriptOrderRuntimeBindingInput,
) (*scriptOrderRuntimeBinding, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if input.source != scriptOrderConversionSourceState {
		return nil, xerrors.Errorf(
			"unknown script order conversion source %d", input.source,
		)
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

		workspaceAddressesByID:    map[string][]string{},
		devcontainerAddressesByID: map[string][]string{},
	}, nil
}

func (b *scriptOrderRuntimeBinding) recordWorkspaceAgent(
	address string,
	agent *proto.Agent,
) {
	b.runtimeTargets[address] = scriptOrderRuntimeTarget{
		workspaceAgent: agent,
	}
	if agent.Id != "" {
		b.workspaceAddressesByID[agent.Id] = append(
			b.workspaceAddressesByID[agent.Id], address,
		)
	}
}

func (b *scriptOrderRuntimeBinding) recordDevcontainer(
	address string,
	devcontainer *proto.Devcontainer,
) {
	b.runtimeTargets[address] = scriptOrderRuntimeTarget{
		devcontainer: devcontainer,
	}
	if devcontainer.SubagentId != "" {
		b.devcontainerAddressesByID[devcontainer.SubagentId] = append(
			b.devcontainerAddressesByID[devcontainer.SubagentId], address,
		)
	}
}

func (b *scriptOrderRuntimeBinding) resolveSelectedScriptRuntimes(
	ctx context.Context,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	for index, address := range b.prepared.SelectedScriptAddresses() {
		if index%256 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		runtimeID := b.scriptRecords[address].attributes.AgentID
		if runtimeID == "" {
			continue
		}
		runtimeAddress, err := b.runtimeAddressForID(runtimeID)
		if err != nil {
			b.setScriptRuntimeError(address, err)
			continue
		}
		b.setScriptRuntimeAddress(address, runtimeAddress)
	}
	return nil
}

func (b *scriptOrderRuntimeBinding) runtimeAddressForID(
	runtimeID string,
) (string, error) {
	candidates := slices.Concat(
		b.workspaceAddressesByID[runtimeID],
		b.devcontainerAddressesByID[runtimeID],
	)
	slices.Sort(candidates)
	switch len(candidates) {
	case 1:
		return candidates[0], nil
	case 0:
		return "", xerrors.New(
			"agent_id does not match any workspace agent or devcontainer subagent",
		)
	default:
		return "", xerrors.Errorf(
			"agent_id matches multiple agent runtimes: %s",
			formatScriptOrderRuntimeCandidateAddresses(candidates),
		)
	}
}

func (b *scriptOrderRuntimeBinding) setScriptRuntimeAddress(
	scriptAddress string,
	runtimeAddress string,
) {
	script := b.scripts[scriptAddress]
	script.RuntimeAddress = runtimeAddress
	script.RuntimeError = ""
	b.scripts[scriptAddress] = script
}

func (b *scriptOrderRuntimeBinding) setScriptRuntimeError(
	scriptAddress string,
	err error,
) {
	script := b.scripts[scriptAddress]
	script.RuntimeError = err.Error()
	b.scripts[scriptAddress] = script
}

func (b *scriptOrderRuntimeBinding) handleSelectedScript(
	resource *tfjson.StateResource,
	script *proto.Script,
) bool {
	if resource.Mode != tfjson.ManagedResourceMode {
		return false
	}
	record, exists := b.scriptRecords[resource.Address]
	if !exists || !record.selected {
		return false
	}
	facts := b.scripts[resource.Address]
	if facts.RuntimeError != "" || facts.RuntimeAddress == "" {
		return true
	}
	target := b.runtimeTargets[facts.RuntimeAddress]
	if target.workspaceAgent != nil {
		target.workspaceAgent.Scripts = append(target.workspaceAgent.Scripts, script)
		return true
	}
	if target.devcontainer != nil {
		target.devcontainer.Scripts = append(target.devcontainer.Scripts, script)
		return true
	}
	facts.RuntimeError = fmt.Sprintf(
		"runtime %q was not associated with a converted workspace agent or devcontainer subagent",
		stringutil.Truncate(facts.RuntimeAddress, 256, stringutil.TruncateWithEllipsis),
	)
	b.scripts[resource.Address] = facts
	return true
}

func formatScriptOrderRuntimeCandidateAddresses(addresses []string) string {
	const maxCandidates = 20

	rendered := min(len(addresses), maxCandidates)
	values := make([]string, 0, rendered+1)
	for _, address := range addresses[:rendered] {
		values = append(values, stringutil.Truncate(
			address, 256, stringutil.TruncateWithEllipsis,
		))
	}
	if omitted := len(addresses) - rendered; omitted > 0 {
		values = append(values, fmt.Sprintf("… (%d omitted)", omitted))
	}
	return strings.Join(values, ", ")
}

func (b *scriptOrderRuntimeBinding) result() *scriptOrderConversionResult {
	return &scriptOrderConversionResult{
		prepared:       b.prepared,
		scripts:        b.scripts,
		scriptRecords:  b.scriptRecords,
		runtimeTargets: b.runtimeTargets,
	}
}
