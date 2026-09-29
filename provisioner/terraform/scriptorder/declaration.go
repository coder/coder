// Package scriptorder resolves Terraform coder_script_order data sources into
// deterministic lifecycle dependency graphs for coder_script resources.
//
// The resolution pipeline is:
//
//	coder_script_order data sources
//	    -> rule declarations
//	    -> phase- and runtime-resolved rules
//	    -> deterministic dependency graphs
//
// Selector validation distinguishes declared-but-empty resources from invalid
// selectors. Graph construction bounds work, deduplicates dependencies, and
// rejects conflicting requirements and cycles.
package scriptorder

import (
	"maps"
	"slices"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/mitchellh/mapstructure"
	"github.com/zclconf/go-cty/cty"
	"golang.org/x/xerrors"
)

// Requirement describes the prerequisite outcome required by a dependency.
type Requirement string

const (
	RequirementSuccess    Requirement = "success"
	RequirementCompletion Requirement = "completion"
)

// Phase identifies the lifecycle phase, start or stop, containing a graph.
type Phase string

const (
	PhaseStart Phase = "start"
	PhaseStop  Phase = "stop"
)

type attributes struct {
	Rules []ruleAttributes `mapstructure:"rule"`
}

type ruleAttributes struct {
	Run      []string `mapstructure:"run"`
	After    []string `mapstructure:"after"`
	Requires string   `mapstructure:"requires"`
	Phase    string   `mapstructure:"phase"`
}

type dataSource struct {
	address       string
	moduleAddress string
	resource      *tfjson.StateResource
}

type resolvedSelector struct {
	field string
	raw   string
	kind  selectorKind
	// addresses contains the concrete coder_script instances selected
	// by `raw`. For example, "coder_script.setup" can expand to
	// "coder_script.setup[0]" and "coder_script.setup[1]". It may be
	// empty for a declared unindexed coder_script resource or module
	// call with no script instances.
	addresses []string
}

// ruleDeclaration contains decoded rule values and resolved
// selectors before phase filtering.
type ruleDeclaration struct {
	dataSourceAddress string
	// Rule's zero-based index in the data source, used for diagnostics.
	ruleIndex     int
	declaredPhase Phase
	requirement   Requirement
	run           []resolvedSelector
	after         []resolvedSelector
}

// collectRuleDeclarations decodes coder_script_order data sources
// and resolves their selectors relative to each declaring module.
func collectRuleDeclarations(
	modules []*tfjson.StateModule,
	planConfig *tfjson.Config,
) ([]ruleDeclaration, error) {
	srcs, err := collectDataSources(modules)
	if err != nil {
		return nil, err
	}

	var decls []ruleDeclaration
	for _, src := range srcs {
		var attrs attributes
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
			requirement, err := parseRequirement(rule.Requires)
			if err != nil {
				return nil, ruleError(src.address, i, err)
			}
			phase, err := parsePhase(rule.Phase)
			if err != nil {
				return nil, ruleError(src.address, i, err)
			}

			run, err := resolveSelectors(
				modules, planConfig, src, i, "run", rule.Run,
			)
			if err != nil {
				return nil, err
			}
			after, err := resolveSelectors(
				modules, planConfig, src, i, "after", rule.After,
			)
			if err != nil {
				return nil, err
			}

			decls = append(decls, ruleDeclaration{
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

// collectDataSources returns unique coder_script_order
// data sources sorted by full Terraform address for deterministic
// rule processing and diagnostics.
func collectDataSources(
	modules []*tfjson.StateModule,
) ([]dataSource, error) {
	byAddr := map[string]dataSource{}
	for _, root := range modules {
		if err := walkStateModuleTree(root, func(module *tfjson.StateModule) error {
			for _, rsrc := range module.Resources {
				if rsrc == nil ||
					rsrc.Mode != tfjson.DataResourceMode ||
					rsrc.Type != "coder_script_order" {
					continue
				}
				byAddr[rsrc.Address] = dataSource{
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
	srcs := make([]dataSource, 0, len(addrs))
	for _, addr := range addrs {
		srcs = append(srcs, byAddr[addr])
	}
	return srcs, nil
}

func parseRequirement(raw string) (Requirement, error) {
	switch Requirement(raw) {
	case "", RequirementSuccess:
		return RequirementSuccess, nil
	case RequirementCompletion:
		return RequirementCompletion, nil
	default:
		return "", xerrors.Errorf(
			"requires must be %q or %q, got %q",
			RequirementSuccess, RequirementCompletion, raw,
		)
	}
}

func parsePhase(raw string) (Phase, error) {
	switch Phase(raw) {
	case "":
		return "", nil
	case PhaseStart:
		return PhaseStart, nil
	case PhaseStop:
		return PhaseStop, nil
	default:
		return "", xerrors.Errorf(
			"phase must be %q or %q, got %q",
			PhaseStart, PhaseStop, raw,
		)
	}
}

// resolveSelectors resolves one run or after field. An unindexed
// selector naming a declared script resource or child module call may
// expand to no scripts.
func resolveSelectors(
	modules []*tfjson.StateModule,
	planConfig *tfjson.Config,
	source dataSource,
	ruleIndex int,
	selectorField string,
	rawSelectors []string,
) ([]resolvedSelector, error) {
	if len(rawSelectors) == 0 {
		return nil, ruleError(
			source.address,
			ruleIndex,
			xerrors.Errorf("%s must contain at least one selector", selectorField),
		)
	}

	selectors := make([]resolvedSelector, 0, len(rawSelectors))
	for _, raw := range rawSelectors {
		selector, err := parseSelector(raw)
		if err != nil {
			return nil, ruleError(
				source.address,
				ruleIndex,
				xerrors.Errorf("invalid %s selector %q: %w", selectorField, raw, err),
			)
		}

		resolution, err := resolveSelector(
			modules, planConfig, source.moduleAddress, selector,
		)
		if err != nil {
			return nil, ruleError(
				source.address,
				ruleIndex,
				xerrors.Errorf("resolve %s selector %q: %w", selectorField, raw, err),
			)
		}

		switch selector.kind {
		case selectorScript:
			if len(resolution.addresses) == 0 {
				if selector.instanceKey != cty.NilVal {
					return nil, ruleError(
						source.address,
						ruleIndex,
						xerrors.Errorf(
							"%s selector %q expanded to no coder_script resources",
							selectorField, raw,
						),
					)
				}
				if !resolution.scriptResourceDeclared {
					return nil, ruleError(
						source.address,
						ruleIndex,
						xerrors.Errorf(
							"%s selector %q does not name a declared coder_script resource",
							selectorField, raw,
						),
					)
				}
			}
		case selectorModule:
			if !resolution.moduleCallDeclared {
				return nil, ruleError(
					source.address,
					ruleIndex,
					xerrors.Errorf(
						"%s selector %q does not name a declared child module call",
						selectorField, raw,
					),
				)
			}
		default:
			return nil, ruleError(
				source.address,
				ruleIndex,
				xerrors.Errorf("developer error: %s selector %q has unknown kind %d",
					selectorField, raw, selector.kind),
			)
		}

		selectors = append(selectors, resolvedSelector{
			field:     selectorField,
			raw:       raw,
			kind:      selector.kind,
			addresses: resolution.addresses,
		})
	}
	return selectors, nil
}

func ruleError(dataSourceAddress string, ruleIndex int, err error) error {
	return xerrors.Errorf(
		"script order data source %q rule %d: %w",
		truncateDiagnosticValue(dataSourceAddress), ruleIndex, err,
	)
}
