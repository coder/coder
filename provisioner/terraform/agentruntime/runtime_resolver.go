package agentruntime

import (
	"context"
	"maps"
	"slices"
	"strings"

	tfjson "github.com/hashicorp/terraform-json"
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
		for _, nodeID := range terminalNodes {
			node, ok := r.graph.Node(nodeID)
			if !ok {
				return runtimeCandidates{}, xerrors.New(
					"agent runtime resolution references a Terraform graph node outside its index",
				)
			}
			for _, runtimeID := range r.runtimes.runtimesForGraphNode(node) {
				candidates[runtimeID] = struct{}{}
			}
		}
	}
	return r.partitionRuntimeCandidates(ctx, candidates)
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
	references, err := mostSpecificTerraformReferences(
		ctx, configuredResource.agentIDReferences,
	)
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
