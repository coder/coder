package terraform

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/mitchellh/mapstructure"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/provisionersdk/proto"
)

// Valid values for the "requires" attribute of a coder_script_order rule.
// These mirror the constants in terraform-provider-coder. They are validated
// again here because templates can pin arbitrary provider versions.
const (
	scriptOrderRequiresSuccess    = "success"
	scriptOrderRequiresCompletion = "completion"
)

// scriptOrderSelectorRe matches the subset of Terraform resource addresses
// accepted in coder_script_order rules:
//
//	coder_script.<name>
//	coder_script.<name>[<index>]
//	module.<name>(.module.<name>...)
//	module.<name>(.module.<name>...).coder_script.<name>
//
// Index keys may be integers or quoted strings (count / for_each). This
// mirrors the validation in terraform-provider-coder.
var scriptOrderSelectorRe = regexp.MustCompile(
	`^(module\.[\w-]+(\[[^\]]+\])?(\.module\.[\w-]+(\[[^\]]+\])?)*(\.coder_script\.[\w-]+(\[[^\]]+\])?)?|coder_script\.[\w-]+(\[[^\]]+\])?)$`,
)

// scriptOrderDataSource is a coder_script_order data source found in
// Terraform state, together with the address of the module that declared
// it. Selectors in its rules resolve relative to that module.
type scriptOrderDataSource struct {
	resource      *tfjson.StateResource
	moduleAddress string
}

// orderableScript is a coder_script resource alongside the proto script it
// produced and the agent or devcontainer the script attached to.
type orderableScript struct {
	address string
	script  *proto.Script
	// owner is the *proto.Agent or *proto.Devcontainer the script attached
	// to, nil when the script is not associated with any agent. Compared by
	// identity to reject ordering rules that span agents.
	owner     any
	ownerName string
}

type scriptOrderRuleAttributes struct {
	Run      string   `mapstructure:"run"`
	After    []string `mapstructure:"after"`
	Requires string   `mapstructure:"requires"`
}

type scriptOrderAttributes struct {
	Rule []scriptOrderRuleAttributes `mapstructure:"rule"`
}

// scriptOrderEdge records why a dependency exists so conflicting duplicate
// declarations can name both data sources in the error.
type scriptOrderEdge struct {
	requires   string
	declaredBy string
}

