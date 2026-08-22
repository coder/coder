// Command smoke-chat-models exercises the AI chat model configs attached to a
// specific provider on a Coder deployment by creating a chat with a single
// "testing" message per selected model and asserting the model replies.
//
// Usage:
//
//	CODER_URL=https://dev.coder.com CODER_SESSION_TOKEN=<token> go run ./scripts/smoke-chat-models [-model <name>] <provider>
//
// <provider> is required and is the provider's name or UUID (for example
// "agents-bedrock" or "bedrock-mantle-us-east-1"). Only enabled model configs
// attached to that provider are tested. Flags must come before the provider
// argument (Go's flag package stops parsing at the first positional).
// Use -model to narrow to a single model by name (substring match). The tool
// exits non-zero if any selected model fails to produce an assistant reply, or
// if the provider cannot be resolved.
package main

import (
	"context"
	"flag"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
)

const (
	maxWait      = 5 * time.Minute
	pollInterval = 2 * time.Second
)

func main() {
	model := flag.String("model", "", "only test the single model whose name contains this substring")
	flag.Usage = func() {
		_, _ = fmt.Fprintf(os.Stderr, "usage: smoke-chat-models [-model <name>] <provider-name-or-id>\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	args := flag.Args()
	// Reject extra positionals so a misplaced trailing flag (e.g. -model after
	// the provider) is caught instead of silently ignored.
	if len(args) != 1 {
		flag.Usage()
		os.Exit(2)
	}
	if err := run(context.Background(), args[0], *model); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "smoke-chat-models: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, providerSelector, modelFilter string) error {
	serverURL, err := url.Parse(envOr("CODER_URL", "https://dev.coder.com"))
	if err != nil {
		return xerrors.Errorf("parse CODER_URL: %w", err)
	}
	token := os.Getenv("CODER_SESSION_TOKEN")
	if token == "" {
		return xerrors.New("CODER_SESSION_TOKEN must be set")
	}

	client := codersdk.New(serverURL, codersdk.WithSessionToken(token))
	exp := codersdk.NewExperimentalClient(client)

	// Resolve the provider by name or UUID so we get a stable ID and a friendly
	// name for output. This rejects unknown selectors early.
	provider, err := client.AIProvider(ctx, providerSelector)
	if err != nil {
		return xerrors.Errorf("resolve provider %q: %w", providerSelector, err)
	}
	providerID := provider.ID
	_, _ = fmt.Printf("provider %q (%s), type %s, %s\n", provider.Name, providerID, provider.Type, provider.BaseURL)

	me, err := client.User(ctx, codersdk.Me)
	if err != nil {
		return xerrors.Errorf("resolve current user: %w", err)
	}
	if len(me.OrganizationIDs) == 0 {
		return xerrors.New("current user has no organization")
	}
	orgID := me.OrganizationIDs[0]

	configs, err := exp.ListChatModelConfigs(ctx)
	if err != nil {
		return xerrors.Errorf("list chat model configs: %w", err)
	}

	var (
		nSelected int
		passed    int
		failed    int
		failures  []string
	)
	for _, cfg := range configs {
		if !cfg.Enabled {
			continue
		}
		if cfg.AIProviderID != providerID {
			continue
		}
		if modelFilter != "" && !strings.Contains(cfg.Model, modelFilter) {
			continue
		}
		nSelected++
		label := fmt.Sprintf("%s (%s)", cfg.DisplayName, cfg.Model)

		reply, status, err := smokeModel(ctx, exp, orgID, cfg)
		if err != nil {
			failed++
			failures = append(failures, fmt.Sprintf("  FAIL %-40s: %v", label, err))
			_, _ = fmt.Printf("FAIL %-40s: %v\n", label, err)
			continue
		}
		if reply == "" {
			failed++
			failures = append(failures, fmt.Sprintf("  FAIL %-40s: chat reached %q but no assistant text reply", label, status))
			_, _ = fmt.Printf("FAIL %-40s: chat reached %q but no assistant text reply\n", label, status)
			continue
		}
		passed++
		_, _ = fmt.Printf("PASS %-40s: %q\n", label, truncate(reply, 120))
	}

	if nSelected == 0 {
		if modelFilter != "" {
			return xerrors.Errorf("no enabled model configs on provider %q match %q", provider.Name, modelFilter)
		}
		return xerrors.Errorf("provider %q has no enabled model configs", provider.Name)
	}

	_, _ = fmt.Printf("\nsummary: %d selected, %d passed, %d failed\n", nSelected, passed, failed)
	if len(failures) > 0 {
		return xerrors.Errorf("failed models:\n%s", strings.Join(failures, "\n"))
	}
	return nil
}

// smokeModel creates a chat for one model config, waits for it to reach a
// terminal state, and returns the assistant's reply text.
func smokeModel(
	ctx context.Context,
	exp *codersdk.ExperimentalClient,
	orgID uuid.UUID,
	cfg codersdk.ChatModelConfig,
) (string, codersdk.ChatStatus, error) {
	modelID := cfg.ID

	chat, err := exp.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: orgID,
		Content: []codersdk.ChatInputPart{
			{Type: codersdk.ChatInputPartTypeText, Text: "testing"},
		},
		ModelConfigID: &modelID,
		ClientType:    codersdk.ChatClientTypeAPI,
	})
	if err != nil {
		return "", "", xerrors.Errorf("create chat: %w", err)
	}

	status, chat, err := waitForChat(ctx, exp, chat.ID)
	if err != nil {
		return "", "", err
	}

	if status == codersdk.ChatStatusError {
		return "", status, chatError(chat)
	}

	msgs, err := exp.GetChatMessages(ctx, chat.ID, nil)
	if err != nil {
		return "", status, xerrors.Errorf("get chat messages: %w", err)
	}

	reply := assistantText(msgs)
	return reply, status, nil
}

