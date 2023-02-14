package coderd

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_workspaceAppRequestValidate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		req         workspaceAppRequest
		errContains string
	}{
		{
			name: "OK1",
			req: workspaceAppRequest{
				AccessMethod:      workspaceAppAccessMethodPath,
				BasePath:          "/",
				UsernameOrID:      "foo",
				WorkspaceNameOrID: "bar",
				AgentNameOrID:     "baz",
				AppSlugOrPort:     "qux",
			},
		},
		{
			name: "OK2",
			req: workspaceAppRequest{
				AccessMethod:      workspaceAppAccessMethodSubdomain,
				BasePath:          "/",
				UsernameOrID:      "foo",
				WorkspaceAndAgent: "bar.baz",
				AppSlugOrPort:     "qux",
			},
		},
		{
			name: "OK3",
			req: workspaceAppRequest{
				AccessMethod:      workspaceAppAccessMethodPath,
				BasePath:          "/",
				UsernameOrID:      "foo",
				WorkspaceNameOrID: "bar",
				AppSlugOrPort:     "baz",
			},
		},
		{
			name: "NoAccessMethod",
			req: workspaceAppRequest{
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
			req: workspaceAppRequest{
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
			req: workspaceAppRequest{
				AccessMethod:      workspaceAppAccessMethodPath,
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
			req: workspaceAppRequest{
				AccessMethod:      workspaceAppAccessMethodPath,
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
			req: workspaceAppRequest{
				AccessMethod:      workspaceAppAccessMethodPath,
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
			req: workspaceAppRequest{
				AccessMethod:      workspaceAppAccessMethodPath,
				BasePath:          "/",
				UsernameOrID:      "foo",
				WorkspaceAndAgent: ".bar",
				AppSlugOrPort:     "baz",
			},
			errContains: "invalid workspace and agent",
		},
		{
			name: "InvalidWorkspaceAndAgent/Empty2",
			req: workspaceAppRequest{
				AccessMethod:      workspaceAppAccessMethodPath,
				BasePath:          "/",
				UsernameOrID:      "foo",
				WorkspaceAndAgent: "bar.",
				AppSlugOrPort:     "baz",
			},
			errContains: "invalid workspace and agent",
		},
		{
			name: "InvalidWorkspaceAndAgent/TwoDots",
			req: workspaceAppRequest{
				AccessMethod:      workspaceAppAccessMethodPath,
				BasePath:          "/",
				UsernameOrID:      "foo",
				WorkspaceAndAgent: "bar.baz.qux",
				AppSlugOrPort:     "baz",
			},
			errContains: "invalid workspace and agent",
		},
		{
			name: "AmbiguousWorkspaceAndAgent/1",
			req: workspaceAppRequest{
				AccessMethod:      workspaceAppAccessMethodPath,
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
			req: workspaceAppRequest{
				AccessMethod:      workspaceAppAccessMethodPath,
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
			req: workspaceAppRequest{
				AccessMethod:      workspaceAppAccessMethodPath,
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
			req: workspaceAppRequest{
				AccessMethod:      workspaceAppAccessMethodPath,
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

// TODO: resolveWorkspaceApp tests
