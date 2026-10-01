package agentruntime

import (
	"context"
	"maps"
	"slices"
	"strings"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/zclconf/go-cty/cty"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/provisioner/terraform/tfaddr"
	"github.com/coder/coder/v2/provisioner/terraform/tfgraph"
)

type runtimeReferenceTarget int

const (
	runtimeReferenceTargetAny runtimeReferenceTarget = iota
	runtimeReferenceTargetWorkspaceAgent
	runtimeReferenceTargetDevcontainer
	// A coder_devcontainer's agent_id identifies its parent workspace agent,
	// not the devcontainer subagent identified by subagent_id.
	runtimeReferenceTargetDevcontainerParent
)

type runtimeCandidates struct {
	agents        []Target
	devcontainers []Target
}

// Resolver resolves Terraform resources to cataloged workspace-agent and
// devcontainer-subagent runtimes. It is request-local and must not be used
// concurrently.
type Resolver struct {
	graph       *tfgraph.Index
	query       *tfgraph.Query
	configIndex *configIndex
	runtimes    *runtimeCatalog
}

// NewResolver creates a request-local agent-runtime resolver.
func NewResolver(
	ctx context.Context,
	graph *tfgraph.Index,
	program *Program,
	targets []Target,
) (*Resolver, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if graph == nil {
		return nil, xerrors.New("Terraform graph is unavailable")
	}
	if program == nil || program.configIndex == nil {
		return nil, xerrors.New("Terraform plan configuration is unavailable")
	}
	runtimes, err := newRuntimeCatalog(ctx, targets)
	if err != nil {
		return nil, err
	}
	query, err := tfgraph.NewQuery(graph)
	if err != nil {
		return nil, err
	}
	return &Resolver{
		graph:       graph,
		query:       query,
		configIndex: program.configIndex,
		runtimes:    runtimes,
	}, nil
}

// ResolveResourceRuntime resolves a managed Terraform resource to one
// workspace-agent or devcontainer-subagent target.
func (r *Resolver) ResolveResourceRuntime(
	ctx context.Context,
	resource *tfjson.StateResource,
) (Target, error) {
	candidates, err := r.runtimeCandidates(ctx, resource)
	if err != nil {
		return Target{}, xerrors.Errorf(
			"resolve resource %q agent_id expression: %w",
			stateResourceDiagnosticAddress(resource), err,
		)
	}
	switch {
	case len(candidates.devcontainers) == 1 && len(candidates.agents) == 0:
		return candidates.devcontainers[0], nil
	case len(candidates.devcontainers) == 0 && len(candidates.agents) == 1:
		return candidates.agents[0], nil
	case len(candidates.devcontainers) > 0 && len(candidates.agents) > 0:
		return Target{}, xerrors.Errorf(
			"agent_id may refer to both devcontainer subagents (%s) and workspace agents (%s)",
			formatRuntimeCandidates(candidates.devcontainers),
			formatRuntimeCandidates(candidates.agents),
		)
	case len(candidates.devcontainers) > 1:
		return Target{}, xerrors.Errorf(
			"agent_id may refer to multiple devcontainer subagents: %s",
			formatRuntimeCandidates(candidates.devcontainers),
		)
	case len(candidates.agents) > 1:
		return Target{}, xerrors.Errorf(
			"agent_id may refer to multiple workspace agents: %s",
			formatRuntimeCandidates(candidates.agents),
		)
	default:
		return Target{}, xerrors.Errorf(
			"resource %q agent_id does not resolve to an agent runtime",
			stateResourceDiagnosticAddress(resource),
		)
	}
}

// ResolveWorkspaceAgent resolves a managed Terraform resource to one
// workspace-agent target and rejects devcontainer subagents.
func (r *Resolver) ResolveWorkspaceAgent(
	ctx context.Context,
	resource *tfjson.StateResource,
) (Target, error) {
	candidates, err := r.runtimeCandidates(ctx, resource)
	if err != nil {
		return Target{}, xerrors.Errorf(
			"resolve resource %q agent_id expression: %w",
			stateResourceDiagnosticAddress(resource), err,
		)
	}
	if len(candidates.devcontainers) > 0 {
		return Target{}, xerrors.Errorf(
			"resource %q agent_id may resolve to a devcontainer subagent; a workspace agent is required",
			stateResourceDiagnosticAddress(resource),
		)
	}
	if len(candidates.agents) != 1 {
		return Target{}, xerrors.Errorf(
			"resource %q agent_id resolves to %d workspace agents",
			stateResourceDiagnosticAddress(resource), len(candidates.agents),
		)
	}
	return candidates.agents[0], nil
}

