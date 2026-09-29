package agentruntime

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/stretchr/testify/require"
)

func TestRuntimeResolverLimits(t *testing.T) {
	t.Parallel()

	t.Run("ReferenceCount", func(t *testing.T) {
		t.Parallel()

		resolver := runtimeResolverForTest(
			t,
			`digraph {
				"[root] coder_agent.first (expand)"
				"[root] coder_agent.second (expand)"
			}`,
			runtimeResolverTestRootConfig(
				runtimeResolverTestConfigResource(
					"coder_script", "work",
					"coder_agent.first.id", "coder_agent.second.id",
				),
			),
			runtimeResolverTestTargets(
				[]string{"coder_agent.first", "coder_agent.second"}, nil,
			),
		)
		resolver.limits.referenceCount = 1

		_, err := resolver.ResolveResourceRuntime(
			t.Context(),
			runtimeResolverTestStateResource(
				"coder_script.work", "coder_script", "work",
			),
		)
		require.ErrorContains(t, err, "limit of 1 Terraform references")
	})

	t.Run("ReferenceBytes", func(t *testing.T) {
		t.Parallel()

		const reference = "coder_agent.main.id"
		resolver := runtimeResolverForTest(
			t,
			`digraph { "[root] coder_agent.main (expand)" }`,
			runtimeResolverTestRootConfig(
				runtimeResolverTestConfigResource(
					"coder_script", "work", reference,
				),
			),
			runtimeResolverTestTargets([]string{"coder_agent.main"}, nil),
		)
		resolver.limits.referenceBytes = len(reference) - 1

		_, err := resolver.ResolveResourceRuntime(
			t.Context(),
			runtimeResolverTestStateResource(
				"coder_script.work", "coder_script", "work",
			),
		)
		require.ErrorContains(t, err, "Terraform reference bytes")
	})

	t.Run("CandidateChecks", func(t *testing.T) {
		t.Parallel()

		resolver := runtimeResolverForTest(
			t,
			`digraph { "[root] coder_agent.main (expand)" }`,
			runtimeResolverTestRootConfig(
				runtimeResolverTestConfigResource(
					"coder_script", "work", "coder_agent.main.id",
				),
			),
			runtimeResolverTestTargets(
				[]string{"coder_agent.main[0]", "coder_agent.main[1]"}, nil,
			),
		)
		resolver.limits.candidateChecks = 1

		_, err := resolver.ResolveResourceRuntime(
			t.Context(),
			runtimeResolverTestStateResource(
				"coder_script.work", "coder_script", "work",
			),
		)
		require.ErrorContains(t, err, "limit of 1 agent runtime candidate checks")
	})

	t.Run("CandidateMatchBytes", func(t *testing.T) {
		t.Parallel()

		resolver := runtimeResolverForTest(
			t,
			`digraph { "[root] coder_agent.main (expand)" }`,
			runtimeResolverTestRootConfig(
				runtimeResolverTestConfigResource(
					"coder_script", "work", "coder_agent.main.id",
				),
			),
			runtimeResolverTestTargets([]string{"coder_agent.main"}, nil),
		)
		resolver.limits.candidateMatchBytes = 1

		_, err := resolver.ResolveResourceRuntime(
			t.Context(),
			runtimeResolverTestStateResource(
				"coder_script.work", "coder_script", "work",
			),
		)
		require.ErrorContains(t, err, "agent runtime candidate match bytes")
	})

	t.Run("CachedReferences", func(t *testing.T) {
		t.Parallel()

		resolver := runtimeResolverForTest(
			t,
			`digraph { "[root] coder_agent.main (expand)" }`,
			runtimeResolverTestRootConfig(
				runtimeResolverTestConfigResource(
					"coder_script", "first", "coder_agent.main.id",
				),
				runtimeResolverTestConfigResource(
					"coder_script", "second", "coder_agent.main.id",
				),
			),
			runtimeResolverTestTargets([]string{"coder_agent.main"}, nil),
		)
		resolver.limits.referenceCacheEntries = 1

		_, err := resolver.ResolveResourceRuntime(
			t.Context(),
			runtimeResolverTestStateResource(
				"coder_script.first", "coder_script", "first",
			),
		)
		require.NoError(t, err)
		checks := resolver.budget.candidateChecks
		_, err = resolver.ResolveResourceRuntime(
			t.Context(),
			runtimeResolverTestStateResource(
				"coder_script.second", "coder_script", "second",
			),
		)
		require.NoError(t, err)
		require.Len(t, resolver.referenceNodes, 1)
		require.Greater(t, resolver.budget.candidateChecks, checks)
	})

	t.Run("ReferenceCacheEntries", func(t *testing.T) {
		t.Parallel()

		resolver, first, second := runtimeResolverCacheLimitTest(t)
		resolver.limits.referenceCacheEntries = 1
		_, err := resolver.ResolveResourceRuntime(t.Context(), first)
		require.NoError(t, err)
		_, err = resolver.ResolveResourceRuntime(t.Context(), second)
		require.ErrorContains(t, err, "cached Terraform reference queries")
	})

	t.Run("ReferenceCacheKeyBytes", func(t *testing.T) {
		t.Parallel()

		resolver, first, _ := runtimeResolverCacheLimitTest(t)
		resolver.limits.referenceCacheKeyBytes =
			len("coder_agent.first.id") - 1
		_, err := resolver.ResolveResourceRuntime(t.Context(), first)
		require.ErrorContains(
			t, err, "cached Terraform reference query bytes",
		)
	})

	t.Run("BoundedDeterministicDiagnostic", func(t *testing.T) {
		t.Parallel()

		agents := make([]string, 0, maxRuntimeDiagnosticCandidates+2)
		for index := range maxRuntimeDiagnosticCandidates + 2 {
			agents = append(
				agents,
				fmt.Sprintf("coder_agent.main[%d]", index),
			)
		}
		slices.Reverse(agents)
		resolver := runtimeResolverForTest(
			t,
			`digraph { "[root] coder_agent.main (expand)" }`,
			runtimeResolverTestRootConfig(
				runtimeResolverTestConfigResource(
					"coder_script", "work", "coder_agent.main.id",
				),
			),
			runtimeResolverTestTargets(agents, nil),
		)

		_, err := resolver.ResolveResourceRuntime(
			t.Context(),
			runtimeResolverTestStateResource(
				"coder_script.work", "coder_script", "work",
			),
		)
		require.ErrorContains(t, err, "2 omitted")
		require.Less(t, len(err.Error()), 4*1024)
		sortedAgents := slices.Clone(agents)
		slices.Sort(sortedAgents)
		require.Contains(
			t, err.Error(),
			strings.Join(
				sortedAgents[:maxRuntimeDiagnosticCandidates], ", ",
			),
		)
	})

	t.Run("Cancellation", func(t *testing.T) {
		t.Parallel()

		resolver := runtimeResolverForTest(
			t,
			`digraph { "[root] coder_agent.main (expand)" }`,
			runtimeResolverTestRootConfig(
				runtimeResolverTestConfigResource(
					"coder_script", "work", "coder_agent.main.id",
				),
			),
			runtimeResolverTestTargets([]string{"coder_agent.main"}, nil),
		)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		_, err := resolver.ResolveResourceRuntime(
			ctx,
			runtimeResolverTestStateResource(
				"coder_script.work", "coder_script", "work",
			),
		)
		require.ErrorIs(t, err, context.Canceled)
	})
}

func runtimeResolverCacheLimitTest(
	t *testing.T,
) (
	resultResolver *Resolver,
	firstResource *tfjson.StateResource,
	secondResource *tfjson.StateResource,
) {
	t.Helper()

	resolver := runtimeResolverForTest(
		t,
		`digraph {
			"[root] coder_agent.first (expand)"
			"[root] coder_agent.second (expand)"
		}`,
		runtimeResolverTestRootConfig(
			runtimeResolverTestConfigResource(
				"coder_script", "first", "coder_agent.first.id",
			),
			runtimeResolverTestConfigResource(
				"coder_script", "second", "coder_agent.second.id",
			),
		),
		runtimeResolverTestTargets(
			[]string{"coder_agent.first", "coder_agent.second"}, nil,
		),
	)
	return resolver,
		runtimeResolverTestStateResource(
			"coder_script.first", "coder_script", "first",
		),
		runtimeResolverTestStateResource(
			"coder_script.second", "coder_script", "second",
		)
}
