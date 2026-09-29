package scriptorder

import (
	tfjson "github.com/hashicorp/terraform-json"
	"github.com/mitchellh/mapstructure"
	"github.com/zclconf/go-cty/cty"
	"golang.org/x/xerrors"

	stringutil "github.com/coder/coder/v2/coderd/util/strings"
)

const (
	// maxExpandedAddresses bounds address entries retained across
	// all selectors before no-op rules are discarded and dependency
	// combinations are counted.
	maxExpandedAddresses   = 100_000
	maxRuleDiagnosticRunes = 64 << 10
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

type ruleIdentity struct {
	dataSourceAddress string
	ruleIndex         int
}

func (d *ruleDeclaration) identity() ruleIdentity {
	return ruleIdentity{
		dataSourceAddress: d.dataSourceAddress,
		ruleIndex:         d.ruleIndex,
	}
}

// collectRuleDeclarations decodes coder_script_order data sources
// and resolves their selectors relative to each declaring module.
func (p *Program) collectRuleDeclarations() ([]ruleDeclaration, error) {
	return p.collectRuleDeclarationsWithExpansionLimit(maxExpandedAddresses)
}

func (p *Program) collectRuleDeclarationsWithExpansionLimit(
	expansionLimit int,
) ([]ruleDeclaration, error) {
	if p == nil {
		return nil, nil
	}

	var decls []ruleDeclaration
	budget := expansionBudget{limit: expansionLimit}
	for _, src := range p.dataSources {
		var attrs attributes
		err := mapstructure.Decode(src.resource.AttributeValues, &attrs)
		if err != nil {
			return nil, xerrors.Errorf(
				"decode script order data source %q: %w", src.resource.Address, err,
			)
		}
		if len(attrs.Rules) == 0 {
			return nil, xerrors.Errorf(
				"script order data source %q must contain at least one rule",
				src.resource.Address,
			)
		}

		for i, rule := range attrs.Rules {
			requirement, err := parseRequirement(rule.Requires)
			if err != nil {
				return nil, ruleError(src.resource.Address, i, err)
			}
			phase, err := parsePhase(rule.Phase)
			if err != nil {
				return nil, ruleError(src.resource.Address, i, err)
			}

			run, err := p.resolveSelectors(src, i, "run", rule.Run, &budget)
			if err != nil {
				return nil, err
			}
			after, err := p.resolveSelectors(src, i, "after", rule.After, &budget)
			if err != nil {
				return nil, err
			}

			decls = append(decls, ruleDeclaration{
				dataSourceAddress: src.resource.Address,
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
func (p *Program) resolveSelectors(
	source dataSource,
	ruleIndex int,
	selectorField string,
	rawSelectors []string,
	budget *expansionBudget,
) ([]resolvedSelector, error) {
	if len(rawSelectors) == 0 {
		return nil, ruleError(
			source.resource.Address,
			ruleIndex,
			xerrors.Errorf("%s must contain at least one selector", selectorField),
		)
	}

	selectors := make([]resolvedSelector, 0, len(rawSelectors))
	seen := make(map[string]struct{}, len(rawSelectors))
	for _, raw := range rawSelectors {
		if _, duplicate := seen[raw]; duplicate {
			continue
		}
		seen[raw] = struct{}{}
		selector, err := parseSelector(raw)
		if err != nil {
			return nil, ruleError(
				source.resource.Address,
				ruleIndex,
				xerrors.Errorf("invalid %s selector %q: %w", selectorField, raw, err),
			)
		}

		resolution, err := p.resolveSelector(
			source.moduleAddress, selector, budget,
		)
		if err != nil {
			return nil, ruleError(
				source.resource.Address,
				ruleIndex,
				xerrors.Errorf("resolve %s selector %q: %w", selectorField, raw, err),
			)
		}

		switch selector.kind {
		case selectorScript:
			if len(resolution.addresses) == 0 {
				if selector.instanceKey != cty.NilVal {
					return nil, ruleError(
						source.resource.Address,
						ruleIndex,
						xerrors.Errorf(
							"%s selector %q expanded to no coder_script resources",
							selectorField, raw,
						),
					)
				}
				if !resolution.scriptResourceDeclared {
					return nil, ruleError(
						source.resource.Address,
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
					source.resource.Address,
					ruleIndex,
					xerrors.Errorf(
						"%s selector %q does not name a declared child module call",
						selectorField, raw,
					),
				)
			}
		default:
			return nil, ruleError(
				source.resource.Address,
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
		truncateDiagnosticValue(dataSourceAddress), ruleIndex,
		boundedDiagnosticError{err: err},
	)
}

type boundedDiagnosticError struct {
	err error
}

func (e boundedDiagnosticError) Error() string {
	return stringutil.Truncate(
		e.err.Error(),
		maxRuleDiagnosticRunes,
		stringutil.TruncateWithEllipsis,
	)
}

func (e boundedDiagnosticError) Unwrap() error {
	return e.err
}
