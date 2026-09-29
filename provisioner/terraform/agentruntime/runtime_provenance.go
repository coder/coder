package agentruntime

import (
	"cmp"
	"context"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/provisioner/terraform/tfaddr"
)

type runtimeTargetKinds uint8

const (
	runtimeTargetKindWorkspaceAgent runtimeTargetKinds = 1 << iota
	runtimeTargetKindDevcontainer
)

type runtimeConfiguredReference struct {
	reference      string
	correlationKey cty.Value
	resultSuffix   string
}

type runtimeResolvedReference struct {
	modulePath tfaddr.ModulePath
	reference  string
}

type runtimeProvenanceQuery struct {
	modulePath     string
	reference      string
	correlationKey string
	resultSuffix   string
}

type runtimeResolvedReferenceKey struct {
	modulePath string
	reference  string
}

type runtimeProvenanceResolution struct {
	resolver  *Resolver
	budget    *provenanceBudget
	completed map[runtimeProvenanceQuery]struct{}
	visiting  map[runtimeProvenanceQuery]struct{}
	results   map[runtimeResolvedReferenceKey]runtimeResolvedReference
}

type runtimeReferenceSource struct {
	moduleAddress string
	reference     string
}

func (r *Resolver) resolveConfiguredRuntimeReferences(
	ctx context.Context,
	modulePath tfaddr.ModulePath,
	references []runtimeConfiguredReference,
	budget *provenanceBudget,
) ([]runtimeResolvedReference, error) {
	resolution := &runtimeProvenanceResolution{
		resolver:  r,
		budget:    budget,
		completed: map[runtimeProvenanceQuery]struct{}{},
		visiting:  map[runtimeProvenanceQuery]struct{}{},
		results:   map[runtimeResolvedReferenceKey]runtimeResolvedReference{},
	}
	for _, configuredReference := range references {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := resolution.resolve(
			ctx,
			modulePath,
			configuredReference.reference,
			configuredReference.correlationKey,
			configuredReference.resultSuffix,
			0,
		); err != nil {
			return nil, err
		}
	}

	keys := slices.Collect(maps.Keys(resolution.results))
	slices.SortFunc(keys, func(a, b runtimeResolvedReferenceKey) int {
		return cmp.Or(
			strings.Compare(a.modulePath, b.modulePath),
			strings.Compare(a.reference, b.reference),
		)
	})
	result := make([]runtimeResolvedReference, 0, len(keys))
	for _, key := range keys {
		result = append(result, resolution.results[key])
	}
	return result, nil
}

