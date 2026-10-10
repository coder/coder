package scriptorder

import (
	"context"
	"maps"
	"slices"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/mitchellh/mapstructure"
	"github.com/zclconf/go-cty/cty"
	"golang.org/x/xerrors"
)

type moduleCallKey struct {
	moduleAddress string
	moduleName    string
}

type stateResourceKey struct {
	moduleAddress string
	resourceName  string
}

type indexedScript struct {
	address     string
	instanceKey cty.Value

	// addressErr records an address validation error found while
	// building the index. The error is reported only if selector
	// expansion encounters this script.
	addressErr error
}

type scriptLifecycleAttributes struct {
	RunOnStart bool   `mapstructure:"run_on_start"`
	RunOnStop  bool   `mapstructure:"run_on_stop"`
	Cron       string `mapstructure:"cron"`
}

type scriptSpan struct {
	start int
	end   int
}

// stateIndex stores each concrete script instance once. Resource
// selectors reference indexes of individual script instances, while
// module selectors reference DFS spans covering module-instance subtrees.
type stateIndex struct {
	// dataSources contains only coder_script_order data source
	// instances, sorted by full Terraform address.
	dataSources       []dataSource
	scripts           []indexedScript
	scriptLifecycles  map[string]scriptLifecycle
	scriptsByResource map[stateResourceKey][]int
	scriptsByModule   map[moduleCallKey][]scriptSpan

	// moduleLookupErrors records child-module address validation
	// errors found while building the index. An error is reported
	// only when resolving a module selector relative to the affected
	// parent.
	moduleLookupErrors map[string]error
}

func newStateIndex(
	ctx context.Context,
	modules []*tfjson.StateModule,
) (*stateIndex, error) {
	idx := &stateIndex{
		scriptLifecycles:   map[string]scriptLifecycle{},
		scriptsByResource:  map[stateResourceKey][]int{},
		scriptsByModule:    map[moduleCallKey][]scriptSpan{},
		moduleLookupErrors: map[string]error{},
	}
	dataSources := map[string]dataSource{}
	for _, module := range modules {
		if _, err := idx.indexModule(ctx, module, dataSources); err != nil {
			return nil, err
		}
	}
	// Keep rule processing and diagnostics deterministic.
	for _, address := range slices.Sorted(maps.Keys(dataSources)) {
		idx.dataSources = append(idx.dataSources, dataSources[address])
	}
	return idx, nil
}

func (idx *stateIndex) indexModule(
	ctx context.Context,
	module *tfjson.StateModule,
	dataSources map[string]dataSource,
) (scriptSpan, error) {
	start := len(idx.scripts)
	if module == nil {
		return scriptSpan{start: start, end: start}, nil
	}
	if err := ctx.Err(); err != nil {
		return scriptSpan{}, err
	}

	for _, resource := range module.Resources {
		if err := ctx.Err(); err != nil {
			return scriptSpan{}, err
		}
		if resource == nil {
			continue
		}
		if resource.Mode == tfjson.DataResourceMode &&
			resource.Type == coderScriptOrderResourceType {
			dataSources[resource.Address] = dataSource{
				moduleAddress: module.Address,
				resource:      resource,
			}
		}
		if resource.Mode != tfjson.ManagedResourceMode ||
			resource.Type != coderScriptResourceType {
			continue
		}
		var attributes scriptLifecycleAttributes
		if err := mapstructure.Decode(resource.AttributeValues, &attributes); err != nil {
			return scriptSpan{}, xerrors.Errorf(
				"decode script %q lifecycle attributes: %w",
				resource.Address, err,
			)
		}
		idx.scriptLifecycles[resource.Address] = scriptLifecycle{
			runOnStart: attributes.RunOnStart,
			runOnStop:  attributes.RunOnStop,
			hasCron:    attributes.Cron != "",
		}

		parsed, err := parseStateResourceAddress(module, resource)
		script := indexedScript{address: resource.Address, addressErr: err}
		if err == nil {
			script.instanceKey = parsed.InstanceKey()
		}
		scriptIndex := len(idx.scripts)
		idx.scripts = append(idx.scripts, script)
		key := stateResourceKey{
			moduleAddress: module.Address,
			resourceName:  resource.Name,
		}
		idx.scriptsByResource[key] = append(idx.scriptsByResource[key], scriptIndex)
	}

	for _, child := range module.ChildModules {
		if err := ctx.Err(); err != nil {
			return scriptSpan{}, err
		}
		if child == nil {
			continue
		}
		childSpan, err := idx.indexModule(ctx, child, dataSources)
		if err != nil {
			return scriptSpan{}, err
		}
		modulePath, err := parseStateModuleAddress(child.Address)
		if err != nil {
			// Preserve the first error to avoid diagnostics changing
			// as later children are processed.
			if idx.moduleLookupErrors[module.Address] == nil {
				idx.moduleLookupErrors[module.Address] = err
			}
			continue
		}
		steps := modulePath.Steps()
		if len(steps) == 0 {
			continue
		}
		key := moduleCallKey{
			moduleAddress: module.Address,
			moduleName:    steps[len(steps)-1].Name(),
		}
		idx.scriptsByModule[key] = append(idx.scriptsByModule[key], childSpan)
	}
	return scriptSpan{start: start, end: len(idx.scripts)}, nil
}