// waitForChat polls GetChat until the chat reaches a terminal status, returning
// the terminal Chat so the caller can inspect LastError on failure.
func waitForChat(ctx context.Context, exp *codersdk.ExperimentalClient, chatID uuid.UUID) (codersdk.ChatStatus, codersdk.Chat, error) {
	deadline := time.Now().Add(maxWait)
	for {
		chat, err := exp.GetChat(ctx, chatID)
		if err != nil {
			return "", codersdk.Chat{}, xerrors.Errorf("get chat: %w", err)
		}
		switch chat.Status {
		case codersdk.ChatStatusWaiting, codersdk.ChatStatusError:
			return chat.Status, chat, nil
		}
		if time.Now().After(deadline) {
			return chat.Status, chat, xerrors.Errorf("timed out waiting for chat to finish (last status %q)", chat.Status)
		}
		select {
		case <-ctx.Done():
			return chat.Status, chat, ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}

// chatError renders a chat's LastError as an error.
func chatError(chat codersdk.Chat) error {
	if chat.LastError == nil {
		return xerrors.New("chat ended with error status (no detail)")
	}
	e := chat.LastError
	msg := fmt.Sprintf("chat error [kind=%s]: %s", e.Kind, e.Message)
	if e.Detail != "" {
		msg += ": " + e.Detail
	}
	if e.StatusCode != 0 {
		msg += fmt.Sprintf(" (status %d)", e.StatusCode)
	}
	return xerrors.New(msg)
}

// assistantText extracts the first non-empty text part from the last assistant
// message in the chat.
func assistantText(resp codersdk.ChatMessagesResponse) string {
	for i := len(resp.Messages) - 1; i >= 0; i-- {
		msg := resp.Messages[i]
		if msg.Role != codersdk.ChatMessageRoleAssistant {
			continue
		}
		for _, part := range msg.Content {
			if part.Type == codersdk.ChatMessagePartTypeText && part.Text != "" {
				return part.Text
			}
		}
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
