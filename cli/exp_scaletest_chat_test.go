//go:build !slim

package cli_test

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/sloghuman"
	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/aibridgedtest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/llmmock"
	"github.com/coder/coder/v2/testutil"
)

const scaletestChatPrompt = "Reply with one short sentence from the scaletest."

func TestScaleTestChat(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	values := coderdtest.DeploymentValues(t, func(dv *codersdk.DeploymentValues) {
		require.NoError(t, dv.AI.BridgeConfig.Enabled.Set("true"))
	})
	client, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
		DeploymentValues: values,
	})
	aibridgedtest.StartTestAIBridgeDaemon(t.Context(), t, api, nil)
	coderdtest.CreateFirstUser(t, client)

	server := new(llmmock.Server)
	require.NoError(t, server.Start(context.Background(), llmmock.Config{
		Address: "127.0.0.1:0",
		Logger:  slog.Make(sloghuman.Sink(io.Discard)).Leveled(slog.LevelDebug),
	}))
	t.Cleanup(func() {
		require.NoError(t, server.Stop())
	})
	mockURL := server.APIAddress() + "/v1"

	inv, root := clitest.New(t,
		"exp", "scaletest", "chat",
		"--chats-per-workspace", "1",
		"--turns", "1",
		"--prompt", scaletestChatPrompt,
		"--timeout", "30s",
		"--job-timeout", "30s",
		"--cleanup-timeout", "30s",
		"--cleanup-job-timeout", "30s",
		"--scaletest-prometheus-address", "127.0.0.1:0",
		"--scaletest-prometheus-wait", "0s",
		"--provider-propagation-wait", "10ms",
		"--llm-mock-url", mockURL,
	)
	//nolint:gocritic // The scaletest chat command requires an admin client.
	clitest.SetupConfig(t, client, root)

	var stderr bytes.Buffer
	inv.Stdout = io.Discard
	inv.Stderr = &stderr

	err := inv.WithContext(ctx).Run()
	require.NoError(t, err, stderr.String())
	require.Contains(t, stderr.String(), "Scale test passed: 1/1 runs succeeded")

	provider, err := client.AIProvider(ctx, "coder-scaletest-mock")
	require.NoError(t, err)
	require.Equal(t, mockURL, provider.BaseURL)

	expClient := codersdk.NewExperimentalClient(client)
	configs, err := expClient.ListChatModelConfigs(ctx)
	require.NoError(t, err)
	matchingConfigs := scaletestModelConfigsForProvider(configs, provider.ID)
	require.Len(t, matchingConfigs, 1)
	require.True(t, matchingConfigs[0].Enabled)

	chats, err := expClient.ListChats(ctx, &codersdk.ListChatsOptions{Query: "archived:true"})
	require.NoError(t, err)

	var scaletestMessages []codersdk.ChatMessage
	for _, chat := range chats {
		resp, err := expClient.GetChatMessages(ctx, chat.ID, nil)
		require.NoError(t, err)
		if userText, ok := chatMessageText(resp.Messages, codersdk.ChatMessageRoleUser); ok &&
			strings.Contains(userText, scaletestChatPrompt) {
			scaletestMessages = resp.Messages
			break
		}
	}
	require.NotEmpty(t, scaletestMessages)
	assistantText, ok := chatMessageText(scaletestMessages, codersdk.ChatMessageRoleAssistant)
	require.True(t, ok, "expected an assistant reply in the scaletest chat")
	require.NotEmpty(t, assistantText)
}

// chatMessageText concatenates the text parts of every message with the given
// role, reporting whether any such message was found. It aggregates across
// messages because the API returns them newest-first and a turn can produce
// more than one message per role.
func chatMessageText(messages []codersdk.ChatMessage, role codersdk.ChatMessageRole) (string, bool) {
	var (
		b     strings.Builder
		found bool
	)
	for _, msg := range messages {
		if msg.Role != role {
			continue
		}
		found = true
		for _, part := range msg.Content {
			if part.Type == codersdk.ChatMessagePartTypeText {
				_, _ = b.WriteString(part.Text)
			}
		}
	}
	return b.String(), found
}

func scaletestModelConfigsForProvider(configs []codersdk.ChatModelConfig, providerID uuid.UUID) []codersdk.ChatModelConfig {
	matches := make([]codersdk.ChatModelConfig, 0, 1)
	for _, config := range configs {
		if config.AIProviderID != providerID {
			continue
		}
		if config.Model != "scaletest-model" {
			continue
		}
		matches = append(matches, config)
	}
	return matches
}
