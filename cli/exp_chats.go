package cli

import (
	"context"
	"io"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

var (
	_ = parseChatID
	_ = readPrompt
	_ = resolveModel
	_ = promptToContent
)

func (r *RootCmd) chatsCommand() *serpent.Command {
	cmd := &serpent.Command{
		Use:     "chats",
		Short:   "Manage and interact with AI chats.",
		Aliases: []string{"chat"},
		Handler: func(i *serpent.Invocation) error {
			return i.Command.HelpHandler(i)
		},
		Children: []*serpent.Command{
			r.chatsList(),
			r.chatsShow(),
			r.chatsModels(),
			r.chatsTranscript(),
			r.chatsStart(),
			r.chatsSend(),
			r.chatsWatch(),
			r.chatsInterrupt(),
			r.chatsDiff(),
		},
	}
	return cmd
}

func parseChatID(inv *serpent.Invocation) (uuid.UUID, error) {
	if len(inv.Args) == 0 {
		return uuid.Nil, xerrors.New("chat ID is required")
	}

	id, err := uuid.Parse(inv.Args[0])
	if err != nil {
		return uuid.Nil, xerrors.Errorf("invalid chat ID %q: %w", inv.Args[0], err)
	}

	return id, nil
}

func readPrompt(inv *serpent.Invocation) (string, error) {
	if len(inv.Args) > 0 {
		return strings.Join(inv.Args, " "), nil
	}

	bytes, err := io.ReadAll(inv.Stdin)
	if err != nil {
		return "", xerrors.Errorf("reading stdin: %w", err)
	}

	prompt := strings.TrimSpace(string(bytes))
	if prompt == "" {
		return "", xerrors.New("prompt is required (provide as argument or pipe to stdin)")
	}

	return prompt, nil
}

//nolint:nilnil // A nil UUID indicates that no model override was provided.
func resolveModel(ctx context.Context, client *codersdk.ExperimentalClient, modelFlag string) (*uuid.UUID, error) {
	if modelFlag == "" {
		return nil, nil
	}

	if id, err := uuid.Parse(modelFlag); err == nil {
		return &id, nil
	}

	catalog, err := client.ListChatModels(ctx)
	if err != nil {
		return nil, xerrors.Errorf("listing models: %w", err)
	}

	for _, provider := range catalog.Providers {
		for _, model := range provider.Models {
			if model.ID == modelFlag || model.Provider+"/"+model.Model == modelFlag || model.DisplayName == modelFlag {
				id, err := uuid.Parse(model.ID)
				if err != nil {
					return nil, xerrors.Errorf("invalid model ID %q: %w", model.ID, err)
				}
				return &id, nil
			}
		}
	}

	return nil, xerrors.Errorf("unknown model %q", modelFlag)
}

func promptToContent(prompt string) []codersdk.ChatInputPart {
	return []codersdk.ChatInputPart{
		{
			Type: codersdk.ChatInputPartTypeText,
			Text: prompt,
		},
	}
}