func (r *runtimeProvenanceResolution) resolve(
	ctx context.Context,
	modulePath tfaddr.ModulePath,
	reference string,
	correlationKey cty.Value,
	resultSuffix string,
	depth int,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := r.budget.checkReferenceDepth(depth); err != nil {
		return err
	}
	if correlationKey != cty.NilVal {
		collection, err := runtimeCorrelatedCollection(reference)
		if err != nil {
			return err
		}
		if collection != "" {
			correlatedReference := runtimeReferenceWithInstanceKey(
				reference, collection, correlationKey,
			)
			correlatedCollection := runtimeReferenceWithInstanceKey(
				collection, collection, correlationKey,
			)
			if strings.HasPrefix(collection, "module.") ||
				len(r.resolver.runtimes.byInstanceAddress[correlatedCollection]) > 0 {
				reference = correlatedReference
			}
			correlationKey = cty.NilVal
		}
	}
	correlationAddressKey, ok := runtimeInstanceKeyString(correlationKey)
	if !ok {
		return xerrors.New(
			"agent runtime provenance correlation key must be a string or integer",
		)
	}
	key := runtimeProvenanceQuery{
		modulePath:     modulePath.String(),
		reference:      reference,
		correlationKey: correlationAddressKey,
		resultSuffix:   resultSuffix,
	}
	if err := r.budget.consumeReference(
		len(key.modulePath) + len(key.reference) + len(key.correlationKey) +
			len(key.resultSuffix),
	); err != nil {
		return err
	}
	if _, cycle := r.visiting[key]; cycle {
		r.addResult(modulePath, reference, resultSuffix)
		return nil
	}
	if _, completed := r.completed[key]; completed {
		return nil
	}

	traversal, err := parseRuntimeTraversal(reference)
	if err != nil {
		return err
	}
	if len(traversal) < 2 {
		r.completed[key] = struct{}{}
		r.addResult(modulePath, reference, resultSuffix)
		return nil
	}
	root, ok := traversal[0].(hcl.TraverseRoot)
	if !ok {
		return xerrors.Errorf(
			"reference %q has no Terraform traversal root", reference,
		)
	}
	if err := rejectRuntimeAggregateIndirection(
		reference, traversal, root,
	); err != nil {
		return err
	}

	r.visiting[key] = struct{}{}
	defer delete(r.visiting, key)

	switch root.Name {
	case "local":
		name, ok := traversal[1].(hcl.TraverseAttr)
		if !ok {
			break
		}
		localKey := runtimeLocalKey{
			moduleAddress: modulePath.ConfigurationAddress(),
			name:          name.Name,
		}
		expressionKey := runtimeExpressionKey{
			moduleAddress: localKey.moduleAddress,
			kind:          runtimeExpressionLocal,
			attribute:     localKey.name,
		}
		if !r.resolver.configIndex.runtimeExpressionPreservesIdentity(
			expressionKey,
		) {
			return xerrors.Errorf(
				"local value %q does not preserve agent runtime identity",
				name.Name,
			)
		}
		localReferences := r.resolver.configIndex.localRuntime[localKey]
		if len(localReferences) == 0 {
			break
		}
		for index, localReference := range localReferences {
			if index%256 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			resolvedReference := localReference.reference
			if localReference.correlatedCollection != "" &&
				correlationKey != cty.NilVal {
				resolvedReference = runtimeReferenceWithInstanceKey(
					resolvedReference,
					localReference.correlatedCollection,
					correlationKey,
				)
			}
			if err := r.resolve(
				ctx,
				modulePath,
				resolvedReference,
				cty.NilVal,
				cmp.Or(
					resultSuffix,
					r.resolver.configIndex.runtimeExpressionResultSuffix(
						expressionKey,
					),
				),
				depth+1,
			); err != nil {
				return err
			}
		}
		r.completed[key] = struct{}{}
		return nil
	case "var":
		name, ok := traversal[1].(hcl.TraverseAttr)
		steps := modulePath.Steps()
		if !ok || len(steps) == 0 {
			break
		}
		lastStep := steps[len(steps)-1]
		parentPath, err := runtimeModulePathFromSteps(
			steps[:len(steps)-1],
		)
		if err != nil {
			return err
		}
		parentAddress := parentPath.ConfigurationAddress()
		call, ok := r.resolver.configIndex.moduleCalls[moduleCallKey{
			moduleAddress: parentAddress,
			moduleName:    lastStep.Name(),
		}]
		inputReferences, declared := call.inputReferences[name.Name]
		if !ok || !declared {
			break
		}
		expressionKey := runtimeExpressionKey{
			moduleAddress: parentAddress,
			kind:          runtimeExpressionModuleCall,
			name:          lastStep.Name(),
			attribute:     name.Name,
		}
		if !r.resolver.configIndex.runtimeExpressionPreservesIdentity(
			expressionKey,
		) {
			return xerrors.Errorf(
				"module %q input %q does not preserve agent runtime identity",
				lastStep.Name(), name.Name,
			)
		}
		references, err := mostSpecificTerraformReferences(
			ctx,
			r.resolver.configIndex.runtimeExpressionValueReferences(
				expressionKey, inputReferences,
			),
		)
		if err != nil {
			return err
		}
		parentCorrelationKey := cty.NilVal
		if r.resolver.configIndex.runtimeExpressionUsesEachValueAsValue(
			expressionKey, references,
		) && len(call.forEachReferences) > 0 {
			forEachKey := runtimeExpressionKey{
				moduleAddress: parentAddress,
				kind:          runtimeExpressionModuleCall,
				name:          lastStep.Name(),
				attribute:     "for_each",
			}
			if !r.resolver.configIndex.runtimeExpressionPreservesIdentity(
				forEachKey,
			) {
				return xerrors.Errorf(
					"module %q for_each expression does not preserve agent runtime identity",
					lastStep.Name(),
				)
			}
			collections, err := mostSpecificTerraformReferences(
				ctx,
				r.resolver.configIndex.runtimeExpressionValueReferences(
					forEachKey, call.forEachReferences,
				),
			)
			if err != nil {
				return err
			}
			parentCorrelationKey = lastStep.InstanceKey()
			err = visitRuntimeSubstitutedIteratorReferences(
				references,
				collections,
				func(parentReference string) error {
					return r.resolve(
						ctx,
						parentPath,
						parentReference,
						parentCorrelationKey,
						cmp.Or(
							resultSuffix,
							r.resolver.configIndex.runtimeExpressionResultSuffix(
								expressionKey,
							),
						),
						depth+1,
					)
				},
			)
			if err != nil {
				return err
			}
		} else {
			for index, parentReference := range references {
				if index%256 == 0 {
					if err := ctx.Err(); err != nil {
						return err
					}
				}
				if err := r.resolve(
					ctx,
					parentPath,
					parentReference,
					parentCorrelationKey,
					cmp.Or(
						resultSuffix,
						r.resolver.configIndex.runtimeExpressionResultSuffix(
							expressionKey,
						),
					),
					depth+1,
				); err != nil {
					return err
				}
			}
		}
		r.completed[key] = struct{}{}
		return nil
	}

	r.completed[key] = struct{}{}
	r.addResult(modulePath, reference, resultSuffix)
	return nil
}

