package scriptorder

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildScriptOrderGraphs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		rules           []resolvedScriptOrderRule
		expected        ScriptOrder
		errorSubstrings []string
	}{
		{
			name:     "NoRules",
			rules:    nil,
			expected: ScriptOrder{},
		},
		{
			name: "EveryDependentRequiresEveryPrerequisite",
			rules: []resolvedScriptOrderRule{scriptOrderGraphTestRule(
				"data.coder_script_order.order", 0,
				"coder_agent.main", ScriptOrderPhaseStart,
				ScriptOrderRequirementCompletion,
				[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector(
					"run", "coder_script.work",
					`coder_script.work["worker"]`,
					`coder_script.work["api"]`,
				)},
				[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector(
					"after", "coder_script.setup",
					`coder_script.setup["repository"]`,
					`coder_script.setup["database"]`,
				)},
			)},
			expected: ScriptOrder{Graphs: []ScriptOrderGraph{{
				RuntimeAddress: "coder_agent.main",
				Phase:          ScriptOrderPhaseStart,
				Dependencies: []ScriptOrderDependency{
					scriptOrderGraphTestDependency(`coder_script.work["api"]`, `coder_script.setup["database"]`, ScriptOrderRequirementCompletion),
					scriptOrderGraphTestDependency(`coder_script.work["api"]`, `coder_script.setup["repository"]`, ScriptOrderRequirementCompletion),
					scriptOrderGraphTestDependency(`coder_script.work["worker"]`, `coder_script.setup["database"]`, ScriptOrderRequirementCompletion),
					scriptOrderGraphTestDependency(`coder_script.work["worker"]`, `coder_script.setup["repository"]`, ScriptOrderRequirementCompletion),
				},
			}}},
		},
		{
			name: "RulesAndDataSourcesComposeAndDeduplicate",
			rules: []resolvedScriptOrderRule{
				scriptOrderGraphTestRule(
					"data.coder_script_order.first", 0,
					"coder_agent.main", ScriptOrderPhaseStart,
					ScriptOrderRequirementSuccess,
					[]resolvedScriptOrderSelector{
						scriptOrderGraphTestSelector("run", "coder_script.work[0]", "coder_script.work[0]"),
						scriptOrderGraphTestSelector("run", "coder_script.work", "coder_script.work[0]", "coder_script.work[1]"),
					},
					[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector(
						"after", "coder_script.a", "coder_script.a",
					)},
				),
				scriptOrderGraphTestRule(
					"data.coder_script_order.second", 0,
					"coder_agent.main", ScriptOrderPhaseStart,
					ScriptOrderRequirementSuccess,
					[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector(
						"run", "coder_script.work[0]", "coder_script.work[0]",
					)},
					[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector(
						"after", "coder_script.a", "coder_script.a",
					)},
				),
			},
			expected: ScriptOrder{Graphs: []ScriptOrderGraph{{
				RuntimeAddress: "coder_agent.main",
				Phase:          ScriptOrderPhaseStart,
				Dependencies: []ScriptOrderDependency{
					scriptOrderGraphTestDependency("coder_script.work[0]", "coder_script.a", ScriptOrderRequirementSuccess),
					scriptOrderGraphTestDependency("coder_script.work[1]", "coder_script.a", ScriptOrderRequirementSuccess),
				},
			}}},
		},
		{
			name: "DisconnectedComponentsRemainDisconnected",
			rules: []resolvedScriptOrderRule{
				scriptOrderGraphTestRule(
					"data.coder_script_order.order", 0,
					"coder_agent.main", ScriptOrderPhaseStart,
					ScriptOrderRequirementSuccess,
					[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector("run", "coder_script.b", "coder_script.b")},
					[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector("after", "coder_script.a", "coder_script.a")},
				),
				scriptOrderGraphTestRule(
					"data.coder_script_order.order", 1,
					"coder_agent.main", ScriptOrderPhaseStart,
					ScriptOrderRequirementSuccess,
					[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector("run", "coder_script.d", "coder_script.d")},
					[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector("after", "coder_script.c", "coder_script.c")},
				),
			},
			expected: ScriptOrder{Graphs: []ScriptOrderGraph{{
				RuntimeAddress: "coder_agent.main",
				Phase:          ScriptOrderPhaseStart,
				Dependencies: []ScriptOrderDependency{
					scriptOrderGraphTestDependency("coder_script.b", "coder_script.a", ScriptOrderRequirementSuccess),
					scriptOrderGraphTestDependency("coder_script.d", "coder_script.c", ScriptOrderRequirementSuccess),
				},
			}}},
		},
		// A --success----> B
		// B --completion-> C
		// A --success----> C
		//
		// If A fails, B is skipped and therefore satisfies B-to-C's
		// completion requirement. However, A-to-C's success requirement
		// remains unsatisfied, so C is skipped.
		{
			name: "RetainsExplicitEdgeEvenWhenTransitivePathExists",
			rules: []resolvedScriptOrderRule{
				scriptOrderGraphTestRule(
					"data.coder_script_order.order", 0,
					"coder_agent.main", ScriptOrderPhaseStart,
					ScriptOrderRequirementSuccess,
					[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector(
						"run", "coder_script.b", "coder_script.b",
					)},
					[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector(
						"after", "coder_script.a", "coder_script.a",
					)},
				),
				scriptOrderGraphTestRule(
					"data.coder_script_order.order", 1,
					"coder_agent.main", ScriptOrderPhaseStart,
					ScriptOrderRequirementCompletion,
					[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector(
						"run", "coder_script.c", "coder_script.c",
					)},
					[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector(
						"after", "coder_script.b", "coder_script.b",
					)},
				),
				scriptOrderGraphTestRule(
					"data.coder_script_order.order", 2,
					"coder_agent.main", ScriptOrderPhaseStart,
					ScriptOrderRequirementSuccess,
					[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector(
						"run", "coder_script.c", "coder_script.c",
					)},
					[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector(
						"after", "coder_script.a", "coder_script.a",
					)},
				),
			},
			expected: ScriptOrder{Graphs: []ScriptOrderGraph{{
				RuntimeAddress: "coder_agent.main",
				Phase:          ScriptOrderPhaseStart,
				Dependencies: []ScriptOrderDependency{
					scriptOrderGraphTestDependency("coder_script.b", "coder_script.a", ScriptOrderRequirementSuccess),
					scriptOrderGraphTestDependency("coder_script.c", "coder_script.a", ScriptOrderRequirementSuccess),
					scriptOrderGraphTestDependency("coder_script.c", "coder_script.b", ScriptOrderRequirementCompletion),
				},
			}}},
		},
		// Nonzero indexes verify that diagnostics retain each rule's
		// original position within its data source after no-op rules
		// are removed.
		{
			name: "ConflictingRequirementsFail",
			rules: []resolvedScriptOrderRule{
				scriptOrderGraphTestRule(
					"data.coder_script_order.first", 2,
					"coder_agent.main", ScriptOrderPhaseStart,
					ScriptOrderRequirementSuccess,
					[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector("run", "coder_script.b", "coder_script.b")},
					[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector("after", "coder_script.a", "coder_script.a")},
				),
				scriptOrderGraphTestRule(
					"data.coder_script_order.second", 1,
					"coder_agent.main", ScriptOrderPhaseStart,
					ScriptOrderRequirementCompletion,
					[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector("run", "coder_script.b", "coder_script.b")},
					[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector("after", "coder_script.a", "coder_script.a")},
				),
			},
			errorSubstrings: []string{
				`data.coder_script_order.second`, `rule 1`, `run selector "coder_script.b"`,
				`after selector "coder_script.a"`, `script "coder_script.b" after "coder_script.a"`,
				`requires "completion"`, `requires "success"`, `data source "data.coder_script_order.first" rule 2`,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			actual, err := buildScriptOrderGraphs(test.rules)
			if len(test.errorSubstrings) > 0 {
				require.Error(t, err)
				require.Empty(t, actual)
				for _, substring := range test.errorSubstrings {
					require.ErrorContains(t, err, substring)
				}
				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expected, actual)
		})
	}
}

