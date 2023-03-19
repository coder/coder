package workspaceapps_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/workspaceapps"
)

func Test_RequestValidate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		req         workspaceapps.Request
		errContains string
	}{
		{
			name: "OK1",
			req: workspaceapps.Request{
				AccessMethod:      workspaceapps.AccessMethodPath,
				BasePath:          "/",
				UsernameOrID:      "foo",
				WorkspaceNameOrID: "bar",
				AgentNameOrID:     "baz",
				AppSlugOrPort:     "qux",
			},
		},
		{
			name: "OK2",
			req: workspaceapps.Request{
				AccessMethod:      workspaceapps.AccessMethodSubdomain,
				BasePath:          "/",
				UsernameOrID:      "foo",
				WorkspaceAndAgent: "bar.baz",
				AppSlugOrPort:     "qux",
			},
		},
		{
			name: "OK3",
			req: workspaceapps.Request{
				AccessMethod:      workspaceapps.AccessMethodPath,
				BasePath:          "/",
				UsernameOrID:      "foo",
				WorkspaceNameOrID: "bar",
				AppSlugOrPort:     "baz",
			},
		},
		{
			name: "NoAccessMethod",
			req: workspaceapps.Request{
				AccessMethod:      "",
				BasePath:          "/",
				UsernameOrID:      "foo",
				WorkspaceNameOrID: "bar",
				AgentNameOrID:     "baz",
				AppSlugOrPort:     "qux",
			},
			errContains: "invalid access method",
		},
		{
			name: "UnknownAccessMethod",
			req: workspaceapps.Request{
				AccessMethod:      "dean was here",
				BasePath:          "/",
				UsernameOrID:      "foo",
				WorkspaceNameOrID: "bar",
				AgentNameOrID:     "baz",
				AppSlugOrPort:     "qux",
			},
			errContains: "invalid access method",
		},
		{
			name: "NoBasePath",
			req: workspaceapps.Request{
				AccessMethod:      workspaceapps.AccessMethodPath,
				BasePath:          "",
				UsernameOrID:      "foo",
				WorkspaceNameOrID: "bar",
				AgentNameOrID:     "baz",
				AppSlugOrPort:     "qux",
			},
			errContains: "base path is required",
		},
		{
			name: "NoUsernameOrID",
			req: workspaceapps.Request{
				AccessMethod:      workspaceapps.AccessMethodPath,
				BasePath:          "/",
				UsernameOrID:      "",
				WorkspaceNameOrID: "bar",
				AgentNameOrID:     "baz",
				AppSlugOrPort:     "qux",
			},
			errContains: "username or ID is required",
		},
		{
			name: "NoMe",
			req: workspaceapps.Request{
				AccessMethod:      workspaceapps.AccessMethodPath,
				BasePath:          "/",
				UsernameOrID:      "me",
				WorkspaceNameOrID: "bar",
				AgentNameOrID:     "baz",
				AppSlugOrPort:     "qux",
			},
			errContains: `username cannot be "me"`,
		},
		{
			name: "InvalidWorkspaceAndAgent/Empty1",
			req: workspaceapps.Request{
				AccessMethod:      workspaceapps.AccessMethodPath,
				BasePath:          "/",
				UsernameOrID:      "foo",
				WorkspaceAndAgent: ".bar",
				AppSlugOrPort:     "baz",
			},
			errContains: "invalid workspace and agent",
		},
		{
			name: "InvalidWorkspaceAndAgent/Empty2",
			req: workspaceapps.Request{
				AccessMethod:      workspaceapps.AccessMethodPath,
				BasePath:          "/",
				UsernameOrID:      "foo",
				WorkspaceAndAgent: "bar.",
				AppSlugOrPort:     "baz",
			},
			errContains: "invalid workspace and agent",
		},
		{
			name: "InvalidWorkspaceAndAgent/TwoDots",
			req: workspaceapps.Request{
				AccessMethod:      workspaceapps.AccessMethodPath,
				BasePath:          "/",
				UsernameOrID:      "foo",
				WorkspaceAndAgent: "bar.baz.qux",
				AppSlugOrPort:     "baz",
			},
			errContains: "invalid workspace and agent",
		},
		{
			name: "AmbiguousWorkspaceAndAgent/1",
			req: workspaceapps.Request{
				AccessMethod:      workspaceapps.AccessMethodPath,
				BasePath:          "/",
				UsernameOrID:      "foo",
				WorkspaceAndAgent: "bar.baz",
				WorkspaceNameOrID: "bar",
				AppSlugOrPort:     "qux",
			},
			errContains: "cannot specify both",
		},
		{
			name: "AmbiguousWorkspaceAndAgent/2",
			req: workspaceapps.Request{
				AccessMethod:      workspaceapps.AccessMethodPath,
				BasePath:          "/",
				UsernameOrID:      "foo",
				WorkspaceAndAgent: "bar.baz",
				AgentNameOrID:     "baz",
				AppSlugOrPort:     "qux",
			},
			errContains: "cannot specify both",
		},
		{
			name: "NoWorkspaceNameOrID",
			req: workspaceapps.Request{
				AccessMethod:      workspaceapps.AccessMethodPath,
				BasePath:          "/",
				UsernameOrID:      "foo",
				WorkspaceNameOrID: "",
				AgentNameOrID:     "baz",
				AppSlugOrPort:     "qux",
			},
			errContains: "workspace name or ID is required",
		},
		{
			name: "NoAppSlugOrPort",
			req: workspaceapps.Request{
				AccessMethod:      workspaceapps.AccessMethodPath,
				BasePath:          "/",
				UsernameOrID:      "foo",
				WorkspaceNameOrID: "bar",
				AgentNameOrID:     "baz",
				AppSlugOrPort:     "",
			},
			errContains: "app slug or port is required",
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			err := c.req.Validate()
			if c.errContains == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				require.Contains(t, err.Error(), c.errContains)
			}
		})
	}
}