func (r *runtimeProvenanceResolution) addResult(
	modulePath tfaddr.ModulePath,
	reference string,
	resultSuffix string,
) {
	reference = runtimeReferenceWithResultSuffix(
		reference, resultSuffix,
	)
	key := runtimeResolvedReferenceKey{
		modulePath: modulePath.String(),
		reference:  reference,
	}
	r.results[key] = runtimeResolvedReference{
		modulePath: modulePath,
		reference:  reference,
	}
}

func (r *Resolver) runtimeReferenceTarget(
	ctx context.Context,
	modulePath tfaddr.ModulePath,
	reference string,
) (runtimeReferenceTarget, error) {
	direct := runtimeReferenceTargetForReference(reference)
	if direct != runtimeReferenceTargetAny {
		return direct, nil
	}
	kinds, err := r.runtimeReferenceTargetKinds(
		ctx,
		modulePath.ConfigurationAddress(),
		reference,
		map[runtimeReferenceQuery]struct{}{},
	)
	if err != nil {
		return runtimeReferenceTargetAny, err
	}
	switch kinds {
	case runtimeTargetKindWorkspaceAgent:
		return runtimeReferenceTargetWorkspaceAgent, nil
	case runtimeTargetKindDevcontainer:
		return runtimeReferenceTargetDevcontainer, nil
	default:
		return runtimeReferenceTargetAny, nil
	}
}

func (r *Resolver) runtimeReferenceTargetKinds(
	ctx context.Context,
	moduleAddress string,
	reference string,
	visiting map[runtimeReferenceQuery]struct{},
) (runtimeTargetKinds, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	key := runtimeReferenceQuery{
		moduleAddress: moduleAddress,
		reference:     reference,
	}
	if kinds, ok := r.referenceTargets[key]; ok {
		return kinds, nil
	}
	if _, cycle := visiting[key]; cycle {
		return 0, nil
	}
	if len(visiting) >= maxRuntimeProvenanceDepth {
		return 0, xerrors.Errorf(
			"agent runtime provenance exceeds the limit of %d indirect Terraform references",
			maxRuntimeProvenanceDepth,
		)
	}
	if len(r.referenceTargets) >= r.limits.referenceCacheEntries {
		return 0, xerrors.Errorf(
			"agent runtime resolution exceeds the limit of %d cached Terraform reference targets",
			r.limits.referenceCacheEntries,
		)
	}
	keyBytes := len(key.moduleAddress) + len(key.reference)
	if exceedsLimit(
		r.referenceTargetKeyBytes,
		keyBytes,
		r.limits.referenceCacheKeyBytes,
	) {
		return 0, xerrors.Errorf(
			"agent runtime resolution exceeds the limit of %d cached Terraform reference target bytes",
			r.limits.referenceCacheKeyBytes,
		)
	}

	direct := runtimeReferenceTargetForReference(reference)
	if direct != runtimeReferenceTargetAny {
		kinds := runtimeKindsForReferenceTarget(direct)
		r.referenceTargets[key] = kinds
		r.referenceTargetKeyBytes += keyBytes
		return kinds, nil
	}

	visiting[key] = struct{}{}
	defer delete(visiting, key)
	sources, err := r.indirectRuntimeReferenceSources(
		ctx, moduleAddress, reference,
	)
	if err != nil {
		return 0, err
	}
	var kinds runtimeTargetKinds
	for _, source := range sources {
		resolved, err := r.runtimeReferenceTargetKinds(
			ctx, source.moduleAddress, source.reference, visiting,
		)
		if err != nil {
			return 0, err
		}
		kinds |= resolved
	}
	r.referenceTargets[key] = kinds
	r.referenceTargetKeyBytes += keyBytes
	return kinds, nil
}