func TestAddScriptOrderRuleEdgesEnforcesCombinationLimit(t *testing.T) {
	t.Parallel()

	t.Run("DeduplicatesBeforeCountingCombinations", func(t *testing.T) {
		t.Parallel()

		graph := &scriptOrderGraphAccumulator{
			dependencies: map[string]map[string]scriptOrderEdge{},
		}
		rule := scriptOrderGraphTestRule(
			"data.coder_script_order.order", 0,
			"coder_agent.main", ScriptOrderPhaseStart,
			ScriptOrderRequirementSuccess,
			[]resolvedScriptOrderSelector{
				scriptOrderGraphTestSelector("run", "coder_script.work[0]", "coder_script.work[0]"),
				scriptOrderGraphTestSelector("run", "coder_script.work", "coder_script.work[0]"),
			},
			[]resolvedScriptOrderSelector{
				scriptOrderGraphTestSelector("after", "coder_script.setup[0]", "coder_script.setup[0]"),
				scriptOrderGraphTestSelector("after", "coder_script.setup", "coder_script.setup[0]"),
			},
		)

		budget := scriptOrderCombinationBudget{limit: 1}
		err := addScriptOrderRuleEdges(graph, rule, &budget)
		require.NoError(t, err)
		require.Equal(t, 1, graph.dependencyCount)
		require.Equal(t, 1, budget.used)
		edge := graph.dependencies["coder_script.work[0]"]["coder_script.setup[0]"]
		require.Equal(t, "coder_script.work[0]", edge.runSelector)
		require.Equal(t, "coder_script.setup[0]", edge.afterSelector)
	})

	t.Run("RejectsOneRuleAboveLimit", func(t *testing.T) {
		t.Parallel()

		graph := &scriptOrderGraphAccumulator{
			dependencies: map[string]map[string]scriptOrderEdge{},
		}
		rule := scriptOrderGraphTestRule(
			"data.coder_script_order.order", 2,
			"coder_agent.main", ScriptOrderPhaseStart,
			ScriptOrderRequirementSuccess,
			[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector(
				"run", "module.work",
				"module.work.coder_script.c",
				"module.work.coder_script.d",
			)},
			[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector(
				"after", "module.setup",
				"module.setup.coder_script.a",
				"module.setup.coder_script.b",
			)},
		)

		budget := scriptOrderCombinationBudget{limit: 3}
		err := addScriptOrderRuleEdges(graph, rule, &budget)
		require.ErrorContains(t, err, `script order data source "data.coder_script_order.order" rule 2`)
		require.ErrorContains(t, err, "run selectors resolve to 2 scripts")
		require.ErrorContains(t, err, "after selectors resolve to 2 scripts")
		require.ErrorContains(t, err, "overall limit of 3 combinations across all script ordering rules")
		require.Zero(t, graph.dependencyCount)
		require.Zero(t, budget.used)
	})
}

