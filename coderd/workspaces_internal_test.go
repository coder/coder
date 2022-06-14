package coderd

import (
	"fmt"
	"strings"
	"testing"

	"github.com/coder/coder/coderd/database"

	"github.com/stretchr/testify/require"
)

func TestSearchWorkspace(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		Name                  string
		Query                 string
		Expected              database.GetWorkspacesWithFilterParams
		ExpectedErrorContains string
	}{
		{
			Name:     "Empty",
			Query:    "",
			Expected: database.GetWorkspacesWithFilterParams{},
		},
		{
			Name:  "Owner/Name",
			Query: "Foo/Bar",
			Expected: database.GetWorkspacesWithFilterParams{
				OwnerUsername: "Foo",
				Name:          "Bar",
			},
		},
		{
			Name:  "Name",
			Query: "workspace-name",
			Expected: database.GetWorkspacesWithFilterParams{
				Name: "workspace-name",
			},
		},
		{
			Name:  "Name+Param",
			Query: "workspace-name template:docker",
			Expected: database.GetWorkspacesWithFilterParams{
				Name:         "workspace-name",
				TemplateName: "docker",
			},
		},
		{
			Name:  "OnlyParams",
			Query: "name:workspace-name template:docker owner:alice",
			Expected: database.GetWorkspacesWithFilterParams{
				Name:          "workspace-name",
				TemplateName:  "docker",
				OwnerUsername: "alice",
			},
		},
		{
			Name:  "QuotedParam",
			Query: `name:workspace-name template:"docker template" owner:alice`,
			Expected: database.GetWorkspacesWithFilterParams{
				Name:          "workspace-name",
				TemplateName:  "docker template",
				OwnerUsername: "alice",
			},
		},
		{
			Name:  "QuotedKey",
			Query: `"name":baz "template":foo "owner":bar`,
			Expected: database.GetWorkspacesWithFilterParams{
				Name:          "baz",
				TemplateName:  "foo",
				OwnerUsername: "bar",
			},
		},
		{
			// This will not return an error
			Name:     "ExtraKeys",
			Query:    `foo:bar`,
			Expected: database.GetWorkspacesWithFilterParams{},
		},
		{
			// Quotes keep elements together
			Name:  "QuotedSpecial",
			Query: `name:"workspace:name"`,
			Expected: database.GetWorkspacesWithFilterParams{
				Name: "workspace:name",
			},
		},
		{
			Name:  "QuotedMadness",
			Query: `"name":"foo:bar:baz/baz/zoo:zonk"`,
			Expected: database.GetWorkspacesWithFilterParams{
				Name: "foo:bar:baz/baz/zoo:zonk",
			},
		},
		{
			Name:  "QuotedName",
			Query: `"foo/bar"`,
			Expected: database.GetWorkspacesWithFilterParams{
				Name: "foo/bar",
			},
		},
		{
			Name:  "QuotedOwner/Name",
			Query: `"foo"/"bar"`,
			Expected: database.GetWorkspacesWithFilterParams{
				Name:          "bar",
				OwnerUsername: "foo",
			},
		},

		// Failures
		{
			Name:                  "ExtraSlashes",
			Query:                 `foo/bar/baz`,
			ExpectedErrorContains: "can only contain 1 '/'",
		},
		{
			Name:                  "ExtraColon",
			Query:                 `owner:name:extra`,
			ExpectedErrorContains: "can only contain 1 ':'",
		},
	}

	for _, c := range testCases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			values, errs := workspaceSearchQuery(c.Query)
			if c.ExpectedErrorContains != "" {
				require.True(t, len(errs) > 0, "expect some errors")
				var s strings.Builder
				for _, err := range errs {
					_, _ = s.WriteString(fmt.Sprintf("%s: %s\n", err.Field, err.Detail))
				}
				require.Contains(t, s.String(), c.ExpectedErrorContains)
			} else {
				require.Len(t, errs, 0, "expected no error")
				require.Equal(t, c.Expected, values, "expected values")
			}
		})
	}
}
