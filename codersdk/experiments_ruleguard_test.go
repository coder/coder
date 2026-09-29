package codersdk_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

// TestExperimentsUserScopedRuleguard keeps the user-scoped experiment
// names in the userScopedExperimentStaticCheck ruleguard rule equal to
// codersdk.ExperimentsUserScoped, so the rule cannot silently skip a newly
// user-scoped experiment.
func TestExperimentsUserScopedRuleguard(t *testing.T) {
	t.Parallel()

	// Map experiment values to their constant names from the source, so a
	// user-scoped entry without a named constant fails.
	file, err := parser.ParseFile(token.NewFileSet(), "deployment.go", nil, 0)
	require.NoError(t, err)
	constNames := map[codersdk.Experiment]string{}
	ast.Inspect(file, func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if !ok {
			return true
		}
		typ, ok := spec.Type.(*ast.Ident)
		if !ok || typ.Name != "Experiment" {
			return true
		}
		for i, name := range spec.Names {
			lit, ok := spec.Values[i].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				continue
			}
			value, err := strconv.Unquote(lit.Value)
			require.NoError(t, err)
			constNames[codersdk.Experiment(value)] = name.Name
		}
		return true
	})
	want := make([]string, 0, len(codersdk.ExperimentsUserScoped))
	for _, ex := range codersdk.ExperimentsUserScoped {
		name, ok := constNames[ex]
		require.Truef(t, ok, "user-scoped experiment %q has no Experiment constant in deployment.go", ex)
		want = append(want, name)
	}
	slices.Sort(want)

	rules, err := os.ReadFile("../scripts/rules.go")
	require.NoError(t, err)
	src := string(rules)
	start := strings.Index(src, "func userScopedExperimentStaticCheck(")
	require.NotEqual(t, -1, start, "userScopedExperimentStaticCheck rule not found in scripts/rules.go")
	end := strings.Index(src[start:], "\n}\n")
	require.NotEqual(t, -1, end)
	body := src[start : start+end]

	// Every experiment alternation in the rule must list exactly the
	// user-scoped experiments.
	matches := regexp.MustCompile(`codersdk\\\.\(([A-Za-z0-9|]+)\)`).FindAllStringSubmatch(body, -1)
	require.Len(t, matches, 2, "expected one experiment list per rule pattern")
	for _, match := range matches {
		got := strings.Split(match[1], "|")
		slices.Sort(got)
		require.Equal(t, want, got, "update userScopedExperimentStaticCheck in scripts/rules.go to match codersdk.ExperimentsUserScoped")
	}
}
