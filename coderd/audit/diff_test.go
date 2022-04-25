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
				left: audit.Empty[database.User](), right: database.User{Username: "colin", Email: "colin@coder.com"},
				exp: audit.Map{
					"email": "colin@coder.com",
				},
			},
			{
				name: "RightEmpty",
				left: database.User{Username: "colin", Email: "colin@coder.com"}, right: audit.Empty[database.User](),
				exp: audit.Map{
					"email": "",
				},
			},
			{
				name: "NoChange",
				left: audit.Empty[database.User](), right: audit.Empty[database.User](),
				exp: audit.Map{},
			},
		})
	})
}

type diffTest[T audit.Auditable] struct {
	name        string
	left, right T
	exp         audit.Map
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
