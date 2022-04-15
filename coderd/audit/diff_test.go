package audit_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/audit"
	"github.com/coder/coder/coderd/database"
)

func TestDiff(t *testing.T) {
	t.Parallel()

	t.Run("Normal", func(t *testing.T) {
		t.Parallel()

		runDiffTests(t, []diffTest[database.User]{
			{
				name: "LeftEmpty",
				left: audit.Empty[database.User](), right: database.User{Name: "colin", Email: "colin@coder.com"},
				exp: audit.DiffMap{
					"name":  "colin",
					"email": "colin@coder.com",
				},
			},
			{
				name: "RightEmpty",
				left: database.User{Name: "colin", Email: "colin@coder.com"}, right: audit.Empty[database.User](),
				exp: audit.DiffMap{
					"name":  "",
					"email": "",
				},
			},
			{
				name: "NoChange",
				left: audit.Empty[database.User](), right: audit.Empty[database.User](),
				exp: audit.DiffMap{},
			},
		})
	})
}

type diffTest[T audit.Auditable] struct {
	name        string
	left, right T
	exp         audit.DiffMap
}

func runDiffTests[T audit.Auditable](t *testing.T, tests []diffTest[T]) {
	t.Helper()

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t,
				test.exp,
				audit.Diff(test.left, test.right),
			)
		})
	}
}