func TestBuildScriptOrderGraphsEnforcesOverallCombinationLimit(t *testing.T) {
	t.Parallel()

	t.Run("AcrossGraphs", func(t *testing.T) {
		t.Parallel()

		rules := []resolvedScriptOrderRule{
			scriptOrderGraphTestRule(
				"data.coder_script_order.order", 0,
				"coder_agent.main", ScriptOrderPhaseStop,
				ScriptOrderRequirementSuccess,
				[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector(
					"run", "coder_script.c", "coder_script.c",
				)},
				[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector(
					"after", "module.setup",
					"module.setup.coder_script.a",
					"module.setup.coder_script.b",
				)},
			),
			scriptOrderGraphTestRule(
				"data.coder_script_order.order", 1,
				"coder_devcontainer.dev", ScriptOrderPhaseStart,
				ScriptOrderRequirementSuccess,
				[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector(
					"run", "coder_script.d", "coder_script.d",
				)},
				[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector(
					"after", "coder_script.a", "coder_script.a",
				)},
			),
		}

		order, err := buildScriptOrderGraphsWithCombinationLimit(rules, 2)
		require.ErrorContains(t, err, `data.coder_script_order.order`)
		require.ErrorContains(t, err, `rule 1`)
		require.ErrorContains(t, err, "limited to 2 run/after script combinations in total")
		require.ErrorContains(t, err, "2 from earlier rules together with 1 from this rule exceed the limit")
		require.Empty(t, order)
	})

	// The limit bounds conversion work, not only final graph size. If
	// combinations were counted after cross-rule edge deduplication,
	// repeated rules could cause unbounded work while producing only
	// one edge.
	t.Run("CountsRepeatedEdgesTowardLimit", func(t *testing.T) {
		t.Parallel()

		rule := scriptOrderGraphTestRule(
			"data.coder_script_order.order", 0,
			"coder_agent.main", ScriptOrderPhaseStart,
			ScriptOrderRequirementSuccess,
			[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector(
				"run", "coder_script.b", "coder_script.b",
			)},
			[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector(
				"after", "coder_script.a", "coder_script.a",
			)},
		)
		rules := []resolvedScriptOrderRule{rule, rule, rule}
		rules[1].ruleIndex = 1
		rules[2].ruleIndex = 2

		order, err := buildScriptOrderGraphsWithCombinationLimit(rules, 2)
		require.ErrorContains(t, err, `rule 2`)
		require.ErrorContains(t, err, "limited to 2 run/after script combinations in total")
		require.ErrorContains(t, err, "2 from earlier rules together with 1 from this rule exceed the limit")
		require.Empty(t, order)
	})
}

