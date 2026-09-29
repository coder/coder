package terraform

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/mitchellh/mapstructure"
	"golang.org/x/xerrors"

	stringutil "github.com/coder/coder/v2/coderd/util/strings"
	"github.com/coder/coder/v2/provisioner/terraform/agentruntime"
	"github.com/coder/coder/v2/provisioner/terraform/scriptorder"
	"github.com/coder/coder/v2/provisioner/terraform/tfgraph"
	"github.com/coder/coder/v2/provisionersdk/proto"
)

type scriptOrderConversionSource int

const (
	_ scriptOrderConversionSource = iota
	scriptOrderConversionSourcePlan
	scriptOrderConversionSourceState
)

type scriptOrderRuntimeBindingInput struct {
	source                scriptOrderConversionSource
	program               *scriptorder.Program
	runtimeProgram        *agentruntime.Program
	prepared              *scriptorder.Prepared
	preparationErr        error
	planGraph             *tfgraph.Index
	loadRuntimeProvenance func(context.Context) error
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

type scriptOrderDevcontainerRecord struct {
	attributes agentDevcontainerAttributes
}

type scriptOrderRuntimeTarget struct {
	workspaceAgent *proto.Agent
	devcontainer   *proto.Devcontainer
}

// scriptOrderRuntimeBinding records the Terraform and conversion objects used
// to associate selected scripts with agent runtimes. It is request-local.
type scriptOrderRuntimeBinding struct {
	source                scriptOrderConversionSource
	runtimeProgram        *agentruntime.Program
	planGraph             *tfgraph.Index
	loadRuntimeProvenance func(context.Context) error

	prepared       *scriptorder.Prepared
	scripts        map[string]scriptorder.Script
	scriptRecords  map[string]scriptOrderScriptRecord
	runtimeTargets map[string]scriptOrderRuntimeTarget

	selectedDevcontainers     map[string]struct{}
	devcontainerRecords       map[string]scriptOrderDevcontainerRecord
	workspaceAddressesByID    map[string][]string
	devcontainerAddressesByID map[string][]string
	devcontainerRuntimeErrors map[string]string
	resolver                  *agentruntime.Resolver
}

func newScriptOrderRuntimeBinding(
	ctx context.Context,
	scriptResources []*tfjson.StateResource,
	input scriptOrderRuntimeBindingInput,
) (*scriptOrderRuntimeBinding, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	switch input.source {
	case scriptOrderConversionSourcePlan:
		if input.runtimeProgram == nil {
			return nil, xerrors.New(
				"agent runtime program is required for plan-time script ordering",
			)
		}
	case scriptOrderConversionSourceState:
		if input.planGraph != nil {
			return nil, xerrors.New(
				"saved plan graph must not be used for post-apply script ordering",
			)
		}
	default:
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
	if input.preparationErr != nil {
		return nil, input.preparationErr
	}
	prepared := input.prepared
	if prepared == nil {
		var err error
		prepared, err = input.program.Prepare(scripts)
		if err != nil {
			return nil, err
		}
	}
	for _, address := range prepared.SelectedScriptAddresses() {
		record := records[address]
		record.selected = true
		records[address] = record
	}

	return &scriptOrderRuntimeBinding{
		source:                input.source,
		runtimeProgram:        input.runtimeProgram,
		planGraph:             input.planGraph,
		loadRuntimeProvenance: input.loadRuntimeProvenance,

		prepared:       prepared,
		scripts:        scripts,
		scriptRecords:  records,
		runtimeTargets: map[string]scriptOrderRuntimeTarget{},

		selectedDevcontainers:     map[string]struct{}{},
		devcontainerRecords:       map[string]scriptOrderDevcontainerRecord{},
		workspaceAddressesByID:    map[string][]string{},
		devcontainerAddressesByID: map[string][]string{},
		devcontainerRuntimeErrors: map[string]string{},
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
}

func (b *scriptOrderRuntimeBinding) resolveSelectedScriptRuntimes(
	ctx context.Context,
	devcontainerResources []*tfjson.StateResource,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	devcontainerAddresses := make([]string, 0, len(devcontainerResources))
	for index, resource := range devcontainerResources {
		if index%256 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		if resource == nil || resource.Mode != tfjson.ManagedResourceMode {
			continue
		}
		var attributes agentDevcontainerAttributes
		if err := mapstructure.Decode(
			resource.AttributeValues, &attributes,
		); err != nil {
			return xerrors.Errorf(
				"decode devcontainer %q attributes: %w", resource.Address, err,
			)
		}
		b.devcontainerRecords[resource.Address] = scriptOrderDevcontainerRecord{
			attributes: attributes,
		}
		devcontainerAddresses = append(devcontainerAddresses, resource.Address)
		if attributes.SubAgentID != "" {
			b.devcontainerAddressesByID[attributes.SubAgentID] = append(
				b.devcontainerAddressesByID[attributes.SubAgentID],
				resource.Address,
			)
		}
	}

	selectedAddresses := b.prepared.SelectedScriptAddresses()
	unresolved := make([]string, 0, len(selectedAddresses))
	for index, address := range selectedAddresses {
		if index%256 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		attributes := b.scriptRecords[address].attributes
		if attributes.AgentID == "" {
			if b.source == scriptOrderConversionSourcePlan {
				unresolved = append(unresolved, address)
			}
			continue
		}
		runtimeAddress, err := b.runtimeAddressForID(attributes.AgentID)
		if err != nil {
			b.setScriptRuntimeError(address, err)
			continue
		}
		b.setScriptRuntimeAddress(address, runtimeAddress)
	}

	needsResolver := len(unresolved) > 0
	if !needsResolver && b.source == scriptOrderConversionSourcePlan {
		for address := range b.selectedDevcontainers {
			if b.devcontainerRecords[address].attributes.AgentID == "" {
				needsResolver = true
				break
			}
		}
	}
	if needsResolver {
		if b.planGraph == nil {
			return xerrors.New(
				"saved plan graph is required for plan-time script ordering",
			)
		}
		if b.loadRuntimeProvenance != nil {
			if err := b.loadRuntimeProvenance(ctx); err != nil {
				return xerrors.Errorf("load agent runtime provenance: %w", err)
			}
		}
		targets := make(
			[]agentruntime.Target,
			0,
			len(b.runtimeTargets)+len(devcontainerAddresses),
		)
		for _, address := range slices.Sorted(maps.Keys(b.runtimeTargets)) {
			target := b.runtimeTargets[address]
			switch {
			case target.workspaceAgent != nil:
				targets = append(targets, agentruntime.Target{
					Kind: agentruntime.KindWorkspaceAgent, Address: address,
				})
			case target.devcontainer != nil:
				targets = append(targets, agentruntime.Target{
					Kind: agentruntime.KindDevcontainer, Address: address,
				})
			}
		}
		for _, address := range devcontainerAddresses {
			targets = append(targets, agentruntime.Target{
				Kind: agentruntime.KindDevcontainer, Address: address,
			})
		}
		resolver, err := agentruntime.NewResolver(
			ctx, b.planGraph, b.runtimeProgram, targets,
		)
		if err != nil {
			return err
		}
		b.resolver = resolver
	}

	for _, address := range unresolved {
		if err := ctx.Err(); err != nil {
			return err
		}
		runtimeTarget, err := b.resolver.ResolveResourceRuntime(
			ctx, b.scriptRecords[address].resource,
		)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			b.setScriptRuntimeError(address, err)
			continue
		}
		b.setScriptRuntimeAddress(address, runtimeTarget.Address)
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
	if _, ok := b.devcontainerRecords[runtimeAddress]; ok {
		b.selectedDevcontainers[runtimeAddress] = struct{}{}
	}
}

func (b *scriptOrderRuntimeBinding) setScriptRuntimeError(
	scriptAddress string,
	err error,
) {
	script := b.scripts[scriptAddress]
	script.RuntimeError = err.Error()
	b.scripts[scriptAddress] = script
}

func (b *scriptOrderRuntimeBinding) workspaceAgentForDevcontainer(
	ctx context.Context,
	resource *tfjson.StateResource,
	agentID string,
) (*proto.Agent, bool, error) {
	if _, selected := b.selectedDevcontainers[resource.Address]; !selected {
		return nil, false, nil
	}

	var (
		address string
		err     error
	)
	if agentID != "" {
		candidates := slices.Clone(b.workspaceAddressesByID[agentID])
		slices.Sort(candidates)
		switch len(candidates) {
		case 1:
			address = candidates[0]
		case 0:
			err = xerrors.Errorf(
				"devcontainer %q agent_id does not match any workspace agent",
				stringutil.Truncate(resource.Address, 256, stringutil.TruncateWithEllipsis),
			)
		default:
			err = xerrors.Errorf(
				"devcontainer %q agent_id matches multiple workspace agents: %s",
				stringutil.Truncate(resource.Address, 256, stringutil.TruncateWithEllipsis),
				formatScriptOrderRuntimeCandidateAddresses(candidates),
			)
		}
	} else if b.source == scriptOrderConversionSourcePlan {
		if b.resolver == nil {
			return nil, true, xerrors.New(
				"planned devcontainer runtime resolver is unavailable",
			)
		}
		runtimeTarget, resolveErr := b.resolver.ResolveWorkspaceAgent(ctx, resource)
		address, err = runtimeTarget.Address, resolveErr
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, true, ctxErr
		}
	} else {
		err = xerrors.Errorf(
			"devcontainer %q agent_id does not match any workspace agent",
			stringutil.Truncate(resource.Address, 256, stringutil.TruncateWithEllipsis),
		)
	}
	if err != nil {
		b.devcontainerRuntimeErrors[resource.Address] = err.Error()
		return nil, true, nil
	}
	target := b.runtimeTargets[address]
	if target.workspaceAgent == nil {
		b.devcontainerRuntimeErrors[resource.Address] = fmt.Sprintf(
			"devcontainer %q resolves to unavailable workspace agent %q",
			stringutil.Truncate(resource.Address, 256, stringutil.TruncateWithEllipsis),
			stringutil.Truncate(address, 256, stringutil.TruncateWithEllipsis),
		)
		return nil, true, nil
	}
	return target.workspaceAgent, true, nil
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
	if runtimeError := b.devcontainerRuntimeErrors[facts.RuntimeAddress]; runtimeError != "" {
		facts.RuntimeError = runtimeError
		b.scripts[resource.Address] = facts
	}
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
