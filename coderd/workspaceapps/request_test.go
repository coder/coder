package workspaceapps_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/workspaceapps"
)

func Test_RequestValidate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		req         workspaceapps.Request
		noNormalize bool
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
				AccessMethod:  workspaceapps.AccessMethodTerminal,
				BasePath:      "/",
				AgentNameOrID: uuid.New().String(),
			},
		},
		{
			name: "OK4",
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
			noNormalize: true,
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
			name: "InvalidWorkspaceAndAgent/EmptyWorkspace",
			req: workspaceapps.Request{
				AccessMethod:      workspaceapps.AccessMethodPath,
				BasePath:          "/",
				UsernameOrID:      "foo",
				WorkspaceAndAgent: ".bar",
				AppSlugOrPort:     "baz",
			},
			errContains: "workspace name or ID is required",
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
		{
			name: "Prefix/OK",
			req: workspaceapps.Request{
				AccessMethod:      workspaceapps.AccessMethodSubdomain,
				Prefix:            "blah---",
				BasePath:          "/",
				UsernameOrID:      "foo",
				WorkspaceNameOrID: "bar",
				AgentNameOrID:     "baz",
				AppSlugOrPort:     "qux",
			},
		},
		{
			name: "Prefix/Invalid",
			req: workspaceapps.Request{
				AccessMethod:      workspaceapps.AccessMethodSubdomain,
				Prefix:            "blah", // no trailing ---
				BasePath:          "/",
				UsernameOrID:      "foo",
				WorkspaceNameOrID: "bar",
				AgentNameOrID:     "baz",
				AppSlugOrPort:     "qux",
			},
			errContains: "prefix must have a trailing '---'",
		},
		{
			name: "Prefix/NotAllowedPath",
			req: workspaceapps.Request{
				AccessMethod:      workspaceapps.AccessMethodPath,
				Prefix:            "blah---",
				BasePath:          "/",
				UsernameOrID:      "foo",
				WorkspaceNameOrID: "bar",
				AgentNameOrID:     "baz",
				AppSlugOrPort:     "qux",
			},
			errContains: "prefix is only valid for subdomain apps",
		},
		{
			name: "Terminal/OtherFields/UsernameOrID",
			req: workspaceapps.Request{
				AccessMethod:  workspaceapps.AccessMethodTerminal,
				BasePath:      "/",
				UsernameOrID:  "foo",
				AgentNameOrID: uuid.New().String(),
			},
			errContains: "cannot specify any fields other than",
		},
		{
			name: "Terminal/OtherFields/WorkspaceAndAgent",
			req: workspaceapps.Request{
				AccessMethod:      workspaceapps.AccessMethodTerminal,
				BasePath:          "/",
				WorkspaceAndAgent: "bar.baz",
				AgentNameOrID:     uuid.New().String(),
			},
			errContains: "cannot specify any fields other than",
		},
		{
			name: "Terminal/OtherFields/WorkspaceNameOrID",
			req: workspaceapps.Request{
				AccessMethod:      workspaceapps.AccessMethodTerminal,
				BasePath:          "/",
				WorkspaceNameOrID: "bar",
				AgentNameOrID:     uuid.New().String(),
			},
			errContains: "cannot specify any fields other than",
		},
		{
			name: "Terminal/OtherFields/AppSlugOrPort",
			req: workspaceapps.Request{
				AccessMethod:  workspaceapps.AccessMethodTerminal,
				BasePath:      "/",
				AgentNameOrID: uuid.New().String(),
				AppSlugOrPort: "baz",
			},
			errContains: "cannot specify any fields other than",
		},
		{
			name: "Terminal/AgentNameOrID/Empty",
			req: workspaceapps.Request{
				AccessMethod:  workspaceapps.AccessMethodTerminal,
				BasePath:      "/",
				AgentNameOrID: "",
			},
			errContains: "agent name or ID is required",
		},
		{
			name: "Terminal/AgentNameOrID/NotUUID",
			req: workspaceapps.Request{
				AccessMethod:  workspaceapps.AccessMethodTerminal,
				BasePath:      "/",
				AgentNameOrID: "baz",
			},
			errContains: `invalid agent name or ID "baz", must be a UUID`,
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			req := c.req
			if !c.noNormalize {
				req = c.req.Normalize()
			}
			err := req.Validate()
			if c.errContains == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				require.ErrorContains(t, err, c.errContains)
			}
		})
	}
}

// getDatabase is tested heavily in auth_test.go, so we don't have specific
// tests for it here.