func TestBuildScriptOrderGraphsBoundsConflictingRequirementDiagnostic(t *testing.T) {
	t.Parallel()

	longRunSelector := "coder_script." + strings.Repeat(
		"r", maxScriptOrderDiagnosticValueRunes+1,
	)
	longAfterSelector := "coder_script." + strings.Repeat(
		"a", maxScriptOrderDiagnosticValueRunes+1,
	)
	longFirstDataSource := "data.coder_script_order." + strings.Repeat(
		"f", maxScriptOrderDiagnosticValueRunes+1,
	)
	longSecondDataSource := "data.coder_script_order." + strings.Repeat(
		"s", maxScriptOrderDiagnosticValueRunes+1,
	)
	rules := []resolvedScriptOrderRule{
		scriptOrderGraphTestRule(
			longFirstDataSource, 0,
			"coder_agent.main", ScriptOrderPhaseStart,
			ScriptOrderRequirementSuccess,
			[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector(
				"run", longRunSelector, longRunSelector,
			)},
			[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector(
				"after", longAfterSelector, longAfterSelector,
			)},
		),
		scriptOrderGraphTestRule(
			longSecondDataSource, 0,
			"coder_agent.main", ScriptOrderPhaseStart,
			ScriptOrderRequirementCompletion,
			[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector(
				"run", longRunSelector, longRunSelector,
			)},
			[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector(
				"after", longAfterSelector, longAfterSelector,
			)},
		),
	}

	actual, err := buildScriptOrderGraphs(rules)
	require.Error(t, err)
	require.Empty(t, actual)
	require.Contains(t, err.Error(), "…")
	require.NotContains(t, err.Error(), longRunSelector)
	require.NotContains(t, err.Error(), longAfterSelector)
	require.NotContains(t, err.Error(), longFirstDataSource)
	require.NotContains(t, err.Error(), longSecondDataSource)
	require.Less(t, len(err.Error()), 4*1024)
}

