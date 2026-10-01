package agentruntime

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/stretchr/testify/require"
)

func TestRuntimeExpressionAnalysis(t *testing.T) {
	t.Parallel()

	t.Run("Identity", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name     string
			source   string
			expected bool
		}{
			{name: "Traversal", source: "coder_agent.main.id", expected: true},
			{name: "ExplicitResource", source: "resource.coder_agent.main.id", expected: true},
			{name: "Collection", source: "coder_agent.main", expected: true},
			{name: "Index", source: "local.agent_ids[each.key]", expected: true},
			{name: "EachValue", source: "each.value", expected: true},
			{name: "Parentheses", source: "(coder_agent.main.id)", expected: true},
			{name: "TemplateWrap", source: `"${coder_agent.main.id}"`, expected: true},
			{name: "Splat", source: "coder_agent.main[*].id", expected: true},
			{name: "Conditional", source: "var.enabled ? coder_agent.first.id : coder_agent.second.id", expected: true},
			{name: "Tuple", source: "[coder_agent.first.id, coder_agent.second.id]", expected: true},
			{name: "Object", source: "{first = coder_agent.first.id, second = coder_agent.second.id}", expected: true},
			{
				name: "IdentityComprehension",
				source: `{
  for name, agent in coder_agent.main : name => agent.id
}`,
				expected: true,
			},
			{
				name: "IndirectComprehension",
				source: `{
  for name, runtime in module.runtime : name => runtime.parent_agent_id
}`,
				expected: true,
			},
			{
				name: "NonIdentityComprehension",
				source: `{
  for name, agent in coder_agent.main : name => agent.name
}`,
			},
			{name: "Function", source: `uuidv5("dns", coder_agent.main.id)`},
			{name: "Attribute", source: "coder_agent.main.name"},
			{name: "UnknownResource", source: "data.external.agent.result"},
			{name: "Interpolation", source: `"agent-${coder_agent.main.id}"`},
			{name: "MixedConditional", source: "var.enabled ? coder_agent.main.id : coder_agent.main.name"},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				t.Parallel()

				expression := runtimeExpressionForTest(t, test.source)
				actual, err := runtimeExpressionPreservesIdentity(
					expression, runtimeProvenanceBudgetForTest(),
				)
				require.NoError(t, err)
				require.Equal(t, test.expected, actual)
			})
		}
	})

	t.Run("EachValue", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name     string
			source   string
			expected bool
		}{
			{name: "Traversal", source: "each.value.id", expected: true},
			{name: "Indexed", source: `each["value"].id`, expected: true},
			{name: "ConditionalResult", source: "var.enabled ? each.value.id : coder_agent.fallback.id", expected: true},
			{name: "Index", source: "coder_agent.target[each.value].id"},
			{name: "Condition", source: "each.value.enabled ? coder_agent.target.id : coder_agent.fallback.id"},
			{name: "ComprehensionValue", source: "[for agent in each.value : agent.id]", expected: true},
			{name: "ComprehensionCollection", source: "[for ignored in each.value : coder_agent.fallback.id]"},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				t.Parallel()

				actual, err := runtimeExpressionUsesEachValueAsValue(
					runtimeExpressionForTest(t, test.source),
					runtimeProvenanceBudgetForTest(),
				)
				require.NoError(t, err)
				require.Equal(t, test.expected, actual)
			})
		}
	})

	t.Run("ResultSuffix", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name     string
			source   string
			expected string
		}{
			{name: "EachValueParent", source: "each.value.agent_id", expected: ".agent_id"},
			{name: "DynamicParent", source: "coder_devcontainer.repo[count.index].agent_id", expected: ".agent_id"},
			{name: "IndirectParent", source: "local.repos[count.index].agent_id", expected: ".agent_id"},
			{name: "LocalName", source: "local.agent_id"},
			{name: "Conditional", source: "var.enabled ? each.value.agent_id : coder_devcontainer.fallback.agent_id", expected: ".agent_id"},
			{name: "Mixed", source: "var.enabled ? each.value.agent_id : coder_agent.fallback.id"},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				t.Parallel()

				actual, err := runtimeExpressionResultSuffix(
					runtimeExpressionForTest(t, test.source),
					runtimeProvenanceBudgetForTest(),
				)
				require.NoError(t, err)
				require.Equal(t, test.expected, actual)
			})
		}
	})

	t.Run("References", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name     string
			source   string
			expected []runtimeLocalReference
		}{
			{
				name:   "Indexed",
				source: `coder_devcontainer.repo["api"].agent_id`,
				expected: []runtimeLocalReference{{
					reference: `coder_devcontainer.repo["api"].agent_id`,
				}},
			},
			{
				name:   "ExplicitResource",
				source: `resource.coder_agent.main[1].id`,
				expected: []runtimeLocalReference{{
					reference: `coder_agent.main[1].id`,
				}},
			},
			{
				name:   "Conditional",
				source: "var.enabled ? coder_agent.first.id : coder_agent.second.id",
				expected: []runtimeLocalReference{
					{reference: "coder_agent.first.id"},
					{reference: "coder_agent.second.id"},
				},
			},
			{
				name:   "WrappedTuple",
				source: `tomap({first = (coder_agent.first.id), second = coder_agent.second[*].id})`,
				expected: []runtimeLocalReference{
					{reference: "coder_agent.first.id"},
					{reference: "coder_agent.second"},
				},
			},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				t.Parallel()

				actual, err := runtimeExpressionReferences(
					t.Context(),
					runtimeExpressionForTest(t, test.source),
					nil,
					runtimeProvenanceBudgetForTest(),
					nil,
				)
				require.NoError(t, err)
				require.Equal(t, test.expected, actual)
			})
		}
	})

	t.Run("ForExpressionKeys", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name                          string
			source                        string
			collectionPreservesSourceKeys bool
			expected                      bool
		}{
			{
				name: "IteratorKey",
				source: `{
  for name, runtime in module.runtime : name => runtime.agent_id
}`,
				collectionPreservesSourceKeys: true,
				expected:                      true,
			},
			{
				name: "NumericKey",
				source: `{
  for index, runtime in local.runtimes : index => runtime.agent_id
}`,
			},
			{
				name: "Function",
				source: `{
  for name, runtime in module.runtime : lower(name) => runtime.agent_id
}`,
				collectionPreservesSourceKeys: true,
			},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				t.Parallel()

				expression, ok := runtimeExpressionForTest(
					t, test.source,
				).(*hclsyntax.ForExpr)
				require.True(t, ok)
				actual, err := runtimeForExpressionPreservesKeys(
					expression,
					func(hcl.Expression, int) (bool, error) {
						return test.collectionPreservesSourceKeys, nil
					},
					0,
				)
				require.NoError(t, err)
				require.Equal(t, test.expected, actual)
			})
		}
	})
}