func runtimeKindsForReferenceTarget(
	target runtimeReferenceTarget,
) runtimeTargetKinds {
	switch target {
	case runtimeReferenceTargetWorkspaceAgent,
		runtimeReferenceTargetDevcontainerParent:
		return runtimeTargetKindWorkspaceAgent
	case runtimeReferenceTargetDevcontainer:
		return runtimeTargetKindDevcontainer
	default:
		return 0
	}
}

func (r *Resolver) indirectRuntimeReferenceSources(
	ctx context.Context,
	moduleAddress string,
	reference string,
) ([]runtimeReferenceSource, error) {
	traversal, err := parseRuntimeTraversal(reference)
	if err != nil {
		return nil, err
	}
	if len(traversal) < 2 {
		return nil, nil
	}
	root, ok := traversal[0].(hcl.TraverseRoot)
	if !ok {
		return nil, nil
	}
	if err := rejectRuntimeAggregateIndirection(
		reference, traversal, root,
	); err != nil {
		return nil, err
	}

	switch root.Name {
	case "local":
		name, ok := traversal[1].(hcl.TraverseAttr)
		if !ok {
			return nil, nil
		}
		localKey := runtimeLocalKey{
			moduleAddress: moduleAddress,
			name:          name.Name,
		}
		if !r.configIndex.runtimeExpressionPreservesIdentity(
			runtimeExpressionKey{
				moduleAddress: moduleAddress,
				kind:          runtimeExpressionLocal,
				attribute:     name.Name,
			},
		) {
			return nil, xerrors.Errorf(
				"local value %q does not preserve agent runtime identity",
				name.Name,
			)
		}
		localReferences := r.configIndex.localRuntime[localKey]
		references := make([]string, 0, len(localReferences))
		for _, localReference := range localReferences {
			references = append(references, localReference.reference)
		}
		return runtimeReferenceSources(
			moduleAddress, references,
		), nil
	case "module":
		referencedModuleAddress, position, err :=
			runtimeReferenceModuleAddress(traversal)
		if err != nil {
			return nil, err
		}
		if referencedModuleAddress == "" || position >= len(traversal) {
			return nil, nil
		}
		outputName, ok := traversal[position].(hcl.TraverseAttr)
		if !ok {
			return nil, nil
		}
		childModuleAddress := runtimeQualifyConfigurationAddress(
			moduleAddress, referencedModuleAddress,
		)
		module, ok := r.configIndex.modules[childModuleAddress]
		outputReferences, declared := module.outputReferences[outputName.Name]
		if !ok || !declared {
			return nil, nil
		}
		expressionKey := runtimeExpressionKey{
			moduleAddress: childModuleAddress,
			kind:          runtimeExpressionOutput,
			name:          outputName.Name,
			attribute:     "value",
		}
		if !r.configIndex.runtimeExpressionPreservesIdentity(expressionKey) {
			return nil, xerrors.Errorf(
				"module output %q does not preserve agent runtime identity",
				outputName.Name,
			)
		}
		references, err := mostSpecificTerraformReferences(
			ctx,
			r.configIndex.runtimeExpressionValueReferences(
				expressionKey, outputReferences,
			),
		)
		if err != nil {
			return nil, err
		}
		return runtimeReferenceSources(
			childModuleAddress, references,
		), nil
	case "var":
		name, ok := traversal[1].(hcl.TraverseAttr)
		if !ok || moduleAddress == "" {
			return nil, nil
		}
		modulePath, err := tfaddr.ParseModulePath(moduleAddress)
		if err != nil {
			return nil, err
		}
		steps := modulePath.Steps()
		parentPath, err := runtimeModulePathFromSteps(
			steps[:len(steps)-1],
		)
		if err != nil {
			return nil, err
		}
		lastStep := steps[len(steps)-1]
		call, ok := r.configIndex.moduleCalls[moduleCallKey{
			moduleAddress: parentPath.ConfigurationAddress(),
			moduleName:    lastStep.Name(),
		}]
		inputReferences, declared := call.inputReferences[name.Name]
		if !ok || !declared {
			return nil, nil
		}
		expressionKey := runtimeExpressionKey{
			moduleAddress: parentPath.ConfigurationAddress(),
			kind:          runtimeExpressionModuleCall,
			name:          lastStep.Name(),
			attribute:     name.Name,
		}
		if !r.configIndex.runtimeExpressionPreservesIdentity(expressionKey) {
			return nil, xerrors.Errorf(
				"module %q input %q does not preserve agent runtime identity",
				lastStep.Name(), name.Name,
			)
		}
		references, err := mostSpecificTerraformReferences(
			ctx,
			r.configIndex.runtimeExpressionValueReferences(
				expressionKey, inputReferences,
			),
		)
		if err != nil {
			return nil, err
		}
		return runtimeReferenceSources(
			parentPath.ConfigurationAddress(), references,
		), nil
	default:
		return nil, nil
	}
}

