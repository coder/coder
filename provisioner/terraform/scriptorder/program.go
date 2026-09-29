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
		return nil, nil
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
		return nil, nil
	}
	return &Program{
		stateIndex:  stateIndex,
		configIndex: configIndex,
		dataSources: dataSources,
	}, nil
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

// Finalize validates bound runtimes and constructs deterministic graphs.
func (p *Prepared) Finalize(scripts map[string]Script) (ScriptOrder, error) {
	if p == nil {
		return ScriptOrder{}, nil
	}
	resolved, err := finalizeScriptOrder(
		p.preparedScriptOrder, internalScriptOrderScripts(scripts),
	)
	if err != nil {
		return ScriptOrder{}, err
	}
	return buildScriptOrderGraphs(resolved.rules)
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