// applyScriptOrder resolves every coder_script_order rule into
// ScriptOrderDependency entries on the matched proto scripts. It returns
// user-facing build warnings and fails on invalid configurations: unknown
// selectors, rules spanning agents, endpoints that do not run on start,
// conflicting requires values for the same pair, and cycles.
//
// Scripts that are not associated with any agent do not participate:
// during plan, graph traversal only associates root-module resources;
// during stop builds, agents are dropped because their compute resource is
// gone. Rules whose selectors match only unattached scripts are inert and
// take effect on the next start build, when association is concrete.
func applyScriptOrder(dataSources map[string]scriptOrderDataSource, scripts []*orderableScript) ([]string, error) {
	if len(dataSources) == 0 {
		return nil, nil
	}

	byAddress := make(map[string]*orderableScript, len(scripts))
	for _, s := range scripts {
		byAddress[s.address] = s
	}

	// Process data sources in address order so validation errors and
	// emitted dependencies are deterministic across runs.
	sortedDataSources := make([]scriptOrderDataSource, 0, len(dataSources))
	for _, ds := range dataSources {
		sortedDataSources = append(sortedDataSources, ds)
	}
	sort.Slice(sortedDataSources, func(i, j int) bool {
		return sortedDataSources[i].resource.Address < sortedDataSources[j].resource.Address
	})

	// edges[dependent address][dependency address]
	edges := map[string]map[string]scriptOrderEdge{}
	for _, ds := range sortedDataSources {
		var attrs scriptOrderAttributes
		err := mapstructure.Decode(ds.resource.AttributeValues, &attrs)
		if err != nil {
			return nil, xerrors.Errorf("decode script order attributes for %q: %w", ds.resource.Address, err)
		}

		for i, rule := range attrs.Rule {
			ruleRef := fmt.Sprintf("%s: rule[%d]", ds.resource.Address, i)

			requires := rule.Requires
			if requires == "" {
				requires = scriptOrderRequiresSuccess
			}
			if requires != scriptOrderRequiresSuccess && requires != scriptOrderRequiresCompletion {
				return nil, xerrors.Errorf("%s: requires must be %q or %q, got %q",
					ruleRef, scriptOrderRequiresSuccess, scriptOrderRequiresCompletion, requires)
			}

			runScripts, err := resolveScriptOrderSelector(ds, rule.Run, scripts)
			if err != nil {
				return nil, xerrors.Errorf("%s: run: %w", ruleRef, err)
			}
			if len(rule.After) == 0 {
				return nil, xerrors.Errorf("%s: after must list at least one selector", ruleRef)
			}
			for _, afterSelector := range rule.After {
				if afterSelector == rule.Run {
					return nil, xerrors.Errorf("%s: %q cannot run after itself", ruleRef, rule.Run)
				}
				depScripts, err := resolveScriptOrderSelector(ds, afterSelector, scripts)
				if err != nil {
					return nil, xerrors.Errorf("%s: after: %w", ruleRef, err)
				}

				for _, run := range runScripts {
					for _, dep := range depScripts {
						if run.address == dep.address {
							// Overlapping module selectors, e.g. run =
							// "module.x" with after pointing at a script
							// inside module.x. The shared script cannot
							// wait on itself; the rest of the module
							// still waits on it.
							continue
						}
						if run.owner != dep.owner {
							return nil, xerrors.Errorf("%s: %q (agent %q) and %q (agent %q) belong to different agents; ordering only applies within a single agent",
								ruleRef, run.address, run.ownerName, dep.address, dep.ownerName)
						}
						if !run.script.RunOnStart {
							return nil, xerrors.Errorf("%s: %q has run_on_start = false; ordering rules only apply to scripts that run on start",
								ruleRef, run.address)
						}
						if !dep.script.RunOnStart {
							return nil, xerrors.Errorf("%s: %q has run_on_start = false; scripts that run on start cannot wait for it",
								ruleRef, dep.address)
						}

						deps, ok := edges[run.address]
						if !ok {
							deps = map[string]scriptOrderEdge{}
							edges[run.address] = deps
						}
						existing, ok := deps[dep.address]
						if ok && existing.requires != requires {
							return nil, xerrors.Errorf("%q is ordered after %q with requires = %q by %s and requires = %q by %s; declare a single requires value for this pair",
								run.address, dep.address, existing.requires, existing.declaredBy, requires, ds.resource.Address)
						}
						deps[dep.address] = scriptOrderEdge{
							requires:   requires,
							declaredBy: ds.resource.Address,
						}
					}
				}
			}
		}
	}

	if cycle := findScriptOrderCycle(edges); len(cycle) > 0 {
		return nil, xerrors.Errorf("coder_script_order rules form a cycle: %s", strings.Join(cycle, " -> "))
	}

	// Emit dependencies in address order so output is deterministic.
	// Script ids are computed at apply; during plan they can be empty, in
	// which case the rules are validated but no dependency is emitted.
	for _, runAddr := range sortedKeys(edges) {
		run := byAddress[runAddr]
		for _, depAddr := range sortedKeys(edges[runAddr]) {
			dep := byAddress[depAddr]
			if run.script.Id == "" || dep.script.Id == "" {
				continue
			}
			run.script.OrderDependencies = append(run.script.OrderDependencies, &proto.ScriptOrderDependency{
				ScriptId: dep.script.Id,
				Requires: edges[runAddr][depAddr].requires,
			})
		}
	}

	return scriptOrderWarnings(edges, byAddress, scripts), nil
}

