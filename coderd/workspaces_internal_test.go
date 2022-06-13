package coderd

import (
	"fmt"
	"strings"
	"testing"

	"github.com/coder/coder/coderd/httpapi"

	"github.com/stretchr/testify/require"
)

func TestSearchWorkspace(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		Name                  string
		Query                 string
		Expected              map[string]string
		ExpectedErrorContains string
	}{
		{
			Name:     "Empty",
			Query:    "",
			Expected: map[string]string{},
		},
		{
			Name:  "Owner/Name",
			Query: "Foo/Bar",
			Expected: map[string]string{
				"owner": "Foo",
				"name":  "Bar",
			},
		},
		{
			Name:  "Name",
			Query: "workspace-name",
			Expected: map[string]string{
				"name": "workspace-name",
			},
		},
		{
			Name:  "Name+Param",
			Query: "workspace-name template:docker",
			Expected: map[string]string{
				"name":     "workspace-name",
				"template": "docker",
			},
		},
		{
			Name:  "OnlyParams",
			Query: "name:workspace-name template:docker owner:alice",
			Expected: map[string]string{
				"owner":    "alice",
				"name":     "workspace-name",
				"template": "docker",
			},
		},
		{
			Name:  "QuotedParam",
			Query: `name:workspace-name template:"docker template" owner:alice`,
			Expected: map[string]string{
				"owner":    "alice",
				"name":     "workspace-name",
				"template": "docker template",
			},
		},
		{
			Name:  "QuotedKey",
			Query: `"spaced key":"spaced value"`,
			Expected: map[string]string{
				"spaced key": "spaced value",
			},
		},
		{
			// This will not return an error
			Name:  "ExtraKeys",
			Query: `foo:bar`,
			Expected: map[string]string{
				"foo": "bar",
			},
		},
		{
			// Quotes keep elements together
			Name:  "QuotedSpecial",
			Query: `name:"workspace:name"`,
			Expected: map[string]string{
				"name": "workspace:name",
			},
		},
		{
			Name:  "QuotedMadness",
			Query: `"key:is:wild/a/b/c":"foo:bar/baz/zoo:zonk"`,
			Expected: map[string]string{
				"key:is:wild/a/b/c": "foo:bar/baz/zoo:zonk",
			},
		},
		{
			Name:  "QuotesName",
			Query: `"foo/bar"`,
			Expected: map[string]string{
				"name": "foo/bar",
			},
		},
		{
			Name:  "QuotedOwner/Name",
			Query: `"foo"/"bar"`,
			Expected: map[string]string{
				"owner": "foo",
				"name":  "bar",
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
				require.Equal(t, []httpapi.Error{}, errs, "expected no error")
				require.Equal(t, c.Expected, values, "expected values")
			}
		})
	}
}