func rejectRuntimeAggregateIndirection(
	reference string,
	traversal hcl.Traversal,
	root hcl.TraverseRoot,
) error {
	var selectsAggregateValue bool
	switch root.Name {
	case "local", "var":
		selectsAggregateValue = len(traversal) > 2
	case "module":
		moduleAddress, position, err :=
			runtimeReferenceModuleAddress(traversal)
		if err != nil {
			return err
		}
		selectsAggregateValue = moduleAddress != "" &&
			position < len(traversal) && position+1 < len(traversal)
	}
	if !selectsAggregateValue {
		return nil
	}
	return xerrors.Errorf(
		"agent runtime reference %q selects a value within an aggregate; aggregate indirection is unsupported",
		reference,
	)
}

func runtimeReferenceSources(
	moduleAddress string,
	references []string,
) []runtimeReferenceSource {
	sources := make([]runtimeReferenceSource, 0, len(references))
	for _, reference := range references {
		sources = append(sources, runtimeReferenceSource{
			moduleAddress: moduleAddress,
			reference:     reference,
		})
	}
	return sources
}

func visitRuntimeSubstitutedIteratorReferences(
	references []string,
	collections []string,
	visit func(string) error,
) error {
	for _, reference := range references {
		suffix, iterator := runtimeIteratorReferenceSuffix(reference)
		if !iterator {
			if reference != "each" && !strings.HasPrefix(reference, "each.") {
				if err := visit(reference); err != nil {
					return err
				}
			}
			continue
		}
		for _, collection := range collections {
			if err := visit(collection + suffix); err != nil {
				return err
			}
		}
	}
	return nil
}

func runtimeIteratorReferenceSuffix(
	reference string,
) (string, bool) {
	for _, prefix := range []string{"each.value", `each["value"]`} {
		suffix, iterator := strings.CutPrefix(reference, prefix)
		if iterator && (suffix == "" || strings.HasPrefix(suffix, ".") ||
			strings.HasPrefix(suffix, "[")) {
			return suffix, true
		}
	}
	return "", false
}

func runtimeReferenceWithInstanceKey(
	reference string,
	collection string,
	instanceKey cty.Value,
) string {
	suffix, ok := strings.CutPrefix(reference, collection)
	if !ok || (suffix != "" && !strings.HasPrefix(suffix, ".")) {
		return reference
	}
	key, ok := runtimeInstanceKeyString(instanceKey)
	if !ok || key == "" {
		return reference
	}
	return collection + "[" + key + "]" + suffix
}

func runtimeCorrelatedCollection(reference string) (string, error) {
	traversal, err := parseRuntimeTraversal(reference)
	if err != nil {
		return "", err
	}
	if len(traversal) < 2 {
		return "", nil
	}
	root, ok := traversal[0].(hcl.TraverseRoot)
	if !ok {
		return "", nil
	}
	name, ok := traversal[1].(hcl.TraverseAttr)
	if !ok {
		return "", nil
	}
	switch root.Name {
	case "module":
		return "module." + name.Name, nil
	case "coder_agent", "coder_devcontainer":
		return root.Name + "." + name.Name, nil
	default:
		return "", nil
	}
}