func (r *Resolver) runtimeCandidates(
	ctx context.Context,
	resource *tfjson.StateResource,
) (runtimeCandidates, error) {
	if err := ctx.Err(); err != nil {
		return runtimeCandidates{}, err
	}
	if resource == nil {
		return runtimeCandidates{}, xerrors.New("Terraform resource is nil")
	}
	if resource.Mode != tfjson.ManagedResourceMode {
		return runtimeCandidates{}, xerrors.Errorf(
			"resource %q is not a managed Terraform resource",
			stateResourceDiagnosticAddress(resource),
		)
	}
	resourceAddress, references, err := r.resourceAgentIDReferences(ctx, resource)
	if err != nil {
		return runtimeCandidates{}, err
	}

	candidates := map[runtimeID]struct{}{}
	for _, reference := range references {
		if err := ctx.Err(); err != nil {
			return runtimeCandidates{}, err
		}
		target := runtimeReferenceTargetForReference(reference)
		terminalNodes, err := r.runtimeNodesForReference(
			ctx, resourceAddress.ModulePath(), reference, target,
		)
		if err != nil {
			return runtimeCandidates{}, err
		}
		constraint := runtimeInstanceConstraintForReference(
			resourceAddress.ModulePath(), reference,
		)
		if target == runtimeReferenceTargetDevcontainerParent {
			// An index on coder_devcontainer constrains the intermediate
			// devcontainer instance. Its parent coder_agent can have a different
			// address and instance key, so it cannot inherit that constraint.
			constraint = nil
		}
		for _, nodeID := range terminalNodes {
			node, ok := r.graph.Node(nodeID)
			if !ok {
				return runtimeCandidates{}, xerrors.New(
					"agent runtime resolution references a Terraform graph node outside its index",
				)
			}
			for _, runtimeID := range r.runtimes.runtimesForGraphNode(node) {
				runtime := r.runtimes.runtimes[runtimeID]
				if constraint != nil {
					if !constraint.matches(runtime.parsed) {
						continue
					}
				} else if !runtimeMatchesResourceModuleInstances(
					resourceAddress, runtime.parsed,
				) {
					continue
				}
				candidates[runtimeID] = struct{}{}
			}
		}
	}

	instanceCandidates, found, err := r.runtimeCandidatesFromInstanceGraph(
		ctx, resource.Address, candidates,
	)
	if err != nil {
		return runtimeCandidates{}, err
	}
	if found {
		candidates, err = intersectRuntimeCandidateSets(
			ctx, candidates, instanceCandidates,
		)
		if err != nil {
			return runtimeCandidates{}, err
		}
	}
	return r.partitionRuntimeCandidates(ctx, candidates)
}

func (r *Resolver) runtimeCandidatesFromInstanceGraph(
	ctx context.Context,
	resourceAddress string,
	configurationCandidates map[runtimeID]struct{},
) (map[runtimeID]struct{}, bool, error) {
	startNodes := r.graph.NodesForInstanceAddress(resourceAddress)
	if len(startNodes) == 0 {
		return nil, false, nil
	}

	candidates := map[runtimeID]struct{}{}
	for _, kind := range []Kind{KindWorkspaceAgent, KindDevcontainer} {
		if err := ctx.Err(); err != nil {
			return nil, true, err
		}
		hasCandidateKind := false
		for runtimeID := range configurationCandidates {
			if r.runtimes.runtimes[runtimeID].target.Kind == kind {
				hasCandidateKind = true
				break
			}
		}
		if !hasCandidateKind {
			continue
		}

		terminalNodes, err := r.query.ReachableTerminalNodes(
			ctx,
			startNodes,
			func(node tfgraph.Node) bool {
				for _, runtimeID := range r.runtimes.runtimesForGraphNode(node) {
					runtime := r.runtimes.runtimes[runtimeID]
					if runtime.target.Kind != kind ||
						runtime.target.Address == resourceAddress {
						continue
					}
					if _, allowed := configurationCandidates[runtimeID]; allowed {
						return true
					}
				}
				return false
			},
		)
		if err != nil {
			return nil, true, err
		}
		for _, nodeID := range terminalNodes {
			if err := ctx.Err(); err != nil {
				return nil, true, err
			}
			node, ok := r.graph.Node(nodeID)
			if !ok {
				return nil, true, xerrors.New(
					"agent runtime resolution references a Terraform graph node outside its index",
				)
			}
			for _, runtimeID := range r.runtimes.runtimesForGraphNode(node) {
				runtime := r.runtimes.runtimes[runtimeID]
				if runtime.target.Kind != kind ||
					runtime.target.Address == resourceAddress {
					continue
				}
				if _, allowed := configurationCandidates[runtimeID]; allowed {
					candidates[runtimeID] = struct{}{}
				}
			}
		}
	}
	return candidates, true, nil
}

