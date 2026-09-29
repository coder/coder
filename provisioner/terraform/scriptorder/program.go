package scriptorder

import (
	"context"
	"slices"

	tfjson "github.com/hashicorp/terraform-json"
	"golang.org/x/xerrors"
)

// Program is the immutable Terraform-derived input to script-order
// preparation.
type Program struct {
	stateIndex  *scriptOrderStateIndex
	configIndex *scriptOrderConfigIndex
	dataSources []scriptOrderDataSource
}

// NewProgram indexes Terraform state and configuration for script ordering.
// It returns nil when the state has no active script-order declarations.
func NewProgram(
	ctx context.Context,
	modules []*tfjson.StateModule,
	config *tfjson.Config,
) (*Program, error) {
	stateIndex, err := newScriptOrderStateIndex(ctx, modules)
	if err != nil {
		return nil, err
	}
	if len(stateIndex.dataSources) == 0 {
		return nil, nil //nolint:nilnil // No declarations disables script ordering.
	}
	configIndex, err := newScriptOrderConfigIndex(ctx, config)
	if err != nil {
		return nil, err
	}
	if configIndex == nil {
		return nil, xerrors.New("Terraform plan configuration is unavailable")
	}

	dataSources := make([]scriptOrderDataSource, 0, len(stateIndex.dataSources))
	for _, dataSource := range stateIndex.dataSources {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		declared, err := configIndex.dataResourceDeclared(
			dataSource.moduleAddress,
			dataSource.resource.Type,
			dataSource.resource.Name,
		)
		if err != nil {
			return nil, xerrors.Errorf(
				"validate script order data source %q: %w",
				dataSource.address, err,
			)
		}
		if declared {
			dataSources = append(dataSources, dataSource)
		}
	}
	if len(dataSources) == 0 {
		return nil, nil //nolint:nilnil // Removed declarations disable script ordering.
	}
	return &Program{
		stateIndex:  stateIndex,
		configIndex: configIndex,
		dataSources: dataSources,
	}, nil
}

// DataSourceAddresses returns the active script-order data source instances in
// deterministic address order.
func (p *Program) DataSourceAddresses() []string {
	if p == nil {
		return nil
	}
	addresses := make([]string, 0, len(p.dataSources))
	for _, dataSource := range p.dataSources {
		addresses = append(addresses, dataSource.address)
	}
	return addresses
}

// FilterDataSources returns an immutable Program containing only data source
// instances accepted by retain. It returns nil when none remain.
func (p *Program) FilterDataSources(retain func(address string) bool) *Program {
	if p == nil {
		return nil
	}
	dataSources := make([]scriptOrderDataSource, 0, len(p.dataSources))
	for _, dataSource := range p.dataSources {
		if retain(dataSource.address) {
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

// Script contains the lifecycle and runtime information needed to prepare and
// finalize script-order rules.
type Script struct {
	RuntimeAddress string
	RuntimeError   string
	RunOnStart     bool
	RunOnStop      bool
	Cron           string
}

// Prepared contains phase-filtered rules awaiting runtime validation.
type Prepared struct {
	preparedScriptOrder
}

// Prepare resolves selectors and lifecycle phases once.
func (p *Program) Prepare(scripts map[string]Script) (*Prepared, error) {
	prepared, err := prepareScriptOrder(p, internalScriptOrderScripts(scripts))
	if err != nil {
		return nil, err
	}
	return &Prepared{preparedScriptOrder: prepared}, nil
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

// WarningsOmitted returns the number of phase-filter warnings omitted after
// the request-local warning limit was reached.
func (p *Prepared) WarningsOmitted() int {
	if p == nil {
		return 0
	}
	return p.warningsOmitted
}

// Finalize validates bound runtimes and constructs deterministic graphs.
func (p *Prepared) Finalize(scripts map[string]Script) (ScriptOrder, error) {
	if p == nil {
		return ScriptOrder{}, nil
	}
	resolved, err := finalizeScriptOrder(
		p.preparedScriptOrder, internalScriptOrderScripts(scripts),
	)
	if err != nil {
		return ScriptOrder{}, xerrors.Errorf("resolve script order: %w", err)
	}
	order, err := buildScriptOrderGraphs(resolved.rules)
	if err != nil {
		return ScriptOrder{}, xerrors.Errorf("build script order graphs: %w", err)
	}
	return order, nil
}

func internalScriptOrderScripts(scripts map[string]Script) map[string]scriptOrderScript {
	result := make(map[string]scriptOrderScript, len(scripts))
	for address, script := range scripts {
		result[address] = scriptOrderScript{
			runtimeAddress: script.RuntimeAddress,
			runtimeError:   script.RuntimeError,
			runOnStart:     script.RunOnStart,
			runOnStop:      script.RunOnStop,
			cron:           script.Cron,
		}
	}
	return result
}
