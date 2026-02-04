package coderd_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/chats"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/toolsdk"
	"github.com/coder/coder/v2/testutil"
)

func TestChats_AnthropicIntegration(t *testing.T) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		t.Skip("Skipping test as ANTHROPIC_API_KEY not set")
	}

	model := os.Getenv("ANTHROPIC_MODEL")
	if model == "" {
		model = string(anthropic.ModelClaudeOpus4_5)
		t.Logf("ANTHROPIC_MODEL not set; defaulting to %s", model)
	}

	ctx := context.Background()
	client, _, api := coderdtest.NewWithAPI(t, nil)
	user := coderdtest.CreateFirstUser(t, client)

	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, template.ID)

	api.ChatRunner = chats.NewRunner(chats.RunnerOptions{
		DB:         api.Database,
		Logger:     api.Logger,
		AccessURL:  api.AccessURL,
		HTTPClient: api.HTTPClient,
		Tools:      []toolsdk.GenericTool{toolsdk.WorkspaceBash.Generic()},
	})

	chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
		Provider:    "anthropic",
		Model:       model,
		WorkspaceID: &workspace.ID,
	})
	require.NoError(t, err)

	resp, err := client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
		Content: "Respond with exactly the word pong and do not use any tools.",
	})
	require.NoError(t, err)
	require.NotEmpty(t, resp.RunID)

	require.Eventually(t, func() bool {
		msgs, err := client.ChatMessages(ctx, chat.ID)
		if err != nil {
			return false
		}
		for _, msg := range msgs {
			if msg.Role != "assistant" {
				continue
			}
			var env chats.MessageEnvelope
			if err := json.Unmarshal(msg.Content, &env); err != nil {
				continue
			}
			if env.RunID != resp.RunID {
				continue
			}
			if strings.Contains(strings.ToLower(env.Message.Content), "pong") {
				return true
			}
		}
		return false
	}, testutil.WaitSuperLong, testutil.IntervalSlow)
}