func TestBuildScriptOrderGraphsSeparatesRuntimeAndPhase(t *testing.T) {
	t.Parallel()

	rules := []resolvedScriptOrderRule{
		scriptOrderGraphTestRule(
			"data.coder_script_order.order", 0,
			"coder_devcontainer.dev", ScriptOrderPhaseStart,
			ScriptOrderRequirementSuccess,
			[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector("run", "coder_script.dc_b", "coder_script.dc_b")},
			[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector("after", "coder_script.dc_a", "coder_script.dc_a")},
		),
		scriptOrderGraphTestRule(
			"data.coder_script_order.order", 1,
			"coder_agent.main", ScriptOrderPhaseStop,
			ScriptOrderRequirementSuccess,
			[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector("run", "coder_script.stop_b", "coder_script.stop_b")},
			[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector("after", "coder_script.stop_a", "coder_script.stop_a")},
		),
		scriptOrderGraphTestRule(
			"data.coder_script_order.order", 2,
			"coder_agent.main", ScriptOrderPhaseStart,
			ScriptOrderRequirementSuccess,
			[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector("run", "coder_script.start_b", "coder_script.start_b")},
			[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector("after", "coder_script.start_a", "coder_script.start_a")},
		),
	}

	actual, err := buildScriptOrderGraphs(rules)
	require.NoError(t, err)
	require.Equal(t, ScriptOrder{Graphs: []ScriptOrderGraph{
		scriptOrderGraphTestGraph(
			"coder_agent.main", ScriptOrderPhaseStart,
			"coder_script.start_b", "coder_script.start_a",
		),
		scriptOrderGraphTestGraph(
			"coder_agent.main", ScriptOrderPhaseStop,
			"coder_script.stop_b", "coder_script.stop_a",
		),
		scriptOrderGraphTestGraph(
			"coder_devcontainer.dev", ScriptOrderPhaseStart,
			"coder_script.dc_b", "coder_script.dc_a",
		),
	}}, actual)
}

func TestBuildScriptOrderGraphsRejectsCycles(t *testing.T) {
	t.Parallel()

	rules := []resolvedScriptOrderRule{
		scriptOrderGraphTestRule(
			"data.coder_script_order.first", 0,
			"coder_agent.main", ScriptOrderPhaseStart,
			ScriptOrderRequirementSuccess,
			[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector("run", "coder_script.b", "coder_script.b")},
			[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector("after", "coder_script.a", "coder_script.a")},
		),
		scriptOrderGraphTestRule(
			"data.coder_script_order.first", 1,
			"coder_agent.main", ScriptOrderPhaseStart,
			ScriptOrderRequirementSuccess,
			[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector("run", "coder_script.c", "coder_script.c")},
			[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector("after", "coder_script.b", "coder_script.b")},
		),
		scriptOrderGraphTestRule(
			"data.coder_script_order.second", 0,
			"coder_agent.main", ScriptOrderPhaseStart,
			ScriptOrderRequirementSuccess,
			[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector("run", "coder_script.a", "coder_script.a")},
			[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector("after", "coder_script.c", "coder_script.c")},
		),
	}

	actual, err := buildScriptOrderGraphs(rules)
	require.Error(t, err)
	require.Empty(t, actual)
	for _, text := range []string{
		`script order dependency cycle`,
		`coder_script.a -> coder_script.b -> coder_script.c -> coder_script.a`,
		`"coder_script.b" after "coder_script.a" from run selector "coder_script.b" and after selector "coder_script.a" (data source "data.coder_script_order.first" rule 0)`,
		`"coder_script.c" after "coder_script.b" from run selector "coder_script.c" and after selector "coder_script.b" (data source "data.coder_script_order.first" rule 1)`,
		`"coder_script.a" after "coder_script.c" from run selector "coder_script.a" and after selector "coder_script.c" (data source "data.coder_script_order.second" rule 0)`,
	} {
		require.ErrorContains(t, err, text)
	}
}

