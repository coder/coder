package scriptorder

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildGraphs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		rules           []resolvedRule
		expected        Order
		errorSubstrings []string
	}{
		{
			name:     "NoRules",
			rules:    nil,
			expected: Order{},
		},
		{
			name: "EveryDependentRequiresEveryPrerequisite",
			rules: []resolvedRule{testRule(
				"data.coder_script_order.order", 0,
				"coder_agent.main", PhaseStart,
				RequirementCompletion,
				[]resolvedSelector{testSelector(
					"run", "coder_script.work",
					`coder_script.work["worker"]`,
					`coder_script.work["api"]`,
				)},
				[]resolvedSelector{testSelector(
					"after", "coder_script.setup",
					`coder_script.setup["repository"]`,
					`coder_script.setup["database"]`,
				)},
			)},
			expected: Order{Graphs: []Graph{{
				RuntimeAddress: "coder_agent.main",
				Phase:          PhaseStart,
				Dependencies: []Dependency{
					testDependency(`coder_script.work["api"]`, `coder_script.setup["database"]`, RequirementCompletion),
					testDependency(`coder_script.work["api"]`, `coder_script.setup["repository"]`, RequirementCompletion),
					testDependency(`coder_script.work["worker"]`, `coder_script.setup["database"]`, RequirementCompletion),
					testDependency(`coder_script.work["worker"]`, `coder_script.setup["repository"]`, RequirementCompletion),
				},
			}}},
		},
		{
			name: "RulesAndDataSourcesComposeAndDeduplicate",
			rules: []resolvedRule{
				testRule(
					"data.coder_script_order.first", 0,
					"coder_agent.main", PhaseStart,
					RequirementSuccess,
					[]resolvedSelector{
						testSelector("run", "coder_script.work[0]", "coder_script.work[0]"),
						testSelector("run", "coder_script.work", "coder_script.work[0]", "coder_script.work[1]"),
					},
					[]resolvedSelector{testSelector(
						"after", "coder_script.a", "coder_script.a",
					)},
				),
				testRule(
					"data.coder_script_order.second", 0,
					"coder_agent.main", PhaseStart,
					RequirementSuccess,
					[]resolvedSelector{testSelector(
						"run", "coder_script.work[0]", "coder_script.work[0]",
					)},
					[]resolvedSelector{testSelector(
						"after", "coder_script.a", "coder_script.a",
					)},
				),
			},
			expected: Order{Graphs: []Graph{{
				RuntimeAddress: "coder_agent.main",
				Phase:          PhaseStart,
				Dependencies: []Dependency{
					testDependency("coder_script.work[0]", "coder_script.a", RequirementSuccess),
					testDependency("coder_script.work[1]", "coder_script.a", RequirementSuccess),
				},
			}}},
		},
		{
			name: "DisconnectedComponentsRemainDisconnected",
			rules: []resolvedRule{
				testRule(
					"data.coder_script_order.order", 0,
					"coder_agent.main", PhaseStart,
					RequirementSuccess,
					[]resolvedSelector{testSelector("run", "coder_script.b", "coder_script.b")},
					[]resolvedSelector{testSelector("after", "coder_script.a", "coder_script.a")},
				),
				testRule(
					"data.coder_script_order.order", 1,
					"coder_agent.main", PhaseStart,
					RequirementSuccess,
					[]resolvedSelector{testSelector("run", "coder_script.d", "coder_script.d")},
					[]resolvedSelector{testSelector("after", "coder_script.c", "coder_script.c")},
				),
			},
			expected: Order{Graphs: []Graph{{
				RuntimeAddress: "coder_agent.main",
				Phase:          PhaseStart,
				Dependencies: []Dependency{
					testDependency("coder_script.b", "coder_script.a", RequirementSuccess),
					testDependency("coder_script.d", "coder_script.c", RequirementSuccess),
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
			rules: []resolvedRule{
				testRule(
					"data.coder_script_order.order", 0,
					"coder_agent.main", PhaseStart,
					RequirementSuccess,
					[]resolvedSelector{testSelector(
						"run", "coder_script.b", "coder_script.b",
					)},
					[]resolvedSelector{testSelector(
						"after", "coder_script.a", "coder_script.a",
					)},
				),
				testRule(
					"data.coder_script_order.order", 1,
					"coder_agent.main", PhaseStart,
					RequirementCompletion,
					[]resolvedSelector{testSelector(
						"run", "coder_script.c", "coder_script.c",
					)},
					[]resolvedSelector{testSelector(
						"after", "coder_script.b", "coder_script.b",
					)},
				),
				testRule(
					"data.coder_script_order.order", 2,
					"coder_agent.main", PhaseStart,
					RequirementSuccess,
					[]resolvedSelector{testSelector(
						"run", "coder_script.c", "coder_script.c",
					)},
					[]resolvedSelector{testSelector(
						"after", "coder_script.a", "coder_script.a",
					)},
				),
			},
			expected: Order{Graphs: []Graph{{
				RuntimeAddress: "coder_agent.main",
				Phase:          PhaseStart,
				Dependencies: []Dependency{
					testDependency("coder_script.b", "coder_script.a", RequirementSuccess),
					testDependency("coder_script.c", "coder_script.a", RequirementSuccess),
					testDependency("coder_script.c", "coder_script.b", RequirementCompletion),
				},
			}}},
		},
		// Nonzero indexes verify that diagnostics retain each rule's
		// original position within its data source after no-op rules
		// are removed.
		{
			name: "ConflictingRequirementsFail",
			rules: []resolvedRule{
				testRule(
					"data.coder_script_order.first", 2,
					"coder_agent.main", PhaseStart,
					RequirementSuccess,
					[]resolvedSelector{testSelector("run", "coder_script.b", "coder_script.b")},
					[]resolvedSelector{testSelector("after", "coder_script.a", "coder_script.a")},
				),
				testRule(
					"data.coder_script_order.second", 1,
					"coder_agent.main", PhaseStart,
					RequirementCompletion,
					[]resolvedSelector{testSelector("run", "coder_script.b", "coder_script.b")},
					[]resolvedSelector{testSelector("after", "coder_script.a", "coder_script.a")},
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

			actual, err := buildGraphs(test.rules)
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

func TestAddRuleEdgesEnforcesCombinationLimit(t *testing.T) {
	t.Parallel()

	t.Run("DeduplicatesBeforeCountingCombinations", func(t *testing.T) {
		t.Parallel()

		graph := &graphAccumulator{
			dependencies: map[string]map[string]dependencyEdge{},
		}
		rule := testRule(
			"data.coder_script_order.order", 0,
			"coder_agent.main", PhaseStart,
			RequirementSuccess,
			[]resolvedSelector{
				testSelector("run", "coder_script.work[0]", "coder_script.work[0]"),
				testSelector("run", "coder_script.work", "coder_script.work[0]"),
			},
			[]resolvedSelector{
				testSelector("after", "coder_script.setup[0]", "coder_script.setup[0]"),
				testSelector("after", "coder_script.setup", "coder_script.setup[0]"),
			},
		)

		budget := combinationBudget{limit: 1}
		err := addRuleEdges(graph, rule, &budget)
		require.NoError(t, err)
		require.Equal(t, 1, graph.dependencyCount)
		require.Equal(t, 1, budget.used)
		edge := graph.dependencies["coder_script.work[0]"]["coder_script.setup[0]"]
		require.Equal(t, "coder_script.work[0]", edge.runSelector)
		require.Equal(t, "coder_script.setup[0]", edge.afterSelector)
	})

	t.Run("RejectsOneRuleAboveLimit", func(t *testing.T) {
		t.Parallel()

		graph := &graphAccumulator{
			dependencies: map[string]map[string]dependencyEdge{},
		}
		rule := testRule(
			"data.coder_script_order.order", 2,
			"coder_agent.main", PhaseStart,
			RequirementSuccess,
			[]resolvedSelector{testSelector(
				"run", "module.work",
				"module.work.coder_script.c",
				"module.work.coder_script.d",
			)},
			[]resolvedSelector{testSelector(
				"after", "module.setup",
				"module.setup.coder_script.a",
				"module.setup.coder_script.b",
			)},
		)

		budget := combinationBudget{limit: 3}
		err := addRuleEdges(graph, rule, &budget)
		require.ErrorContains(t, err, `script order data source "data.coder_script_order.order" rule 2`)
		require.ErrorContains(t, err, "run selectors resolve to 2 scripts")
		require.ErrorContains(t, err, "after selectors resolve to 2 scripts")
		require.ErrorContains(t, err, "overall limit of 3 combinations across all script ordering rules")
		require.Zero(t, graph.dependencyCount)
		require.Zero(t, budget.used)
	})
}

func TestBuildGraphsEnforcesOverallCombinationLimit(t *testing.T) {
	t.Parallel()

	t.Run("AcrossGraphs", func(t *testing.T) {
		t.Parallel()

		rules := []resolvedRule{
			testRule(
				"data.coder_script_order.order", 0,
				"coder_agent.main", PhaseStop,
				RequirementSuccess,
				[]resolvedSelector{testSelector(
					"run", "coder_script.c", "coder_script.c",
				)},
				[]resolvedSelector{testSelector(
					"after", "module.setup",
					"module.setup.coder_script.a",
					"module.setup.coder_script.b",
				)},
			),
			testRule(
				"data.coder_script_order.order", 1,
				"coder_devcontainer.dev", PhaseStart,
				RequirementSuccess,
				[]resolvedSelector{testSelector(
					"run", "coder_script.d", "coder_script.d",
				)},
				[]resolvedSelector{testSelector(
					"after", "coder_script.a", "coder_script.a",
				)},
			),
		}

		order, err := buildGraphsWithCombinationLimit(rules, 2)
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

		rule := testRule(
			"data.coder_script_order.order", 0,
			"coder_agent.main", PhaseStart,
			RequirementSuccess,
			[]resolvedSelector{testSelector(
				"run", "coder_script.b", "coder_script.b",
			)},
			[]resolvedSelector{testSelector(
				"after", "coder_script.a", "coder_script.a",
			)},
		)
		rules := []resolvedRule{rule, rule, rule}
		rules[1].ruleIndex = 1
		rules[2].ruleIndex = 2

		order, err := buildGraphsWithCombinationLimit(rules, 2)
		require.ErrorContains(t, err, `rule 2`)
		require.ErrorContains(t, err, "limited to 2 run/after script combinations in total")
		require.ErrorContains(t, err, "2 from earlier rules together with 1 from this rule exceed the limit")
		require.Empty(t, order)
	})
}

func TestBuildGraphsBoundsConflictingRequirementDiagnostic(t *testing.T) {
	t.Parallel()

	longRunSelector := "coder_script." + strings.Repeat(
		"r", maxDiagnosticValueRunes+1,
	)
	longAfterSelector := "coder_script." + strings.Repeat(
		"a", maxDiagnosticValueRunes+1,
	)
	longFirstDataSource := "data.coder_script_order." + strings.Repeat(
		"f", maxDiagnosticValueRunes+1,
	)
	longSecondDataSource := "data.coder_script_order." + strings.Repeat(
		"s", maxDiagnosticValueRunes+1,
	)
	rules := []resolvedRule{
		testRule(
			longFirstDataSource, 0,
			"coder_agent.main", PhaseStart,
			RequirementSuccess,
			[]resolvedSelector{testSelector(
				"run", longRunSelector, longRunSelector,
			)},
			[]resolvedSelector{testSelector(
				"after", longAfterSelector, longAfterSelector,
			)},
		),
		testRule(
			longSecondDataSource, 0,
			"coder_agent.main", PhaseStart,
			RequirementCompletion,
			[]resolvedSelector{testSelector(
				"run", longRunSelector, longRunSelector,
			)},
			[]resolvedSelector{testSelector(
				"after", longAfterSelector, longAfterSelector,
			)},
		),
	}

	actual, err := buildGraphs(rules)
	require.Error(t, err)
	require.Empty(t, actual)
	require.Contains(t, err.Error(), "…")
	require.NotContains(t, err.Error(), longRunSelector)
	require.NotContains(t, err.Error(), longAfterSelector)
	require.NotContains(t, err.Error(), longFirstDataSource)
	require.NotContains(t, err.Error(), longSecondDataSource)
	require.Less(t, len(err.Error()), 4*1024)
}

func TestBuildGraphsSeparatesRuntimeAndPhase(t *testing.T) {
	t.Parallel()

	rules := []resolvedRule{
		testRule(
			"data.coder_script_order.order", 0,
			"coder_devcontainer.dev", PhaseStart,
			RequirementSuccess,
			[]resolvedSelector{testSelector("run", "coder_script.dc_b", "coder_script.dc_b")},
			[]resolvedSelector{testSelector("after", "coder_script.dc_a", "coder_script.dc_a")},
		),
		testRule(
			"data.coder_script_order.order", 1,
			"coder_agent.main", PhaseStop,
			RequirementSuccess,
			[]resolvedSelector{testSelector("run", "coder_script.stop_b", "coder_script.stop_b")},
			[]resolvedSelector{testSelector("after", "coder_script.stop_a", "coder_script.stop_a")},
		),
		testRule(
			"data.coder_script_order.order", 2,
			"coder_agent.main", PhaseStart,
			RequirementSuccess,
			[]resolvedSelector{testSelector("run", "coder_script.start_b", "coder_script.start_b")},
			[]resolvedSelector{testSelector("after", "coder_script.start_a", "coder_script.start_a")},
		),
	}

	actual, err := buildGraphs(rules)
	require.NoError(t, err)
	require.Equal(t, Order{Graphs: []Graph{
		testGraph(
			"coder_agent.main", PhaseStart,
			"coder_script.start_b", "coder_script.start_a",
		),
		testGraph(
			"coder_agent.main", PhaseStop,
			"coder_script.stop_b", "coder_script.stop_a",
		),
		testGraph(
			"coder_devcontainer.dev", PhaseStart,
			"coder_script.dc_b", "coder_script.dc_a",
		),
	}}, actual)
}

func TestBuildGraphsRejectsCycles(t *testing.T) {
	t.Parallel()

	rules := []resolvedRule{
		testRule(
			"data.coder_script_order.first", 0,
			"coder_agent.main", PhaseStart,
			RequirementSuccess,
			[]resolvedSelector{testSelector("run", "coder_script.b", "coder_script.b")},
			[]resolvedSelector{testSelector("after", "coder_script.a", "coder_script.a")},
		),
		testRule(
			"data.coder_script_order.first", 1,
			"coder_agent.main", PhaseStart,
			RequirementSuccess,
			[]resolvedSelector{testSelector("run", "coder_script.c", "coder_script.c")},
			[]resolvedSelector{testSelector("after", "coder_script.b", "coder_script.b")},
		),
		testRule(
			"data.coder_script_order.second", 0,
			"coder_agent.main", PhaseStart,
			RequirementSuccess,
			[]resolvedSelector{testSelector("run", "coder_script.a", "coder_script.a")},
			[]resolvedSelector{testSelector("after", "coder_script.c", "coder_script.c")},
		),
	}

	actual, err := buildGraphs(rules)
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

func TestBuildGraphsSelectsCycleDeterministically(t *testing.T) {
	t.Parallel()

	// Supply the lexically later X-Y cycle first, then give A two
	// cycle-forming prerequisites in reverse lexical order.
	// Diagnostics must still select the lexically first root and
	// prerequisite.
	rules := []resolvedRule{
		testRule(
			"data.coder_script_order.order", 0,
			"coder_agent.main", PhaseStart,
			RequirementSuccess,
			[]resolvedSelector{testSelector("run", "coder_script.x", "coder_script.x")},
			[]resolvedSelector{testSelector("after", "coder_script.y", "coder_script.y")},
		),
		testRule(
			"data.coder_script_order.order", 1,
			"coder_agent.main", PhaseStart,
			RequirementSuccess,
			[]resolvedSelector{testSelector("run", "coder_script.y", "coder_script.y")},
			[]resolvedSelector{testSelector("after", "coder_script.x", "coder_script.x")},
		),
		testRule(
			"data.coder_script_order.order", 2,
			"coder_agent.main", PhaseStart,
			RequirementSuccess,
			[]resolvedSelector{testSelector("run", "coder_script.a", "coder_script.a")},
			[]resolvedSelector{
				testSelector("after", "coder_script.c", "coder_script.c"),
				testSelector("after", "coder_script.b", "coder_script.b"),
			},
		),
		testRule(
			"data.coder_script_order.order", 3,
			"coder_agent.main", PhaseStart,
			RequirementSuccess,
			[]resolvedSelector{testSelector("run", "coder_script.c", "coder_script.c")},
			[]resolvedSelector{testSelector("after", "coder_script.a", "coder_script.a")},
		),
		testRule(
			"data.coder_script_order.order", 4,
			"coder_agent.main", PhaseStart,
			RequirementSuccess,
			[]resolvedSelector{testSelector("run", "coder_script.b", "coder_script.b")},
			[]resolvedSelector{testSelector("after", "coder_script.a", "coder_script.a")},
		),
	}

	actual, err := buildGraphs(rules)
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

func TestBuildGraphsRejectsCycleAfterAcyclicPrerequisite(t *testing.T) {
	t.Parallel()

	// A depends on instances 0 and 1 of coder_script.prerequisite.
	// Instance 0 is acyclic, while instance 1 depends on A and forms
	// the reported cycle.
	rules := []resolvedRule{
		testRule(
			"data.coder_script_order.order", 0,
			"coder_agent.main", PhaseStart,
			RequirementSuccess,
			[]resolvedSelector{testSelector(
				"run", "coder_script.a", "coder_script.a",
			)},
			[]resolvedSelector{testSelector(
				"after", "coder_script.prerequisite",
				"coder_script.prerequisite[0]",
				"coder_script.prerequisite[1]",
			)},
		),
		testRule(
			"data.coder_script_order.order", 1,
			"coder_agent.main", PhaseStart,
			RequirementSuccess,
			[]resolvedSelector{testSelector(
				"run", "coder_script.prerequisite[1]",
				"coder_script.prerequisite[1]",
			)},
			[]resolvedSelector{testSelector(
				"after", "coder_script.a", "coder_script.a",
			)},
		),
	}

	actual, err := buildGraphs(rules)
	require.ErrorContains(
		t,
		err,
		"coder_script.a -> coder_script.prerequisite[1] -> coder_script.a",
	)
	require.NotContains(t, err.Error(), "coder_script.prerequisite[0]")
	require.Empty(t, actual)
}

func TestBuildGraphsRejectsCycleInLaterComponent(t *testing.T) {
	t.Parallel()

	rules := []resolvedRule{
		testRule(
			"data.coder_script_order.order", 0,
			"coder_agent.main", PhaseStart,
			RequirementSuccess,
			[]resolvedSelector{testSelector(
				"run", "coder_script.acyclic_b", "coder_script.acyclic_b",
			)},
			[]resolvedSelector{testSelector(
				"after", "coder_script.acyclic_a", "coder_script.acyclic_a",
			)},
		),
		testRule(
			"data.coder_script_order.order", 1,
			"coder_agent.main", PhaseStart,
			RequirementSuccess,
			[]resolvedSelector{testSelector(
				"run", "coder_script.cycle_b", "coder_script.cycle_b",
			)},
			[]resolvedSelector{testSelector(
				"after", "coder_script.cycle_a", "coder_script.cycle_a",
			)},
		),
		testRule(
			"data.coder_script_order.order", 2,
			"coder_agent.main", PhaseStart,
			RequirementSuccess,
			[]resolvedSelector{testSelector(
				"run", "coder_script.cycle_a", "coder_script.cycle_a",
			)},
			[]resolvedSelector{testSelector(
				"after", "coder_script.cycle_b", "coder_script.cycle_b",
			)},
		),
	}

	actual, err := buildGraphs(rules)
	require.ErrorContains(
		t,
		err,
		"coder_script.cycle_a -> coder_script.cycle_b -> coder_script.cycle_a",
	)
	require.Empty(t, actual)
}

func TestCycleErrorLimitsEdges(t *testing.T) {
	t.Parallel()

	cycleLength := maxCycleDiagnosticEdges + 2
	cycle := make([]cycleEdge, 0, cycleLength)
	for i := range cycleLength {
		dependentAddress := fmt.Sprintf("coder_script.script[%d]", i)
		prerequisiteAddress := fmt.Sprintf(
			"coder_script.script[%d]", (i+1)%cycleLength,
		)
		cycle = append(cycle, cycleEdge{
			dependentAddress:    dependentAddress,
			prerequisiteAddress: prerequisiteAddress,
			edge: dependencyEdge{
				dataSourceAddress: "data.coder_script_order.order",
				ruleIndex:         i,
				runSelector:       dependentAddress,
				afterSelector:     prerequisiteAddress,
			},
		})
	}

	err := cycleError(cycle)
	require.Error(t, err)
	require.Contains(t, err.Error(), "2 cycle edges omitted")
	require.Contains(
		t,
		err.Error(),
		" -> … -> coder_script.script[0]; cycle edges:",
	)
	require.Equal(
		t,
		maxCycleDiagnosticEdges,
		strings.Count(err.Error(), "(data source"),
	)
	require.NotContains(t, err.Error(), "\n")
	require.Less(t, len(err.Error()), 128*1024)
}

func TestCycleErrorTruncatesValues(t *testing.T) {
	t.Parallel()

	longScriptA := "coder_script." + strings.Repeat(
		"a", maxDiagnosticValueRunes+1,
	)
	longScriptB := "coder_script." + strings.Repeat(
		"b", maxDiagnosticValueRunes+1,
	)
	longDataSource := "data.coder_script_order." + strings.Repeat(
		"c", maxDiagnosticValueRunes+1,
	)
	cycle := []cycleEdge{
		{
			dependentAddress:    longScriptA,
			prerequisiteAddress: longScriptB,
			edge: dependencyEdge{
				dataSourceAddress: longDataSource,
				ruleIndex:         0,
				runSelector:       longScriptA,
				afterSelector:     longScriptB,
			},
		},
		{
			dependentAddress:    longScriptB,
			prerequisiteAddress: longScriptA,
			edge: dependencyEdge{
				dataSourceAddress: longDataSource,
				ruleIndex:         1,
				runSelector:       longScriptB,
				afterSelector:     longScriptA,
			},
		},
	}

	err := cycleError(cycle)
	require.Error(t, err)
	require.Contains(t, err.Error(), "…")
	require.NotContains(t, err.Error(), longScriptA)
	require.NotContains(t, err.Error(), longScriptB)
	require.NotContains(t, err.Error(), longDataSource)
	require.NotContains(t, err.Error(), "cycle edges omitted")
	require.NotContains(t, err.Error(), "\n")
}

func TestBuildGraphsDeterministic(t *testing.T) {
	t.Parallel()

	descending, err := buildGraphs([]resolvedRule{
		testRule(
			"data.coder_script_order.order", 0,
			"coder_agent.main", PhaseStart,
			RequirementCompletion,
			[]resolvedSelector{testSelector(
				"run", "module.work",
				"module.work.coder_script.d",
				"module.work.coder_script.c",
			)},
			[]resolvedSelector{testSelector(
				"after", "module.setup",
				"module.setup.coder_script.b",
				"module.setup.coder_script.a",
			)},
		),
	})
	require.NoError(t, err)
	ascending, err := buildGraphs([]resolvedRule{
		testRule(
			"data.coder_script_order.order", 0,
			"coder_agent.main", PhaseStart,
			RequirementCompletion,
			[]resolvedSelector{testSelector(
				"run", "module.work",
				"module.work.coder_script.c",
				"module.work.coder_script.d",
			)},
			[]resolvedSelector{testSelector(
				"after", "module.setup",
				"module.setup.coder_script.a",
				"module.setup.coder_script.b",
			)},
		),
	})
	require.NoError(t, err)
	require.Equal(t, descending, ascending)
}

func TestBuildGraphsRejectsCombinationsAboveLimit(t *testing.T) {
	t.Parallel()

	sideSize := 1
	for sideSize*sideSize <= maxCandidateDependencies {
		sideSize++
	}
	runAddresses := make([]string, 0, sideSize)
	afterAddresses := make([]string, 0, sideSize)
	for i := range sideSize {
		runAddresses = append(runAddresses, fmt.Sprintf("coder_script.run[%d]", i))
		afterAddresses = append(afterAddresses, fmt.Sprintf("coder_script.after[%d]", i))
	}

	rule := testRule(
		"data.coder_script_order.order", 0,
		"coder_agent.main", PhaseStart,
		RequirementSuccess,
		[]resolvedSelector{testSelector(
			"run", "coder_script.run", runAddresses...,
		)},
		[]resolvedSelector{testSelector(
			"after", "coder_script.after", afterAddresses...,
		)},
	)

	order, err := buildGraphs([]resolvedRule{rule})
	require.ErrorContains(t, err, fmt.Sprintf(
		"limit of %d combinations",
		maxCandidateDependencies,
	))
	require.Empty(t, order)
}

func testSelector(
	field string,
	raw string,
	addresses ...string,
) resolvedSelector {
	selector, err := parseSelector(raw)
	if err != nil {
		panic(fmt.Sprintf("parse test script order selector %q: %s", raw, err))
	}
	return resolvedSelector{
		field:     field,
		raw:       raw,
		kind:      selector.kind,
		addresses: addresses,
	}
}

func testRule(
	dataSourceAddress string,
	ruleIndex int,
	runtimeAddress string,
	phase Phase,
	requirement Requirement,
	run []resolvedSelector,
	after []resolvedSelector,
) resolvedRule {
	return resolvedRule{
		dataSourceAddress: dataSourceAddress,
		ruleIndex:         ruleIndex,
		runtimeAddress:    runtimeAddress,
		phase:             phase,
		requirement:       requirement,
		run:               run,
		after:             after,
	}
}

func testDependency(
	dependentAddress string,
	prerequisiteAddress string,
	requirement Requirement,
) Dependency {
	return Dependency{
		DependentAddress:    dependentAddress,
		PrerequisiteAddress: prerequisiteAddress,
		Requirement:         requirement,
	}
}

func testGraph(
	runtimeAddress string,
	phase Phase,
	dependentAddress string,
	prerequisiteAddress string,
) Graph {
	return Graph{
		RuntimeAddress: runtimeAddress,
		Phase:          phase,
		Dependencies: []Dependency{testDependency(
			dependentAddress, prerequisiteAddress, RequirementSuccess,
		)},
	}
}