func intersectRuntimeCandidateSets(
	ctx context.Context,
	left map[runtimeID]struct{},
	right map[runtimeID]struct{},
) (map[runtimeID]struct{}, error) {
	if len(left) > len(right) {
		left, right = right, left
	}
	intersection := make(map[runtimeID]struct{}, len(left))
	for runtimeID := range left {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if _, ok := right[runtimeID]; ok {
			intersection[runtimeID] = struct{}{}
		}
	}
	return intersection, nil
}

func (r *Resolver) runtimeNodesForReference(
	ctx context.Context,
	modulePath tfaddr.ModulePath,
	reference string,
	target runtimeReferenceTarget,
) ([]tfgraph.NodeID, error) {
	startNodes, err := r.query.ConfigurationNodesForReferences(
		ctx, modulePath.ConfigurationAddress(), []string{reference},
	)
	if err != nil {
		return nil, err
	}

	kinds := []Kind{KindWorkspaceAgent, KindDevcontainer}
	switch target {
	case runtimeReferenceTargetWorkspaceAgent,
		runtimeReferenceTargetDevcontainerParent:
		kinds = []Kind{KindWorkspaceAgent}
	case runtimeReferenceTargetDevcontainer:
		kinds = []Kind{KindDevcontainer}
	}

	seen := map[tfgraph.NodeID]struct{}{}
	var terminals []tfgraph.NodeID
	for _, kind := range kinds {
		nodes, err := r.query.ReachableTerminalNodes(
			ctx,
			startNodes,
			func(node tfgraph.Node) bool {
				return runtimeKindForGraphNode(node) == kind
			},
		)
		if err != nil {
			return nil, err
		}
		for _, nodeID := range nodes {
			if _, duplicate := seen[nodeID]; duplicate {
				continue
			}
			seen[nodeID] = struct{}{}
			terminals = append(terminals, nodeID)
		}
	}
	return terminals, nil
}

func (r *Resolver) resourceAgentIDReferences(
	ctx context.Context,
	resource *tfjson.StateResource,
) (tfaddr.ManagedResourceAddress, []string, error) {
	if err := ctx.Err(); err != nil {
		return tfaddr.ManagedResourceAddress{}, nil, err
	}
	parsed, err := tfaddr.ParseManagedResourceAddress(resource.Address)
	if err != nil {
		return tfaddr.ManagedResourceAddress{}, nil, xerrors.Errorf(
			"parse resource address %q: %s",
			stateResourceDiagnosticAddress(resource),
			truncateDiagnosticValue(err.Error()),
		)
	}
	if parsed.ResourceType() != resource.Type || parsed.ResourceName() != resource.Name {
		return tfaddr.ManagedResourceAddress{}, nil, xerrors.Errorf(
			"Terraform resource address %q does not match its state fields",
			stateResourceDiagnosticAddress(resource),
		)
	}
	configuredResource, ok, err := r.configIndex.resource(
		parsed.ModulePath().String(),
		tfjson.ManagedResourceMode,
		resource.Type,
		resource.Name,
	)
	if err != nil {
		return tfaddr.ManagedResourceAddress{}, nil, xerrors.Errorf(
			"find configuration for resource %q: %s",
			stateResourceDiagnosticAddress(resource),
			truncateDiagnosticValue(err.Error()),
		)
	}
	if !ok {
		return tfaddr.ManagedResourceAddress{}, nil, xerrors.Errorf(
			"resource %q is absent from Terraform plan configuration",
			stateResourceDiagnosticAddress(resource),
		)
	}
	if len(configuredResource.agentIDReferences) == 0 {
		return tfaddr.ManagedResourceAddress{}, nil, xerrors.Errorf(
			"resource %q agent_id expression has no Terraform references",
			stateResourceDiagnosticAddress(resource),
		)
	}
	additionalReferences := []string(nil)
	usesEachValueAsValue := false
	for _, reference := range configuredResource.agentIDReferences {
		suffix, eachValue := strings.CutPrefix(reference, "each.value")
		if eachValue && (suffix == "" || strings.HasPrefix(suffix, ".") ||
			strings.HasPrefix(suffix, "[")) {
			usesEachValueAsValue = true
			continue
		}
		if reference != "each" && !strings.HasPrefix(reference, "each.") {
			usesEachValueAsValue = false
			break
		}
	}
	if usesEachValueAsValue {
		additionalReferences = configuredResource.forEachReferences
	}
	references := make(
		[]string,
		0,
		len(configuredResource.agentIDReferences)+len(additionalReferences),
	)
	references = append(references, configuredResource.agentIDReferences...)
	references = append(references, additionalReferences...)
	references, err = mostSpecificTerraformReferences(ctx, references)
	if err != nil {
		return tfaddr.ManagedResourceAddress{}, nil, err
	}
	return parsed, references, nil
}

