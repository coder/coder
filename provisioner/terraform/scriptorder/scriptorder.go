// Package scriptorder resolves Terraform coder_script_order data sources into
// deterministic dependency graphs for coder_script resources.
//
// Usage:
//
//	program, err := NewProgram(ctx, modules, config)
//	program = program.FilterDataSources(retainDataSource)
//	prepared, err := program.Prepare()
//	runtimeBindings := resolveRuntimes(prepared.SelectedScriptAddresses())
//	order, err := prepared.Finalize(runtimeBindings)
//
// resolveRuntimes represents caller-owned resolution. Runtime binding remains
// outside this package because plan-time resolution requires Terraform graph,
// expression provenance and runtime information.
//
// Selector validation distinguishes declared-but-empty resources from invalid
// selectors.
// Graph construction limits dependency combinations, deduplicates dependencies,
// and rejects conflicting requirements and cycles.
package scriptorder

import (
	"context"
	"slices"

	tfjson "github.com/hashicorp/terraform-json"
	"golang.org/x/xerrors"
)

const (
	coderScriptResourceType      = "coder_script"
	coderScriptOrderResourceType = "coder_script_order"
)

// Program contains the immutable indexes of Terraform state and config
// for script ordering. It is the input to script-order preparation.
type Program struct {
	stateIndex  *stateIndex
	configIndex *configIndex
	// dataSources contains coder_script_order instances retained for preparation.
	dataSources []dataSource
}

// NewProgram indexes Terraform state and configuration for script
// ordering. It returns nil when the supplied values contain no
// coder_script_order instances declared by the current configuration,
// including when all declarations expand to zero instances or only
// stale prior-state instances are present.
func NewProgram(
	ctx context.Context,
	modules []*tfjson.StateModule,
	config *tfjson.Config,
) (*Program, error) {
	stateIndex, err := newStateIndex(ctx, modules)
	if err != nil {
		return nil, err
	}
	if len(stateIndex.dataSources) == 0 {
		//nolint:nilnil // No data source instances disables script ordering.
		return nil, nil
	}
	configIndex, err := newConfigIndex(ctx, config)
	if err != nil {
		return nil, err
	}

	dataSources := make([]dataSource, 0, len(stateIndex.dataSources))
	for _, dataSource := range stateIndex.dataSources {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		// Plan-derived state may retain prior-state data sources
		// whose configuration declarations were removed. Processing
		// these stale instances could preserve removed ordering rules
		// or produce preparation errors.
		declaration, err := configIndex.lookupResource(
			dataSource.moduleAddress,
			tfjson.DataResourceMode,
			dataSource.resource.Type,
			dataSource.resource.Name,
		)
		if err != nil {
			return nil, xerrors.Errorf(
				"validate script order data source %q: %w",
				dataSource.resource.Address, err,
			)
		}
		if declaration != nil {
			dataSources = append(dataSources, dataSource)
		}
	}
	if len(dataSources) == 0 {
		//nolint:nilnil // No configured data sources disables script ordering.
		return nil, nil
	}
	return &Program{
		stateIndex:  stateIndex,
		configIndex: configIndex,
		dataSources: dataSources,
	}, nil
}

// DataSourceAddresses returns the active coder_script_order data source
// instances in deterministic address order.
func (p *Program) DataSourceAddresses() []string {
	if p == nil {
		return nil
	}
	addresses := make([]string, 0, len(p.dataSources))
	for _, dataSource := range p.dataSources {
		addresses = append(addresses, dataSource.resource.Address)
	}
	return addresses
}

// FilterDataSources returns an immutable Program containing only data source
// instances accepted by retain. Plan-time callers derive retain from the
// saved-plan graph, which identifies destroy-only instances that plan-derived
// state cannot distinguish. It returns nil when none remain.
func (p *Program) FilterDataSources(retain func(address string) bool) *Program {
	if p == nil {
		return nil
	}
	dataSources := make([]dataSource, 0, len(p.dataSources))
	for _, dataSource := range p.dataSources {
		if retain(dataSource.resource.Address) {
			dataSources = append(dataSources, dataSource)
		}
	}
	if len(dataSources) == 0 {
		return nil
	}
	if len(dataSources) == len(p.dataSources) {
		return p
	}
	return &Program{
		stateIndex:  p.stateIndex,
		configIndex: p.configIndex,
		dataSources: dataSources,
	}
}

// RuntimeBinding contains the resolved runtime information needed to
// finalize script-order rules for one coder_script instance.
type RuntimeBinding struct {
	// RuntimeAddress identifies the workspace agent or devcontainer subagent
	// that executes the script, such as "coder_agent.main[0]".
	RuntimeAddress string
}

// Prepared contains phase-filtered rules awaiting runtime validation.
type Prepared struct {
	rules []preparedRule
	// selectedScriptAddresses is the sorted, deduplicated union of
	// run and after addresses. It is precomputed because multiple
	// runtime-binding stages consume it.
	selectedScriptAddresses []string
	warnings                []phaseFilterWarning
}

// Prepare resolves selectors and phases within rules.
func (p *Program) Prepare() (*Prepared, error) {
	prepared, err := p.prepareOrder()
	if err != nil {
		return nil, err
	}
	return &prepared, nil
}

// SelectedScriptAddresses returns the scripts that require runtime binding.
func (p *Prepared) SelectedScriptAddresses() []string {
	if p == nil {
		return nil
	}
	return slices.Clone(p.selectedScriptAddresses)
}

// Warnings returns deterministic phase-filter warnings.
func (p *Prepared) Warnings() []string {
	if p == nil {
		return nil
	}
	warnings := make([]string, 0, len(p.warnings))
	for _, warning := range p.warnings {
		warnings = append(warnings, warning.String())
	}
	return warnings
}

// Finalize validates bound runtimes and constructs deterministic graphs.
// runtimeBindings maps each selected coder_script instance address to its
// resolved runtime information.
func (p *Prepared) Finalize(
	runtimeBindings map[string]RuntimeBinding,
) (Order, error) {
	if p == nil {
		return Order{}, nil
	}
	resolved, err := p.finalizeOrder(runtimeBindings)
	if err != nil {
		return Order{}, err
	}
	return buildGraphs(resolved)
}
