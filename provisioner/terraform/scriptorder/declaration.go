package scriptorder

import (
	tfjson "github.com/hashicorp/terraform-json"
	"github.com/mitchellh/mapstructure"
	"github.com/zclconf/go-cty/cty"
	"golang.org/x/xerrors"

	stringutil "github.com/coder/coder/v2/coderd/util/strings"
)

// Requirement describes the prerequisite outcome required by a dependency.
type Requirement string

const (
	ScriptOrderRequirementSuccess    Requirement = "success"
	ScriptOrderRequirementCompletion Requirement = "completion"
)

// Phase identifies the lifecycle phase, start or stop, containing a graph.
type Phase string

const (
	ScriptOrderPhaseStart Phase = "start"
	ScriptOrderPhaseStop  Phase = "stop"
)

type scriptOrderAttributes struct {
	Rules []scriptOrderRuleAttributes `mapstructure:"rule"`
}

type scriptOrderRuleAttributes struct {
	Run      []string `mapstructure:"run"`
	After    []string `mapstructure:"after"`
	Requires string   `mapstructure:"requires"`
	Phase    string   `mapstructure:"phase"`
}

type scriptOrderDataSource struct {
	address       string
	moduleAddress string
	resource      *tfjson.StateResource
}

type resolvedScriptOrderSelector struct {
	field string
	raw   string
	kind  scriptOrderSelectorKind
	// addresses contains the concrete coder_script instances selected
	// by `raw`. For example, "coder_script.setup" can expand to
	// "coder_script.setup[0]" and "coder_script.setup[1]". It may be
	// empty for a declared unindexed coder_script resource or module
	// call with no script instances.
	addresses []string
}

// scriptOrderRuleDeclaration contains decoded rule values and resolved
// selectors before phase filtering.
type scriptOrderRuleDeclaration struct {
	dataSourceAddress string
	// Rule's zero-based index in the data source, used for diagnostics.
	ruleIndex     int
	declaredPhase Phase
	requirement   Requirement
	run           []resolvedScriptOrderSelector
	after         []resolvedScriptOrderSelector
}

type scriptOrderRuleIdentity struct {
	dataSourceAddress string
	ruleIndex         int
}

func (d scriptOrderRuleDeclaration) identity() scriptOrderRuleIdentity {
	return scriptOrderRuleIdentity{
		dataSourceAddress: d.dataSourceAddress,
		ruleIndex:         d.ruleIndex,
	}
}

// collectScriptOrderRuleDeclarations decodes coder_script_order data
// sources and resolves their selectors relative to each declaring module.
func collectScriptOrderRuleDeclarations(
	program *Program,
) ([]scriptOrderRuleDeclaration, error) {
	return collectScriptOrderRuleDeclarationsWithExpansionLimit(
		program,
		maxScriptOrderExpandedAddresses,
	)
}

func collectScriptOrderRuleDeclarationsWithExpansionLimit(
	program *Program,
	expansionLimit int,
) ([]scriptOrderRuleDeclaration, error) {
	if program == nil {
		return nil, nil
	}

	var decls []scriptOrderRuleDeclaration
	expansionBudget := scriptOrderExpansionBudget{limit: expansionLimit}
	for _, src := range program.dataSources {
		var attrs scriptOrderAttributes
		err := mapstructure.Decode(src.resource.AttributeValues, &attrs)
		if err != nil {
			return nil, xerrors.Errorf(
				"decode script order data source %q: %w", src.address, err,
			)
		}
		if len(attrs.Rules) == 0 {
			return nil, xerrors.Errorf(
				"script order data source %q must contain at least one rule",
				src.address,
			)
		}

		for i, rule := range attrs.Rules {
			requirement, err := parseScriptOrderRequirement(rule.Requires)
			if err != nil {
				return nil, scriptOrderRuleError(src.address, i, err)
			}
			phase, err := parseScriptOrderPhase(rule.Phase)
			if err != nil {
				return nil, scriptOrderRuleError(src.address, i, err)
			}

			run, err := resolveScriptOrderSelectors(
				program.stateIndex, program.configIndex, src, i, "run", rule.Run,
				&expansionBudget,
			)
			if err != nil {
				return nil, err
			}
			after, err := resolveScriptOrderSelectors(
				program.stateIndex, program.configIndex, src, i, "after", rule.After,
				&expansionBudget,
			)
			if err != nil {
				return nil, err
			}

			decls = append(decls, scriptOrderRuleDeclaration{
				dataSourceAddress: src.address,
				ruleIndex:         i,
				declaredPhase:     phase,
				requirement:       requirement,
				run:               run,
				after:             after,
			})
		}
	}
	return decls, nil
}

