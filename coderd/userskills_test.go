package coderd_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestPatchUserSkill(t *testing.T) {
	t.Parallel()

	ownerRawClient := coderdtest.New(t, nil)
	firstUser := coderdtest.CreateFirstUser(t, ownerRawClient)
	memberRawClient, member := coderdtest.CreateAnotherUser(t, ownerRawClient, firstUser.OrganizationID)
	memberClient := codersdk.NewExperimentalClient(memberRawClient)
	auditorRawClient, _ := coderdtest.CreateAnotherUser(t, ownerRawClient, firstUser.OrganizationID, rbac.RoleAuditor())
	auditorClient := codersdk.NewExperimentalClient(auditorRawClient)
	ctx := testutil.Context(t, testutil.WaitMedium)

	_, err := memberClient.CreateUserSkill(ctx, codersdk.Me, codersdk.CreateUserSkillRequest{
		Content: userSkillTestContent("forbidden-skill", "Original body."),
	})
	require.NoError(t, err)

	_, err = auditorClient.UpdateUserSkill(ctx, member.ID.String(), "forbidden-skill", codersdk.UpdateUserSkillRequest{
		Content: userSkillTestContent("forbidden-skill", "Updated body."),
	})
	require.Error(t, err)
	var sdkErr *codersdk.Error
	require.ErrorAs(t, err, &sdkErr)
	assert.Equal(t, http.StatusForbidden, sdkErr.StatusCode())
}

func userSkillTestContent(name string, body string) string {
	return "---\nname: " + name + "\ndescription: Test skill\n---\n" + body + "\n"
}