func TestBuildScriptOrderGraphsSelectsCycleDeterministically(t *testing.T) {
	t.Parallel()

	// Supply the lexically later X-Y cycle first, then give A two
	// cycle-forming prerequisites in reverse lexical order.
	// Diagnostics must still select the lexically first root and
	// prerequisite.
	rules := []resolvedScriptOrderRule{
		scriptOrderGraphTestRule(
			"data.coder_script_order.order", 0,
			"coder_agent.main", ScriptOrderPhaseStart,
			ScriptOrderRequirementSuccess,
			[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector("run", "coder_script.x", "coder_script.x")},
			[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector("after", "coder_script.y", "coder_script.y")},
		),
		scriptOrderGraphTestRule(
			"data.coder_script_order.order", 1,
			"coder_agent.main", ScriptOrderPhaseStart,
			ScriptOrderRequirementSuccess,
			[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector("run", "coder_script.y", "coder_script.y")},
			[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector("after", "coder_script.x", "coder_script.x")},
		),
		scriptOrderGraphTestRule(
			"data.coder_script_order.order", 2,
			"coder_agent.main", ScriptOrderPhaseStart,
			ScriptOrderRequirementSuccess,
			[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector("run", "coder_script.a", "coder_script.a")},
			[]resolvedScriptOrderSelector{
				scriptOrderGraphTestSelector("after", "coder_script.c", "coder_script.c"),
				scriptOrderGraphTestSelector("after", "coder_script.b", "coder_script.b"),
			},
		),
		scriptOrderGraphTestRule(
			"data.coder_script_order.order", 3,
			"coder_agent.main", ScriptOrderPhaseStart,
			ScriptOrderRequirementSuccess,
			[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector("run", "coder_script.c", "coder_script.c")},
			[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector("after", "coder_script.a", "coder_script.a")},
		),
		scriptOrderGraphTestRule(
			"data.coder_script_order.order", 4,
			"coder_agent.main", ScriptOrderPhaseStart,
			ScriptOrderRequirementSuccess,
			[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector("run", "coder_script.b", "coder_script.b")},
			[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector("after", "coder_script.a", "coder_script.a")},
		),
	}

	actual, err := buildScriptOrderGraphs(rules)
	require.ErrorContains(
		t,
		err,
		"coder_script.a -> coder_script.b -> coder_script.a",
	)
	require.NotContains(t, err.Error(), "coder_script.c")
	require.NotContains(t, err.Error(), "coder_script.x")
	require.NotContains(t, err.Error(), "coder_script.y")
	require.Empty(t, actual)
}

func TestBuildScriptOrderGraphsRejectsCycleAfterAcyclicPrerequisite(t *testing.T) {
	t.Parallel()

	// A depends on instances 0 and 1 of coder_script.prerequisite.
	// Instance 0 is acyclic, while instance 1 depends on A and forms
	// the reported cycle.
	rules := []resolvedScriptOrderRule{
		scriptOrderGraphTestRule(
			"data.coder_script_order.order", 0,
			"coder_agent.main", ScriptOrderPhaseStart,
			ScriptOrderRequirementSuccess,
			[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector(
				"run", "coder_script.a", "coder_script.a",
			)},
			[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector(
				"after", "coder_script.prerequisite",
				"coder_script.prerequisite[0]",
				"coder_script.prerequisite[1]",
			)},
		),
		scriptOrderGraphTestRule(
			"data.coder_script_order.order", 1,
			"coder_agent.main", ScriptOrderPhaseStart,
			ScriptOrderRequirementSuccess,
			[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector(
				"run", "coder_script.prerequisite[1]",
				"coder_script.prerequisite[1]",
			)},
			[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector(
				"after", "coder_script.a", "coder_script.a",
			)},
		),
	}

	actual, err := buildScriptOrderGraphs(rules)
	require.ErrorContains(
		t,
		err,
		"coder_script.a -> coder_script.prerequisite[1] -> coder_script.a",
	)
	require.NotContains(t, err.Error(), "coder_script.prerequisite[0]")
	require.Empty(t, actual)
}

func TestBuildScriptOrderGraphsRejectsCycleInLaterComponent(t *testing.T) {
	t.Parallel()

	rules := []resolvedScriptOrderRule{
		scriptOrderGraphTestRule(
			"data.coder_script_order.order", 0,
			"coder_agent.main", ScriptOrderPhaseStart,
			ScriptOrderRequirementSuccess,
			[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector(
				"run", "coder_script.acyclic_b", "coder_script.acyclic_b",
			)},
			[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector(
				"after", "coder_script.acyclic_a", "coder_script.acyclic_a",
			)},
		),
		scriptOrderGraphTestRule(
			"data.coder_script_order.order", 1,
			"coder_agent.main", ScriptOrderPhaseStart,
			ScriptOrderRequirementSuccess,
			[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector(
				"run", "coder_script.cycle_b", "coder_script.cycle_b",
			)},
			[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector(
				"after", "coder_script.cycle_a", "coder_script.cycle_a",
			)},
		),
		scriptOrderGraphTestRule(
			"data.coder_script_order.order", 2,
			"coder_agent.main", ScriptOrderPhaseStart,
			ScriptOrderRequirementSuccess,
			[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector(
				"run", "coder_script.cycle_a", "coder_script.cycle_a",
			)},
			[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector(
				"after", "coder_script.cycle_b", "coder_script.cycle_b",
			)},
		),
	}

	actual, err := buildScriptOrderGraphs(rules)
	require.ErrorContains(
		t,
		err,
		"coder_script.cycle_a -> coder_script.cycle_b -> coder_script.cycle_a",
	)
	require.Empty(t, actual)
}