func parseScriptOrderRequirement(raw string) (Requirement, error) {
	switch Requirement(raw) {
	case "", ScriptOrderRequirementSuccess:
		return ScriptOrderRequirementSuccess, nil
	case ScriptOrderRequirementCompletion:
		return ScriptOrderRequirementCompletion, nil
	default:
		return "", xerrors.Errorf(
			"requires must be %q or %q, got %q",
			ScriptOrderRequirementSuccess, ScriptOrderRequirementCompletion, raw,
		)
	}
}

func parseScriptOrderPhase(raw string) (Phase, error) {
	switch Phase(raw) {
	case "":
		return "", nil
	case ScriptOrderPhaseStart:
		return ScriptOrderPhaseStart, nil
	case ScriptOrderPhaseStop:
		return ScriptOrderPhaseStop, nil
	default:
		return "", xerrors.Errorf(
			"phase must be %q or %q, got %q",
			ScriptOrderPhaseStart, ScriptOrderPhaseStop, raw,
		)
	}
}

// resolveScriptOrderSelectors resolves one run or after field. An
// unindexed selector naming a declared script resource or child module
// call may expand to no scripts.
func resolveScriptOrderSelectors(
	stateIndex *scriptOrderStateIndex,
	configIndex *scriptOrderConfigIndex,
	dataSource scriptOrderDataSource,
	ruleIndex int,
	selectorField string,
	rawSelectors []string,
	expansionBudget *scriptOrderExpansionBudget,
) ([]resolvedScriptOrderSelector, error) {
	if len(rawSelectors) == 0 {
		return nil, scriptOrderRuleError(
			dataSource.address,
			ruleIndex,
			xerrors.Errorf("%s must contain at least one selector", selectorField),
		)
	}

	selectors := make([]resolvedScriptOrderSelector, 0, len(rawSelectors))
	seen := make(map[string]struct{}, len(rawSelectors))
	for _, raw := range rawSelectors {
		if _, duplicate := seen[raw]; duplicate {
			continue
		}
		seen[raw] = struct{}{}
		selector, err := parseScriptOrderSelector(raw)
		if err != nil {
			return nil, scriptOrderRuleError(
				dataSource.address,
				ruleIndex,
				xerrors.Errorf("invalid %s selector %q: %w", selectorField, raw, err),
			)
		}

		resolution, err := resolveScriptOrderSelector(
			stateIndex, configIndex, dataSource.moduleAddress, selector,
			expansionBudget,
		)
		if err != nil {
			return nil, scriptOrderRuleError(
				dataSource.address,
				ruleIndex,
				xerrors.Errorf("resolve %s selector %q: %w", selectorField, raw, err),
			)
		}

		switch selector.kind {
		case scriptOrderSelectorScript:
			if len(resolution.addresses) == 0 {
				if selector.instanceKey != cty.NilVal {
					return nil, scriptOrderRuleError(
						dataSource.address,
						ruleIndex,
						xerrors.Errorf(
							"%s selector %q expanded to no coder_script resources",
							selectorField, raw,
						),
					)
				}
				if !resolution.scriptResourceDeclared {
					return nil, scriptOrderRuleError(
						dataSource.address,
						ruleIndex,
						xerrors.Errorf(
							"%s selector %q does not name a declared coder_script resource",
							selectorField, raw,
						),
					)
				}
			}
		case scriptOrderSelectorModule:
			if !resolution.moduleCallDeclared {
				return nil, scriptOrderRuleError(
					dataSource.address,
					ruleIndex,
					xerrors.Errorf(
						"%s selector %q does not name a declared child module call",
						selectorField, raw,
					),
				)
			}
		default:
			return nil, scriptOrderRuleError(
				dataSource.address,
				ruleIndex,
				xerrors.Errorf("developer error: %s selector %q has unknown kind %d",
					selectorField, raw, selector.kind),
			)
		}

		selectors = append(selectors, resolvedScriptOrderSelector{
			field:     selectorField,
			raw:       raw,
			kind:      selector.kind,
			addresses: resolution.addresses,
		})
	}
	return selectors, nil
}

func scriptOrderRuleError(dataSourceAddress string, ruleIndex int, err error) error {
	return xerrors.Errorf(
		"script order data source %q rule %d: %w",
		truncateScriptOrderDiagnosticValue(dataSourceAddress), ruleIndex,
		boundedScriptOrderDiagnosticError{err: err},
	)
}

type boundedScriptOrderDiagnosticError struct {
	err error
}

func (e boundedScriptOrderDiagnosticError) Error() string {
	return stringutil.Truncate(
		e.err.Error(),
		maxScriptOrderRuleDiagnosticRunes,
		stringutil.TruncateWithEllipsis,
	)
}

func (e boundedScriptOrderDiagnosticError) Unwrap() error {
	return e.err
}

const (
	// maxScriptOrderExpandedAddresses bounds address entries retained across
	// all selectors before no-op rules are discarded and dependency
	// combinations are counted.
	maxScriptOrderExpandedAddresses   = 100_000
	maxScriptOrderRuleDiagnosticRunes = 64 << 10
)