// resolveScriptOrderSelector resolves a selector relative to the module
// that declared the data source and returns every matching script that is
// associated with an agent, sorted by address. A selector whose matches
// are all unattached resolves to nothing; only a selector that matches no
// coder_script at all is an error.
func resolveScriptOrderSelector(ds scriptOrderDataSource, selector string, scripts []*orderableScript) ([]*orderableScript, error) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return nil, xerrors.New("selector must not be empty")
	}
	if !scriptOrderSelectorRe.MatchString(selector) {
		return nil, xerrors.Errorf("invalid selector %q: expected coder_script.<name> or module.<name>, optionally nested or indexed", selector)
	}

	prefix := selector
	if ds.moduleAddress != "" {
		prefix = ds.moduleAddress + "." + selector
	}

	matched := []*orderableScript{}
	unattached := 0
	for _, s := range scripts {
		if !addressHasPrefix(s.address, prefix) {
			continue
		}
		if s.owner == nil {
			unattached++
			continue
		}
		matched = append(matched, s)
	}
	if len(matched) > 0 {
		sort.Slice(matched, func(i, j int) bool { return matched[i].address < matched[j].address })
		return matched, nil
	}
	if unattached > 0 {
		// The selector names real scripts that are not attached to any
		// agent right now. This is routine: module-nested scripts during
		// plan, and every script during stop builds, when agents are
		// dropped with their compute resource. An unattached script never
		// runs, so the rule is inert rather than an error.
		return nil, nil
	}

	scope := "the root module"
	if ds.moduleAddress != "" {
		scope = ds.moduleAddress
	}
	candidates := []string{}
	for _, s := range scripts {
		if ds.moduleAddress == "" {
			candidates = append(candidates, s.address)
			continue
		}
		if addressHasPrefix(s.address, ds.moduleAddress) {
			candidates = append(candidates, strings.TrimPrefix(s.address, ds.moduleAddress+"."))
		}
	}
	sort.Strings(candidates)
	if len(candidates) == 0 {
		return nil, xerrors.Errorf("selector %q matches no coder_script in %s, which contains no coder_script resources", selector, scope)
	}
	return nil, xerrors.Errorf("selector %q matches no coder_script in %s; coder_script addresses in scope: %s", selector, scope, strings.Join(candidates, ", "))
}

// addressHasPrefix reports whether address equals prefix or extends it at a
// segment boundary, so "module.git" never matches "module.git_clone".
func addressHasPrefix(address, prefix string) bool {
	if address == prefix {
		return true
	}
	if !strings.HasPrefix(address, prefix) {
		return false
	}
	next := address[len(prefix)]
	return next == '.' || next == '['
}

// findScriptOrderCycle returns the first dependency cycle found as a list
// of addresses ending where it started, or nil when the graph is acyclic.
func findScriptOrderCycle(edges map[string]map[string]scriptOrderEdge) []string {
	const (
		unvisited = iota
		visiting
		done
	)
	state := map[string]int{}
	var cycle []string

	var visit func(node string, path []string) bool
	visit = func(node string, path []string) bool {
		state[node] = visiting
		path = append(path, node)
		for _, dep := range sortedKeys(edges[node]) {
			switch state[dep] {
			case visiting:
				for i, n := range path {
					if n == dep {
						cycle = append(append([]string{}, path[i:]...), dep)
						return true
					}
				}
			case unvisited:
				if visit(dep, path) {
					return true
				}
			}
		}
		state[node] = done
		return false
	}

	for _, node := range sortedKeys(edges) {
		if state[node] == unvisited && visit(node, nil) {
			return cycle
		}
	}
	return nil
}

// scriptOrderWarnings warns once per agent that combines coder_script_order
// rules with scripts that call the experimental bash-level coordination.
// Both feed the same agent DAG, which makes combined behavior hard to
// reason about.
func scriptOrderWarnings(edges map[string]map[string]scriptOrderEdge, byAddress map[string]*orderableScript, scripts []*orderableScript) []string {
	ordering := map[any]bool{}
	for runAddr, deps := range edges {
		ordering[byAddress[runAddr].owner] = true
		for depAddr := range deps {
			ordering[byAddress[depAddr].owner] = true
		}
	}

	warned := map[any]bool{}
	warnings := []string{}
	for _, s := range scripts {
		if s.owner == nil || warned[s.owner] || !ordering[s.owner] {
			continue
		}
		if !strings.Contains(s.script.Script, "coder exp sync") {
			continue
		}
		warned[s.owner] = true
		warnings = append(warnings, fmt.Sprintf("Agent %q uses coder_script_order rules, but script %q calls \"coder exp sync\". Mixing both coordination mechanisms on one agent can deadlock or reorder scripts. Declare the ordering with coder_script_order instead.", s.ownerName, s.script.DisplayName))
	}
	sort.Strings(warnings)
	return warnings
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
