package scriptorder

import (
	"maps"
	"slices"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/mitchellh/mapstructure"
	"github.com/zclconf/go-cty/cty"
	"golang.org/x/xerrors"
)

// ScriptOrderRequirement describes the prerequisite outcome required
// by a dependency.
type ScriptOrderRequirement string

const (
	ScriptOrderRequirementSuccess    ScriptOrderRequirement = "success"
	ScriptOrderRequirementCompletion ScriptOrderRequirement = "completion"
)

// ScriptOrderPhase identifies the lifecycle phase, start or stop,
// containing a graph.
type ScriptOrderPhase string

const (
	ScriptOrderPhaseStart ScriptOrderPhase = "start"
	ScriptOrderPhaseStop  ScriptOrderPhase = "stop"
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
	declaredPhase ScriptOrderPhase
	requirement   ScriptOrderRequirement
	run           []resolvedScriptOrderSelector
	after         []resolvedScriptOrderSelector
}

// collectScriptOrderRuleDeclarations decodes coder_script_order data
// sources and resolves their selectors relative to each declaring module.
func collectScriptOrderRuleDeclarations(
	modules []*tfjson.StateModule,
	planConfig *tfjson.Config,
) ([]scriptOrderRuleDeclaration, error) {
	srcs, err := collectScriptOrderDataSources(modules)
	if err != nil {
		return nil, err
	}

	var decls []scriptOrderRuleDeclaration
	for _, src := range srcs {
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
				modules, planConfig, src, i, "run", rule.Run,
			)
			if err != nil {
				return nil, err
			}
			after, err := resolveScriptOrderSelectors(
				modules, planConfig, src, i, "after", rule.After,
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

// collectScriptOrderDataSources returns unique coder_script_order
// data sources sorted by full Terraform address for deterministic
// rule processing and diagnostics.
func collectScriptOrderDataSources(
	modules []*tfjson.StateModule,
) ([]scriptOrderDataSource, error) {
	byAddr := map[string]scriptOrderDataSource{}
	for _, root := range modules {
		if err := walkStateModuleTree(root, func(module *tfjson.StateModule) error {
			for _, rsrc := range module.Resources {
				if rsrc == nil ||
					rsrc.Mode != tfjson.DataResourceMode ||
					rsrc.Type != "coder_script_order" {
					continue
				}
				byAddr[rsrc.Address] = scriptOrderDataSource{
					address:       rsrc.Address,
					moduleAddress: module.Address,
					resource:      rsrc,
				}
			}
			return nil
		}); err != nil {
			return nil, err
		}
	}

	addrs := slices.Sorted(maps.Keys(byAddr))
	srcs := make([]scriptOrderDataSource, 0, len(addrs))
	for _, addr := range addrs {
		srcs = append(srcs, byAddr[addr])
	}
	return srcs, nil
}

func parseScriptOrderRequirement(raw string) (ScriptOrderRequirement, error) {
	switch ScriptOrderRequirement(raw) {
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

func parseScriptOrderPhase(raw string) (ScriptOrderPhase, error) {
	switch ScriptOrderPhase(raw) {
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
	modules []*tfjson.StateModule,
	planConfig *tfjson.Config,
	dataSource scriptOrderDataSource,
	ruleIndex int,
	selectorField string,
	rawSelectors []string,
) ([]resolvedScriptOrderSelector, error) {
	if len(rawSelectors) == 0 {
		return nil, scriptOrderRuleError(
			dataSource.address,
			ruleIndex,
			xerrors.Errorf("%s must contain at least one selector", selectorField),
		)
	}

	selectors := make([]resolvedScriptOrderSelector, 0, len(rawSelectors))
	for _, raw := range rawSelectors {
		selector, err := parseScriptOrderSelector(raw)
		if err != nil {
			return nil, scriptOrderRuleError(
				dataSource.address,
				ruleIndex,
				xerrors.Errorf("invalid %s selector %q: %w", selectorField, raw, err),
			)
		}

		resolution, err := resolveScriptOrderSelector(
			modules, planConfig, dataSource.moduleAddress, selector,
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
		truncateScriptOrderDiagnosticValue(dataSourceAddress), ruleIndex, err,
	)
}