func TestRuntimeExpressionAnalysisBoundsDepth(t *testing.T) {
	t.Parallel()

	expression := runtimeExpressionForTest(t, "(each.value.agent_id)")
	limits := defaultProvenanceLimits()
	limits.expressionDepth = 1
	tests := []struct {
		name string
		walk func(*provenanceBudget) error
	}{
		{
			name: "Identity",
			walk: func(budget *provenanceBudget) error {
				_, err := runtimeExpressionPreservesIdentity(expression, budget)
				return err
			},
		},
		{
			name: "EachValue",
			walk: func(budget *provenanceBudget) error {
				_, err := runtimeExpressionUsesEachValueAsValue(expression, budget)
				return err
			},
		},
		{
			name: "Suffix",
			walk: func(budget *provenanceBudget) error {
				_, err := runtimeExpressionResultSuffix(expression, budget)
				return err
			},
		},
		{
			name: "References",
			walk: func(budget *provenanceBudget) error {
				_, err := runtimeExpressionReferences(
					t.Context(), expression, nil, budget, nil,
				)
				return err
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			err := test.walk(newProvenanceBudget(limits))
			require.ErrorContains(
				t, err, "limit of 1 nested Terraform expressions",
			)
		})
	}
}

func TestRuntimeExpressionAnalysisIndexesMetadata(t *testing.T) {
	t.Parallel()

	workdir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(workdir, "main.tf"),
		[]byte(`module "runtime" {
  source   = "./runtime"
  for_each = {}
}

locals {
  runtime_ids = tomap({
    for name, runtime in module.runtime : name => runtime.agent_id
  })
}

resource "coder_script" "work" {
  for_each = local.runtime_ids
  agent_id = each.value
}
`),
		0o600,
	))
	program, err := NewProgram(t.Context(), runtimeResolverTestRootConfig())
	require.NoError(t, err)
	require.NoError(t, program.LoadProvenance(t.Context(), workdir))
	index := program.configIndex
	require.True(t, index.runtimeSourceIndexed)
	require.True(t, index.runtimeIdentityExpressions[runtimeExpressionKey{
		kind:         runtimeExpressionResource,
		resourceType: "coder_script", name: "work", attribute: "agent_id",
	}])
	require.True(t, index.runtimeEachValueExpressions[runtimeExpressionKey{
		kind:         runtimeExpressionResource,
		resourceType: "coder_script", name: "work", attribute: "agent_id",
	}])
	require.Equal(t, []runtimeLocalReference{{
		reference:            "module.runtime.agent_id",
		correlatedCollection: "module.runtime",
	}}, index.localRuntime[runtimeLocalKey{name: "runtime_ids"}])
}

func runtimeExpressionForTest(t *testing.T, source string) hcl.Expression {
	t.Helper()

	expression, diagnostics := hclsyntax.ParseExpression(
		[]byte(source), "expression.tf", hcl.InitialPos,
	)
	require.False(t, diagnostics.HasErrors(), diagnostics.Error())
	return expression
}

func runtimeProvenanceBudgetForTest() *provenanceBudget {
	return newProvenanceBudget(defaultProvenanceLimits())
}