func TestScriptOrderCycleErrorLimitsEdges(t *testing.T) {
	t.Parallel()

	cycleLength := maxScriptOrderCycleDiagnosticEdges + 2
	cycle := make([]scriptOrderCycleEdge, 0, cycleLength)
	for i := range cycleLength {
		dependentAddress := fmt.Sprintf("coder_script.script[%d]", i)
		prerequisiteAddress := fmt.Sprintf(
			"coder_script.script[%d]", (i+1)%cycleLength,
		)
		cycle = append(cycle, scriptOrderCycleEdge{
			dependentAddress:    dependentAddress,
			prerequisiteAddress: prerequisiteAddress,
			edge: scriptOrderEdge{
				dataSourceAddress: "data.coder_script_order.order",
				ruleIndex:         i,
				runSelector:       dependentAddress,
				afterSelector:     prerequisiteAddress,
			},
		})
	}

	err := scriptOrderCycleError(cycle)
	require.Error(t, err)
	require.Contains(t, err.Error(), "2 cycle edges omitted")
	require.Contains(
		t,
		err.Error(),
		" -> … -> coder_script.script[0]; cycle edges:",
	)
	require.Equal(
		t,
		maxScriptOrderCycleDiagnosticEdges,
		strings.Count(err.Error(), "(data source"),
	)
	require.NotContains(t, err.Error(), "\n")
	require.Less(t, len(err.Error()), 128*1024)
}

func TestScriptOrderCycleErrorTruncatesValues(t *testing.T) {
	t.Parallel()

	longScriptA := "coder_script." + strings.Repeat(
		"a", maxScriptOrderDiagnosticValueRunes+1,
	)
	longScriptB := "coder_script." + strings.Repeat(
		"b", maxScriptOrderDiagnosticValueRunes+1,
	)
	longDataSource := "data.coder_script_order." + strings.Repeat(
		"c", maxScriptOrderDiagnosticValueRunes+1,
	)
	cycle := []scriptOrderCycleEdge{
		{
			dependentAddress:    longScriptA,
			prerequisiteAddress: longScriptB,
			edge: scriptOrderEdge{
				dataSourceAddress: longDataSource,
				ruleIndex:         0,
				runSelector:       longScriptA,
				afterSelector:     longScriptB,
			},
		},
		{
			dependentAddress:    longScriptB,
			prerequisiteAddress: longScriptA,
			edge: scriptOrderEdge{
				dataSourceAddress: longDataSource,
				ruleIndex:         1,
				runSelector:       longScriptB,
				afterSelector:     longScriptA,
			},
		},
	}

	err := scriptOrderCycleError(cycle)
	require.Error(t, err)
	require.Contains(t, err.Error(), "…")
	require.NotContains(t, err.Error(), longScriptA)
	require.NotContains(t, err.Error(), longScriptB)
	require.NotContains(t, err.Error(), longDataSource)
	require.NotContains(t, err.Error(), "cycle edges omitted")
	require.NotContains(t, err.Error(), "\n")
}

