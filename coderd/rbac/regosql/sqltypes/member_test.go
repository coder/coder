package sqltypes_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/rbac/regosql/sqltypes"
)

func TestMembership(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		Name           string
		Membership     sqltypes.Node
		ExpectedSQL    string
		ExpectedErrors int
	}{
		{
			Name: "StringArray",
			Membership: sqltypes.MemberOf(
				sqltypes.String("foo"),
				must(sqltypes.Array("",
					sqltypes.String("bar"),
					sqltypes.String("buzz"),
				)),
			),
			ExpectedSQL: "'foo' = ANY(ARRAY ['bar','buzz'])",
		},
		{
			Name: "NumberArray",
			Membership: sqltypes.MemberOf(
				sqltypes.Number("", "5"),
				must(sqltypes.Array("",
					sqltypes.Number("", "2"),
					sqltypes.Number("", "5"),
				)),
			),
			ExpectedSQL: "5 = ANY(ARRAY [2,5])",
		},
		{
			Name: "BoolArray",
			Membership: sqltypes.MemberOf(
				sqltypes.Bool(true),
				must(sqltypes.Array("",
					sqltypes.Bool(false),
					sqltypes.Bool(true),
				)),
			),
			ExpectedSQL: "true = ANY(ARRAY [false,true])",
		},
		{
			Name: "EmptyArray",
			Membership: sqltypes.MemberOf(
				sqltypes.Bool(true),
				must(sqltypes.Array("")),
			),
			ExpectedSQL: "false",
		},
		{
			Name: "AlwaysFalseMember",
			Membership: sqltypes.MemberOf(
				sqltypes.AlwaysFalseNode(sqltypes.Bool(true)),
				must(sqltypes.Array("",
					sqltypes.Bool(false),
					sqltypes.Bool(true),
				)),
			),
			ExpectedSQL: "false",
		},
		{
			Name: "AlwaysFalseArray",
			Membership: sqltypes.MemberOf(
				sqltypes.Bool(true),
				sqltypes.AlwaysFalseNode(must(sqltypes.Array("",
					sqltypes.Bool(false),
					sqltypes.Bool(true),
				))),
			),
			ExpectedSQL: "false",
		},

		// Errors
		{
			Name: "Unsupported",
			Membership: sqltypes.MemberOf(
				sqltypes.Bool(true),
				sqltypes.Bool(true),
			),
			ExpectedErrors: 1,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()

			gen := sqltypes.NewSQLGenerator()
			found := tc.Membership.SQLString(gen)
			if tc.ExpectedErrors > 0 {
				require.Equal(t, tc.ExpectedErrors, len(gen.Errors()), "expected some errors")
			} else {
				require.Equal(t, tc.ExpectedSQL, found, "expected sql")
				require.Equal(t, tc.ExpectedErrors, 0, "expected no errors")
			}
		})
	}
}

func must[V any](v V, err error) V {
	if err != nil {
		panic(err)
	}
	return v
}
