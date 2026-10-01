// Package agentruntime resolves Terraform resources to the workspace agent
// or devcontainer subagent that executes them.
package agentruntime

import (
	"cmp"
	"context"
	"slices"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/provisioner/terraform/tfaddr"
	"github.com/coder/coder/v2/provisioner/terraform/tfgraph"
)

// Kind identifies the kind of agent runtime target.
type Kind int

const (
	// KindWorkspaceAgent identifies a workspace agent runtime.
	KindWorkspaceAgent Kind = iota + 1
	// KindDevcontainer identifies a devcontainer subagent runtime.
	KindDevcontainer
)

// Target identifies a Terraform resource that provides an agent runtime.
type Target struct {
	Kind    Kind
	Address string
}

const (
	maxRuntimeCatalogEntries      = 100_000
	maxRuntimeCatalogAddressBytes = 16 << 20
	maxDiagnosticValueRunes       = 256
)

type runtimeID int

type runtime struct {
	target Target
	parsed tfaddr.ManagedResourceAddress
}

// runtimeCatalog indexes concrete runtime instances by both their
// configuration and evaluated addresses. It is immutable after construction.
type runtimeCatalog struct {
	runtimes               []runtime
	byConfigurationAddress map[string][]runtimeID
	byInstanceAddress      map[string][]runtimeID
}

func newRuntimeCatalog(
	ctx context.Context,
	targets []Target,
) (*runtimeCatalog, error) {
	return newRuntimeCatalogWithLimits(
		ctx,
		targets,
		maxRuntimeCatalogEntries,
		maxRuntimeCatalogAddressBytes,
	)
}

func newRuntimeCatalogWithLimits(
	ctx context.Context,
	targets []Target,
	entryLimit int,
	addressByteLimit int,
) (*runtimeCatalog, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if exceedsLimit(0, len(targets), entryLimit) {
		return nil, xerrors.Errorf(
			"agent runtime resolution exceeds the limit of %d agent runtime instances",
			entryLimit,
		)
	}

	inputs := slices.Clone(targets)
	addressBytes := 0
	for _, target := range inputs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if exceedsLimit(addressBytes, len(target.Address), addressByteLimit) {
			return nil, xerrors.Errorf(
				"agent runtime resolution exceeds the limit of %d agent runtime address bytes",
				addressByteLimit,
			)
		}
		addressBytes += len(target.Address)
	}
	slices.SortFunc(inputs, func(a, b Target) int {
		return cmp.Or(
			cmp.Compare(a.Address, b.Address),
			cmp.Compare(a.Kind, b.Kind),
		)
	})

	catalog := &runtimeCatalog{
		runtimes:               make([]runtime, 0, len(inputs)),
		byConfigurationAddress: map[string][]runtimeID{},
		byInstanceAddress:      map[string][]runtimeID{},
	}
	for index, input := range inputs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if index > 0 && input == inputs[index-1] {
			continue
		}
		parsed, err := tfaddr.ParseManagedResourceAddress(input.Address)
		if err != nil {
			return nil, xerrors.Errorf(
				"parse agent runtime address %q: %s",
				truncateDiagnosticValue(input.Address),
				truncateDiagnosticValue(err.Error()),
			)
		}
		expectedType := ""
		switch input.Kind {
		case KindWorkspaceAgent:
			expectedType = "coder_agent"
		case KindDevcontainer:
			expectedType = "coder_devcontainer"
		default:
			return nil, xerrors.Errorf("unknown agent runtime kind %d", input.Kind)
		}
		if parsed.ResourceType() != expectedType {
			return nil, xerrors.Errorf(
				"agent runtime address %q has resource type %q; expected %q",
				truncateDiagnosticValue(input.Address),
				truncateDiagnosticValue(parsed.ResourceType()),
				expectedType,
			)
		}

		runtimeID := runtimeID(len(catalog.runtimes))
		catalog.runtimes = append(catalog.runtimes, runtime{
			target: input,
			parsed: parsed,
		})
		configurationAddress := parsed.ConfigurationAddress()
		catalog.byConfigurationAddress[configurationAddress] = append(
			catalog.byConfigurationAddress[configurationAddress], runtimeID,
		)
		catalog.byInstanceAddress[input.Address] = append(
			catalog.byInstanceAddress[input.Address], runtimeID,
		)
	}
	return catalog, nil
}

func (c *runtimeCatalog) runtimesForGraphNode(node tfgraph.Node) []runtimeID {
	if address := node.InstanceAddress(); address != "" {
		return slices.Clone(c.byInstanceAddress[address])
	}
	return slices.Clone(c.byConfigurationAddress[node.ConfigurationAddress()])
}

func exceedsLimit(used, additional, limit int) bool {
	return used > limit || additional > limit-used
}

func truncateDiagnosticValue(value string) string {
	runes := 0
	for index := range value {
		if runes == maxDiagnosticValueRunes {
			return value[:index] + "…"
		}
		runes++
	}
	return value
}