func TestBuildScriptOrderGraphsDeterministic(t *testing.T) {
	t.Parallel()

	descending, err := buildScriptOrderGraphs([]resolvedScriptOrderRule{
		scriptOrderGraphTestRule(
			"data.coder_script_order.order", 0,
			"coder_agent.main", ScriptOrderPhaseStart,
			ScriptOrderRequirementCompletion,
			[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector(
				"run", "module.work",
				"module.work.coder_script.d",
				"module.work.coder_script.c",
			)},
			[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector(
				"after", "module.setup",
				"module.setup.coder_script.b",
				"module.setup.coder_script.a",
			)},
		),
	})
	require.NoError(t, err)
	ascending, err := buildScriptOrderGraphs([]resolvedScriptOrderRule{
		scriptOrderGraphTestRule(
			"data.coder_script_order.order", 0,
			"coder_agent.main", ScriptOrderPhaseStart,
			ScriptOrderRequirementCompletion,
			[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector(
				"run", "module.work",
				"module.work.coder_script.c",
				"module.work.coder_script.d",
			)},
			[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector(
				"after", "module.setup",
				"module.setup.coder_script.a",
				"module.setup.coder_script.b",
			)},
		),
	})
	require.NoError(t, err)
	require.Equal(t, descending, ascending)
}

func TestBuildScriptOrderGraphsRejectsCombinationsAboveLimit(t *testing.T) {
	t.Parallel()

	sideSize := 1
	for sideSize*sideSize <= maxScriptOrderCandidateDependencies {
		sideSize++
	}
	runAddresses := make([]string, 0, sideSize)
	afterAddresses := make([]string, 0, sideSize)
	for i := range sideSize {
		runAddresses = append(runAddresses, fmt.Sprintf("coder_script.run[%d]", i))
		afterAddresses = append(afterAddresses, fmt.Sprintf("coder_script.after[%d]", i))
	}

	rule := scriptOrderGraphTestRule(
		"data.coder_script_order.order", 0,
		"coder_agent.main", ScriptOrderPhaseStart,
		ScriptOrderRequirementSuccess,
		[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector(
			"run", "coder_script.run", runAddresses...,
		)},
		[]resolvedScriptOrderSelector{scriptOrderGraphTestSelector(
			"after", "coder_script.after", afterAddresses...,
		)},
	)

	order, err := buildScriptOrderGraphs([]resolvedScriptOrderRule{rule})
	require.ErrorContains(t, err, fmt.Sprintf(
		"limit of %d combinations",
		maxScriptOrderCandidateDependencies,
	))
	require.Empty(t, order)
}

func scriptOrderGraphTestSelector(
	field string,
	raw string,
	addresses ...string,
) resolvedScriptOrderSelector {
	selector, err := parseScriptOrderSelector(raw)
	if err != nil {
		panic(fmt.Sprintf("parse test script order selector %q: %s", raw, err))
	}
	return resolvedScriptOrderSelector{
		field:     field,
		raw:       raw,
		kind:      selector.kind,
		addresses: addresses,
	}
}

func scriptOrderGraphTestRule(
	dataSourceAddress string,
	ruleIndex int,
	runtimeAddress string,
	phase ScriptOrderPhase,
	requirement ScriptOrderRequirement,
	run []resolvedScriptOrderSelector,
	after []resolvedScriptOrderSelector,
) resolvedScriptOrderRule {
	return resolvedScriptOrderRule{
		dataSourceAddress: dataSourceAddress,
		ruleIndex:         ruleIndex,
		runtimeAddress:    runtimeAddress,
		phase:             phase,
		requirement:       requirement,
		run:               run,
		after:             after,
	}
}

func scriptOrderGraphTestDependency(
	dependentAddress string,
	prerequisiteAddress string,
	requirement ScriptOrderRequirement,
) ScriptOrderDependency {
	return ScriptOrderDependency{
		DependentAddress:    dependentAddress,
		PrerequisiteAddress: prerequisiteAddress,
		Requirement:         requirement,
	}
}

func scriptOrderGraphTestGraph(
	runtimeAddress string,
	phase ScriptOrderPhase,
	dependentAddress string,
	prerequisiteAddress string,
) ScriptOrderGraph {
	return ScriptOrderGraph{
		RuntimeAddress: runtimeAddress,
		Phase:          phase,
		Dependencies: []ScriptOrderDependency{scriptOrderGraphTestDependency(
			dependentAddress, prerequisiteAddress, ScriptOrderRequirementSuccess,
		)},
	}
}
