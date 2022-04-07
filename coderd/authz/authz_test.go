package authz_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/authz"
)

func TestAuthorize(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		subject  authz.Subject
		resource authz.Resource
		actions  []authz.Action
		error    string
	}{
		{
			name: "unauthenticated user cannot perform an action",
			subject: authz.SubjectTODO{
				UserID: "",
				Site:   []authz.Role{authz.RoleNoPerm},
			},
			resource: authz.ResourceWorkspace,
			actions:  []authz.Action{authz.ActionRead, authz.ActionCreate, authz.ActionDelete, authz.ActionUpdate},
			error:    "unauthorized",
		},
		{
			name: "admin can do anything",
			subject: authz.SubjectTODO{
				UserID: "admin",
				Site:   []authz.Role{authz.RoleAllowAll},
			},
			resource: authz.ResourceWorkspace,
			actions:  []authz.Action{authz.ActionRead, authz.ActionCreate, authz.ActionDelete, authz.ActionUpdate},
			error:    "",
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			for _, action := range testCase.actions {
				err := authz.Authorize(testCase.subject, testCase.resource, action)
				if testCase.error == "" {
					require.NoError(t, err, "expected no error for testcase testcase %q action %s", testCase.name, action)
					continue
				}
				require.EqualError(t, err, testCase.error, "unexpected error")
			}
		})
	}
}