// mostSpecificTerraformReferences removes the traversal prefixes that
// Terraform includes alongside each complete expression reference. Treating an
// indexed reference's unindexed prefix as another reference would widen it to
// every instance of the resource or module call.
func mostSpecificTerraformReferences(
	ctx context.Context,
	references []string,
) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	result := make([]string, 0, len(references))
	seen := map[string]struct{}{}
	for index, reference := range references {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if index > 0 && terraformReferenceIsPrefix(reference, references[index-1]) {
			continue
		}
		if _, duplicate := seen[reference]; duplicate {
			continue
		}
		seen[reference] = struct{}{}
		result = append(result, reference)
	}
	return result, nil
}

func terraformReferenceIsPrefix(prefix, reference string) bool {
	return strings.HasPrefix(reference, prefix+".") ||
		strings.HasPrefix(reference, prefix+"[")
}

type runtimeInstanceConstraint struct {
	// An empty resourceType represents a module-instance constraint. Otherwise,
	// the remaining fields identify one concrete resource instance.
	moduleSteps  []tfaddr.ModuleStep
	resourceType string
	resourceName string
	instanceKey  cty.Value
}

func runtimeInstanceConstraintForReference(
	declaringModulePath tfaddr.ModulePath,
	reference string,
) *runtimeInstanceConstraint {
	if strings.HasPrefix(reference, "module.") {
		referencedModulePath, ok := referencedModulePath(reference)
		if !ok {
			return nil
		}
		referencedSteps := referencedModulePath.Steps()
		if !slices.ContainsFunc(referencedSteps, func(step tfaddr.ModuleStep) bool {
			return step.InstanceKey() != cty.NilVal
		}) {
			return nil
		}
		return &runtimeInstanceConstraint{
			moduleSteps: slices.Concat(
				declaringModulePath.Steps(), referencedSteps,
			),
		}
	}

	referencedResource, ok := referencedRuntimeResource(reference)
	if !ok || referencedResource.InstanceKey() == cty.NilVal {
		return nil
	}
	return &runtimeInstanceConstraint{
		moduleSteps: slices.Concat(
			declaringModulePath.Steps(), referencedResource.ModulePath().Steps(),
		),
		resourceType: referencedResource.ResourceType(),
		resourceName: referencedResource.ResourceName(),
		instanceKey:  referencedResource.InstanceKey(),
	}
}

func referencedModulePath(reference string) (tfaddr.ModulePath, bool) {
	for candidate := reference; candidate != ""; {
		modulePath, err := tfaddr.ParseModulePath(candidate)
		if err == nil && len(modulePath.Steps()) > 0 {
			return modulePath, true
		}
		separator := strings.LastIndexByte(candidate, '.')
		if separator < 0 {
			break
		}
		candidate = candidate[:separator]
	}
	return tfaddr.ModulePath{}, false
}

func referencedRuntimeResource(
	reference string,
) (tfaddr.ManagedResourceAddress, bool) {
	for candidate := reference; candidate != ""; {
		resource, err := tfaddr.ParseManagedResourceAddress(candidate)
		if err == nil && (resource.ResourceType() == "coder_agent" ||
			resource.ResourceType() == "coder_devcontainer") {
			return resource, true
		}
		separator := strings.LastIndexByte(candidate, '.')
		if separator < 0 {
			break
		}
		candidate = candidate[:separator]
	}
	return tfaddr.ManagedResourceAddress{}, false
}