func runtimeConfiguredResultSuffix(
	reference string,
	resultSuffix string,
) string {
	if resultSuffix == "" {
		return ""
	}
	target := runtimeReferenceTargetForReference(reference)
	if target != runtimeReferenceTargetAny {
		return ""
	}
	traversal, err := parseRuntimeTraversal(reference)
	if err != nil || len(traversal) == 0 {
		return ""
	}
	root, ok := traversal[0].(hcl.TraverseRoot)
	if !ok {
		return ""
	}
	switch root.Name {
	case "coder_agent", "coder_devcontainer", "local", "module", "var":
		return resultSuffix
	default:
		return ""
	}
}

func runtimeReferenceWithResultSuffix(
	reference string,
	resultSuffix string,
) string {
	if resultSuffix == "" {
		return reference
	}
	target := runtimeReferenceTargetForReference(reference)
	if target != runtimeReferenceTargetAny {
		return reference
	}
	target = runtimeReferenceTargetForReference(
		reference + resultSuffix,
	)
	if target == runtimeReferenceTargetAny {
		return reference
	}
	return reference + resultSuffix
}

func parseRuntimeTraversal(
	reference string,
) (hcl.Traversal, error) {
	traversal, diagnostics := hclsyntax.ParseTraversalAbs(
		[]byte(reference), "terraform-reference", hcl.InitialPos,
	)
	if diagnostics.HasErrors() {
		return nil, xerrors.Errorf(
			"invalid Terraform reference syntax: %s", diagnostics.Error(),
		)
	}
	return traversal, nil
}

func runtimeReferenceModuleAddress(
	traversal hcl.Traversal,
) (string, int, error) {
	var (
		parts    []string
		position int
	)
	for position < len(traversal) {
		name, ok := runtimeTraversalName(traversal[position])
		if !ok || name != "module" {
			break
		}
		if position+1 >= len(traversal) {
			return "", 0, xerrors.New(
				"module prefix must be followed by a module name",
			)
		}
		moduleName, ok := traversal[position+1].(hcl.TraverseAttr)
		if !ok {
			return "", 0, xerrors.New(
				"module prefix must be followed by a module name",
			)
		}
		parts = append(parts, "module."+moduleName.Name)
		position += 2
		if position < len(traversal) {
			if _, indexed := traversal[position].(hcl.TraverseIndex); indexed {
				position++
			}
		}
	}
	return strings.Join(parts, "."), position, nil
}

func runtimeTraversalName(
	traverser hcl.Traverser,
) (string, bool) {
	switch traverser := traverser.(type) {
	case hcl.TraverseRoot:
		return traverser.Name, true
	case hcl.TraverseAttr:
		return traverser.Name, true
	default:
		return "", false
	}
}

func runtimeModulePathFromSteps(
	steps []tfaddr.ModuleStep,
) (tfaddr.ModulePath, error) {
	var address strings.Builder
	for index, step := range steps {
		if index > 0 {
			_ = address.WriteByte('.')
		}
		_, _ = address.WriteString("module.")
		_, _ = address.WriteString(step.Name())
		key, ok := runtimeInstanceKeyString(step.InstanceKey())
		if !ok {
			return tfaddr.ModulePath{}, xerrors.New(
				"module instance key must be a string or integer",
			)
		}
		if key != "" {
			_ = address.WriteByte('[')
			_, _ = address.WriteString(key)
			_ = address.WriteByte(']')
		}
	}
	return tfaddr.ParseModulePath(address.String())
}

func runtimeInstanceKeyString(
	instanceKey cty.Value,
) (string, bool) {
	if instanceKey == cty.NilVal {
		return "", true
	}
	switch instanceKey.Type() {
	case cty.String:
		return strconv.Quote(instanceKey.AsString()), true
	case cty.Number:
		value, accuracy := instanceKey.AsBigFloat().Int64()
		return strconv.FormatInt(value, 10), accuracy == 0
	default:
		return "", false
	}
}

func runtimeQualifyConfigurationAddress(
	prefix string,
	address string,
) string {
	if prefix == "" {
		return address
	}
	if address == "" {
		return prefix
	}
	return prefix + "." + address
}
