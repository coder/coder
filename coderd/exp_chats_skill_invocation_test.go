package coderd_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestChatSkillInvocation(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, db := newChatClientWithDatabase(t)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	_ = createChatModel(t, client)
	_, err := client.CreateUserSkill(ctx, codersdk.Me, codersdk.CreateSkillRequest{
		Content: "---\nname: notes\ndescription: Notes\n---\n\nTake notes.\n",
	})
	require.NoError(t, err)

	for _, tt := range []struct {
		name   string
		part   codersdk.ChatInputPart
		detail string
	}{
		{
			name:   "UnknownSource",
			part:   codersdk.ChatInputPart{Type: codersdk.ChatInputPartTypeSkillInvocation, SkillSource: "team", SkillName: "notes"},
			detail: `content[0].skill_source "team" is not supported.`,
		},
		{
			name:   "InvalidName",
			part:   codersdk.ChatInputPart{Type: codersdk.ChatInputPartTypeSkillInvocation, SkillSource: codersdk.ChatSkillSourcePersonal, SkillName: "Bad_Name"},
			detail: "content[0].skill_name must be a kebab-case skill name.",
		},
	} {
		_, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content:        []codersdk.ChatInputPart{tt.part},
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Invalid input part.", sdkErr.Message, tt.name)
		require.Equal(t, tt.detail, sdkErr.Detail, tt.name)
	}

	// The body is stored with the message but never returned by the API.
	chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: firstUser.OrganizationID,
		Content: []codersdk.ChatInputPart{
			{Type: codersdk.ChatInputPartTypeSkillInvocation, SkillSource: codersdk.ChatSkillSourcePersonal, SkillName: "notes"},
			{Type: codersdk.ChatInputPartTypeText, Text: "go"},
		},
	})
	require.NoError(t, err)
	messages, err := db.GetChatMessagesByChatID(dbauthz.AsSystemRestricted(ctx), database.GetChatMessagesByChatIDParams{ChatID: chat.ID})
	require.NoError(t, err)
	stored, err := chatprompt.ParseContent(findUserMessage(t, messages))
	require.NoError(t, err)
	require.Equal(t, codersdk.ChatMessagePartTypeSkillInvocation, stored[0].Type)
	require.Equal(t, "Take notes.", stored[0].SkillBody)

	response, err := client.GetChatMessages(ctx, chat.ID, nil)
	require.NoError(t, err)
	var returned []codersdk.ChatMessagePart
	for _, message := range response.Messages {
		for _, part := range message.Content {
			if part.Type == codersdk.ChatMessagePartTypeSkillInvocation {
				returned = append(returned, part)
			}
		}
	}
	require.Equal(t, []codersdk.ChatMessagePart{
		codersdk.ChatMessageSkillInvocation(codersdk.ChatSkillSourcePersonal, "notes"),
	}, returned)

	// A skill the owner cannot load rejects the send.
	_, err = client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
		Content: []codersdk.ChatInputPart{
			{Type: codersdk.ChatInputPartTypeSkillInvocation, SkillSource: codersdk.ChatSkillSourcePersonal, SkillName: "missing"},
		},
	})
	sdkErr := requireSDKError(t, err, http.StatusBadRequest)
	require.Equal(t, "Skill personal/missing is not available.", sdkErr.Message)
}
