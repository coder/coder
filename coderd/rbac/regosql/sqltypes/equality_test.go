package sqltypes_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/rbac/regosql/sqltypes"
)

func TestEquality(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		Name           string
		Equality       sqltypes.Node
		ExpectedSQL    string
		ExpectedErrors int
	}{
		{
			Name: "String=String",
			Equality: sqltypes.Equality(false,
				sqltypes.String("foo"),
				sqltypes.String("bar"),
			),
			ExpectedSQL: "'foo' = 'bar'",
		},
		{
			Name: "Number=Number",
			Equality: sqltypes.Equality(false,
				sqltypes.Number("", json.Number("5")),
				sqltypes.Number("", json.Number("22")),
			),
			ExpectedSQL: "5 = 22",
		},
		{
			Name: "Bool=Bool",
			Equality: sqltypes.Equality(false,
				sqltypes.Bool(true),
				sqltypes.Bool(false),
			),
			ExpectedSQL: "true = false",
		},
		{
			Name: "Bool=Equality",
			Equality: sqltypes.Equality(false,
				sqltypes.Bool(true),
				sqltypes.Equality(true,
					sqltypes.Equality(true,
						sqltypes.String("foo"),
						sqltypes.String("bar"),
					),
					sqltypes.Bool(false),
				),
			),
			ExpectedSQL: "true = (('foo' != 'bar') != false)",
		},
		{
			Name: "Equality=Equality",
			Equality: sqltypes.Equality(false,
				sqltypes.Equality(true,
					sqltypes.Bool(true),
					sqltypes.Bool(false),
				),
				sqltypes.Equality(false,
					sqltypes.String("foo"),
					sqltypes.String("foo"),
				),
			),
			ExpectedSQL: "(true != false) = ('foo' = 'foo')",
		},
		{
			Name: "Membership=Membership",
			Equality: sqltypes.Equality(false,
				sqltypes.Equality(true,
					sqltypes.MemberOf(
						sqltypes.String("foo"),
						must(sqltypes.Array("",
							sqltypes.String("foo"),
							sqltypes.String("bar"),
						)),
					),
					sqltypes.Bool(false),
				),
				sqltypes.Equality(false,
					sqltypes.Bool(true),
					sqltypes.MemberOf(
						sqltypes.Number("", "2"),
						must(sqltypes.Array("",
							sqltypes.Number("", "5"),
							sqltypes.Number("", "2"),
						)),
					),
				),
			),
			ExpectedSQL: "(('foo' = ANY(ARRAY ['foo','bar'])) != false) = (true = (2 = ANY(ARRAY [5,2])))",
		},
		{
			Name: "AlwaysFalse=String",
			Equality: sqltypes.Equality(false,
				sqltypes.AlwaysFalseNode(sqltypes.String("foo")),
				sqltypes.String("foo"),
			),
			ExpectedSQL: "false",
		},
		{
			Name: "String=AlwaysFalse",
			Equality: sqltypes.Equality(false,
				sqltypes.String("foo"),
				sqltypes.AlwaysFalseNode(sqltypes.String("foo")),
			),
			ExpectedSQL: "false",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()

			gen := sqltypes.NewSQLGenerator()
			found := tc.Equality.SQLString(gen)
			if tc.ExpectedErrors > 0 {
				require.Equal(t, tc.ExpectedErrors, len(gen.Errors()), "expected AstNumber of errors")
			} else {
				require.Equal(t, tc.ExpectedSQL, found, "expected sql")
			}
		})
	}
}