func (c runtimeInstanceConstraint) matches(
	runtime tfaddr.ManagedResourceAddress,
) bool {
	runtimeSteps := runtime.ModulePath().Steps()
	if c.resourceType == "" &&
		!modulePathHasConfigurationPrefix(runtimeSteps, c.moduleSteps) {
		// A module output may re-export a runtime ID passed into the module.
		// Instance keys on the module reference do not constrain such an
		// external runtime resource.
		return true
	}
	if len(runtimeSteps) < len(c.moduleSteps) {
		return false
	}
	for index, expected := range c.moduleSteps {
		actual := runtimeSteps[index]
		if expected.Name() != actual.Name() {
			return false
		}
		if expected.InstanceKey() != cty.NilVal &&
			(actual.InstanceKey() == cty.NilVal || !tfaddr.InstanceKeysEqual(
				expected.InstanceKey(), actual.InstanceKey(),
			)) {
			return false
		}
	}
	if c.resourceType == "" {
		return true
	}
	return len(runtimeSteps) == len(c.moduleSteps) &&
		runtime.ResourceType() == c.resourceType &&
		runtime.ResourceName() == c.resourceName &&
		runtime.InstanceKey() != cty.NilVal &&
		tfaddr.InstanceKeysEqual(c.instanceKey, runtime.InstanceKey())
}

func modulePathHasConfigurationPrefix(
	path []tfaddr.ModuleStep,
	prefix []tfaddr.ModuleStep,
) bool {
	if len(path) < len(prefix) {
		return false
	}
	for index, expected := range prefix {
		if path[index].Name() != expected.Name() {
			return false
		}
	}
	return true
}

func runtimeMatchesResourceModuleInstances(
	resource tfaddr.ManagedResourceAddress,
	runtime tfaddr.ManagedResourceAddress,
) bool {
	resourceSteps := resource.ModulePath().Steps()
	runtimeSteps := runtime.ModulePath().Steps()
	for index := range min(len(resourceSteps), len(runtimeSteps)) {
		resourceStep := resourceSteps[index]
		runtimeStep := runtimeSteps[index]
		if resourceStep.Name() != runtimeStep.Name() {
			break
		}
		if resourceStep.InstanceKey() != cty.NilVal &&
			runtimeStep.InstanceKey() != cty.NilVal &&
			!tfaddr.InstanceKeysEqual(
				resourceStep.InstanceKey(), runtimeStep.InstanceKey(),
			) {
			return false
		}
	}
	return true
}

func runtimeReferenceTargetForReference(reference string) runtimeReferenceTarget {
	attribute := ""
	resourceAddress := ""
	for _, candidate := range []string{"id", "subagent_id", "agent_id"} {
		if prefix, ok := strings.CutSuffix(reference, "."+candidate); ok {
			attribute = candidate
			resourceAddress = prefix
			break
		}
	}
	if attribute == "" {
		return runtimeReferenceTargetAny
	}
	parsed, err := tfaddr.ParseManagedResourceAddress(resourceAddress)
	if err != nil {
		return runtimeReferenceTargetAny
	}
	switch {
	case parsed.ResourceType() == "coder_agent" && attribute == "id":
		return runtimeReferenceTargetWorkspaceAgent
	case parsed.ResourceType() == "coder_devcontainer" && attribute == "subagent_id":
		return runtimeReferenceTargetDevcontainer
	case parsed.ResourceType() == "coder_devcontainer" && attribute == "agent_id":
		return runtimeReferenceTargetDevcontainerParent
	default:
		return runtimeReferenceTargetAny
	}
}

func runtimeKindForGraphNode(node tfgraph.Node) Kind {
	parsed, err := tfaddr.ParseManagedResourceAddress(node.Address())
	if err != nil {
		return 0
	}
	switch parsed.ResourceType() {
	case "coder_agent":
		return KindWorkspaceAgent
	case "coder_devcontainer":
		return KindDevcontainer
	default:
		return 0
	}
}

func (r *Resolver) partitionRuntimeCandidates(
	ctx context.Context,
	candidates map[runtimeID]struct{},
) (runtimeCandidates, error) {
	var result runtimeCandidates
	for _, runtimeID := range slices.Sorted(maps.Keys(candidates)) {
		if err := ctx.Err(); err != nil {
			return runtimeCandidates{}, err
		}
		target := r.runtimes.runtimes[runtimeID].target
		switch target.Kind {
		case KindWorkspaceAgent:
			result.agents = append(result.agents, target)
		case KindDevcontainer:
			result.devcontainers = append(result.devcontainers, target)
		}
	}
	slices.SortFunc(result.agents, compareTargets)
	slices.SortFunc(result.devcontainers, compareTargets)
	return result, nil
}

func compareTargets(a, b Target) int {
	return strings.Compare(a.Address, b.Address)
}

func stateResourceDiagnosticAddress(resource *tfjson.StateResource) string {
	if resource == nil {
		return "<nil>"
	}
	return truncateDiagnosticValue(resource.Address)
}

func formatRuntimeCandidates(targets []Target) string {
	addresses := make([]string, 0, len(targets))
	for _, target := range targets {
		addresses = append(addresses, target.Address)
	}
	return strings.Join(addresses, ", ")
}
