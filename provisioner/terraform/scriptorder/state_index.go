package scriptorder

import (
	"context"
	"maps"
	"slices"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/zclconf/go-cty/cty"
)

type scriptOrderModuleCallKey struct {
	moduleAddress string
	moduleName    string
}

type scriptOrderStateResourceKey struct {
	moduleAddress string
	resourceName  string
}

type scriptOrderIndexedScript struct {
	address     string
	instanceKey cty.Value
	err         error
}

type scriptOrderScriptSpan struct {
	start int
	end   int
}

// scriptOrderStateIndex retains each concrete script once. Resource selectors
// reference script indexes, while module selectors reference DFS subtree spans.
type scriptOrderStateIndex struct {
	dataSources        []scriptOrderDataSource
	scripts            []scriptOrderIndexedScript
	scriptsByResource  map[scriptOrderStateResourceKey][]int
	scriptsByModule    map[scriptOrderModuleCallKey][]scriptOrderScriptSpan
	moduleAddressError map[string]error
}

func newScriptOrderStateIndex(
	ctx context.Context,
	modules []*tfjson.StateModule,
) (*scriptOrderStateIndex, error) {
	index := &scriptOrderStateIndex{
		scriptsByResource:  map[scriptOrderStateResourceKey][]int{},
		scriptsByModule:    map[scriptOrderModuleCallKey][]scriptOrderScriptSpan{},
		moduleAddressError: map[string]error{},
	}
	dataSources := map[string]scriptOrderDataSource{}
	for _, module := range modules {
		if _, err := index.indexModule(ctx, module, dataSources); err != nil {
			return nil, err
		}
	}
	for _, address := range slices.Sorted(maps.Keys(dataSources)) {
		index.dataSources = append(index.dataSources, dataSources[address])
	}
	return index, nil
}

func (i *scriptOrderStateIndex) indexModule(
	ctx context.Context,
	module *tfjson.StateModule,
	dataSources map[string]scriptOrderDataSource,
) (scriptOrderScriptSpan, error) {
	start := len(i.scripts)
	if module == nil {
		return scriptOrderScriptSpan{start: start, end: start}, nil
	}
	if err := ctx.Err(); err != nil {
		return scriptOrderScriptSpan{}, err
	}

	for _, resource := range module.Resources {
		if err := ctx.Err(); err != nil {
			return scriptOrderScriptSpan{}, err
		}
		if resource == nil {
			continue
		}
		if resource.Mode == tfjson.DataResourceMode && resource.Type == "coder_script_order" {
			dataSources[resource.Address] = scriptOrderDataSource{
				address:       resource.Address,
				moduleAddress: module.Address,
				resource:      resource,
			}
		}
		if resource.Mode != tfjson.ManagedResourceMode || resource.Type != "coder_script" {
			continue
		}

		parsed, err := parseStateResourceAddress(module, resource)
		script := scriptOrderIndexedScript{address: resource.Address, err: err}
		if err == nil {
			script.instanceKey = parsed.InstanceKey()
		}
		scriptIndex := len(i.scripts)
		i.scripts = append(i.scripts, script)
		key := scriptOrderStateResourceKey{
			moduleAddress: module.Address,
			resourceName:  resource.Name,
		}
		i.scriptsByResource[key] = append(i.scriptsByResource[key], scriptIndex)
	}

	for _, child := range module.ChildModules {
		if err := ctx.Err(); err != nil {
			return scriptOrderScriptSpan{}, err
		}
		if child == nil {
			continue
		}
		childSpan, err := i.indexModule(ctx, child, dataSources)
		if err != nil {
			return scriptOrderScriptSpan{}, err
		}
		modulePath, err := parseStateModuleAddress(child.Address)
		if err != nil {
			if i.moduleAddressError[module.Address] == nil {
				i.moduleAddressError[module.Address] = err
			}
			continue
		}
		steps := modulePath.Steps()
		if len(steps) == 0 {
			continue
		}
		key := scriptOrderModuleCallKey{
			moduleAddress: module.Address,
			moduleName:    steps[len(steps)-1].Name(),
		}
		i.scriptsByModule[key] = append(i.scriptsByModule[key], childSpan)
	}
	return scriptOrderScriptSpan{start: start, end: len(i.scripts)}, nil
}
