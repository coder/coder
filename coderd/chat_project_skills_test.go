package coderd_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/x/skills"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestChatProjectSkills(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, _ := newChatProjectClient(t)
	orgID := coderdtest.CreateFirstUser(t, client.Client).OrganizationID
	project := createChatProject(t, client, orgID, "Skills Project")
	memberRaw, _ := coderdtest.CreateAnotherUser(t, client.Client, orgID)
	member := codersdk.NewExperimentalClient(memberRaw)
	content := userSkillMarkdown("deploy", "Deploy the app", "Run make deploy.")

	created, err := client.CreateChatProjectSkill(ctx, orgID, project.ID, codersdk.CreateSkillRequest{
		Content:        content,
		Action:         ptr.Ref(true),
		ModelInvocable: ptr.Ref(false),
	})
	require.NoError(t, err)
	require.Equal(t, "Deploy the app", created.Description)
	require.True(t, created.Action)
	require.False(t, created.ModelInvocable)
	list, err := client.ChatProjectSkills(ctx, orgID, project.ID)
	require.NoError(t, err)
	require.Equal(t, []codersdk.SkillMetadata{created.SkillMetadata}, list)

	updated, err := client.UpdateChatProjectSkill(ctx, orgID, project.ID, "deploy", codersdk.UpdateSkillRequest{
		Content: ptr.Ref(userSkillMarkdown("deploy", "Ship the app", "Run make ship.")),
		Enabled: ptr.Ref(false),
	})
	require.NoError(t, err)
	require.Equal(t, "Ship the app", updated.Description)
	require.False(t, updated.Enabled)
	// Omitted settings keep their values, and set ones flip independently.
	require.True(t, updated.Action)
	require.False(t, updated.ModelInvocable)
	updated, err = client.UpdateChatProjectSkill(ctx, orgID, project.ID, "deploy", codersdk.UpdateSkillRequest{
		Action:         ptr.Ref(false),
		ModelInvocable: ptr.Ref(true),
	})
	require.NoError(t, err)
	require.False(t, updated.Action)
	require.True(t, updated.ModelInvocable)
	require.Equal(t, "Ship the app", updated.Description)

	_, err = client.CreateChatProjectSkill(ctx, orgID, project.ID, codersdk.CreateSkillRequest{Content: content})
	requireSDKErrorStatus(t, err, http.StatusConflict)
	_, err = client.UpdateChatProjectSkill(ctx, orgID, project.ID, "deploy", codersdk.UpdateSkillRequest{
		Content: ptr.Ref(userSkillMarkdown("other", "Other", "Body.")),
	})
	requireSDKErrorStatus(t, err, http.StatusBadRequest)

	// A member who cannot read the project gets 404 on every route.
	for i, call := range []func() error{
		func() error { _, err := member.ChatProjectSkills(ctx, orgID, project.ID); return err },
		func() error { _, err := member.ChatProjectSkill(ctx, orgID, project.ID, "deploy"); return err },
		func() error {
			_, err := member.CreateChatProjectSkill(ctx, orgID, project.ID, codersdk.CreateSkillRequest{Content: content})
			return err
		},
		func() error {
			_, err := member.UpdateChatProjectSkill(ctx, orgID, project.ID, "deploy", codersdk.UpdateSkillRequest{Enabled: ptr.Ref(true)})
			return err
		},
		func() error { return member.DeleteChatProjectSkill(ctx, orgID, project.ID, "deploy") },
	} {
		requireSDKErrorStatus(t, call(), http.StatusNotFound, "member call %d", i)
	}
	// Reads return the update, and rejected writes leave it alone.
	got, err := client.ChatProjectSkill(ctx, orgID, project.ID, "deploy")
	require.NoError(t, err)
	require.Equal(t, updated, got)

	require.NoError(t, client.DeleteChatProjectSkill(ctx, orgID, project.ID, "deploy"))
	_, err = client.ChatProjectSkill(ctx, orgID, project.ID, "deploy")
	requireSDKErrorStatus(t, err, http.StatusNotFound)

	for i := range skills.MaxPersonalSkillsPerUser {
		_, err := client.CreateChatProjectSkill(ctx, orgID, project.ID, codersdk.CreateSkillRequest{Content: userSkillMarkdown(fmt.Sprintf("skill-%03d", i), "Limit", "Body.")})
		require.NoError(t, err)
	}
	_, err = client.CreateChatProjectSkill(ctx, orgID, project.ID, codersdk.CreateSkillRequest{Content: content})
	require.Equal(t, "Project skill limit reached.", requireSDKErrorStatus(t, err, http.StatusConflict).Message)
}
