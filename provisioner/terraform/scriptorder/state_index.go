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
	address string
	// addressErr records an address validation error found while
	// building the index. The error is reported only if selector
	// expansion encounters this script.
	addressErr error
}

// scriptInstanceKey distinguishes `count` indices from `for_each`
// string keys, including count index 0 and the empty string key.
type scriptInstanceKey struct {
	stringKey string
	countKey  int64
	isString  bool
}

// instanceLookupKey converts cty instance keys to Go map keys.
// The key must be validated by tfaddr and must not be cty.NilVal.
func instanceLookupKey(key cty.Value) scriptInstanceKey {
	if key.Type() == cty.String {
		return scriptInstanceKey{stringKey: key.AsString(), isString: true}
	}
	// tfaddr bounds numeric instance keys to nonnegative Go ints.
	count, _ := key.AsBigFloat().Int64()
	return scriptInstanceKey{countKey: count}
}

// scriptResourceIndex stores instances of one script resource in a module
// instance. Both scripts and instances store indices into stateIndex.scripts.
type scriptResourceIndex struct {
	// Lists all instances for selectors like coder_script.setup.
	scripts []int
	// Maps instance keys to indices for selectors like
	// coder_script.setup[0]. Use a slice defensively in case
	// Terraform plan/state data contains multiple records for one
	// key.
	instances map[scriptInstanceKey][]int
	// Holds the first address error among this resource's instances.
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
	dataSources           []dataSource
	scripts               []indexedScript
	scriptLifecycles      map[string]scriptLifecycle
	scriptResourceIndexes map[stateResourceKey]*scriptResourceIndex
	scriptsByModule       map[moduleCallKey][]scriptSpan

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
		scriptLifecycles:      map[string]scriptLifecycle{},
		scriptResourceIndexes: map[stateResourceKey]*scriptResourceIndex{},
		scriptsByModule:       map[moduleCallKey][]scriptSpan{},
		moduleLookupErrors:    map[string]error{},
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
		scriptIndex := len(idx.scripts)
		idx.scripts = append(idx.scripts, indexedScript{
			address: resource.Address, addressErr: err,
		})
		key := stateResourceKey{
			moduleAddress: module.Address,
			resourceName:  resource.Name,
		}
		// All instances of this resource share one index within the module.
		resourceIndex := idx.scriptResourceIndexes[key]
		if resourceIndex == nil {
			resourceIndex = &scriptResourceIndex{}
			idx.scriptResourceIndexes[key] = resourceIndex
		}
		resourceIndex.scripts = append(resourceIndex.scripts, scriptIndex)
		if err != nil {
			if resourceIndex.addressErr == nil {
				resourceIndex.addressErr = err
			}
		} else if instanceKey := parsed.InstanceKey(); instanceKey != cty.NilVal {
			if resourceIndex.instances == nil {
				resourceIndex.instances = map[scriptInstanceKey][]int{}
			}
			lookupKey := instanceLookupKey(instanceKey)
			resourceIndex.instances[lookupKey] = append(resourceIndex.instances[lookupKey], scriptIndex)
		}
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
