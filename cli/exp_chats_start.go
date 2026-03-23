package cli

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

type chatStartRow struct {
	ID        uuid.UUID           `json:"-" table:"id,default_sort"`
	Status    codersdk.ChatStatus `json:"-" table:"status"`
	Workspace string              `json:"-" table:"workspace"`
	UpdatedAt time.Time           `json:"-" table:"updated at"`
}

func chatStartRowFromChat(chat codersdk.Chat) chatStartRow {
	workspace := ""
	if chat.WorkspaceID != nil {
		workspace = chat.WorkspaceID.String()
	}

	return chatStartRow{
		ID:        chat.ID,
		Status:    chat.Status,
		Workspace: workspace,
		UpdatedAt: chat.UpdatedAt,
	}
}

func (r *RootCmd) chatsStart() *serpent.Command {
	var (
		workspaceFlag string
		modelFlag     string
		follow        bool
		formatter     = cliui.NewOutputFormatter(
			cliui.ChangeFormatterData(
				cliui.TableFormat([]chatStartRow{}, []string{"id", "status", "workspace", "updated at"}),
				func(data any) (any, error) {
					chat, ok := data.(codersdk.Chat)
					if !ok {
						return nil, xerrors.Errorf("expected codersdk.Chat, got %T", data)
					}
					return []chatStartRow{chatStartRowFromChat(chat)}, nil
				},
			),
			cliui.JSONFormat(),
		)
	)

	cmd := &serpent.Command{
		Use:   "start [prompt]",
		Short: "Start a new chat.",
		Options: serpent.OptionSet{
			{
				Name:        "workspace",
				Flag:        "workspace",
				Description: "Associate the chat with a workspace by name, owner/name, or UUID.",
				Value:       serpent.StringOf(&workspaceFlag),
			},
			{
				Name:        "model",
				Flag:        "model",
				Description: "Choose a model by ID, provider/model, or display name.",
				Value:       serpent.StringOf(&modelFlag),
			},
			{
				Name:          "follow",
				Flag:          "follow",
				FlagShorthand: "f",
				Default:       "false",
				Description:   "Watch the chat after creating it.",
				Value:         serpent.BoolOf(&follow),
			},
		},
		Handler: func(inv *serpent.Invocation) error {
			prompt, err := readPrompt(inv)
			if err != nil {
				return err
			}

			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			ctx := inv.Context()

			var workspaceID *uuid.UUID
			if workspaceFlag != "" {
				workspace, err := namedWorkspace(ctx, client, workspaceFlag)
				if err != nil {
					return xerrors.Errorf("resolve workspace %q: %w", workspaceFlag, err)
				}
				workspaceID = &workspace.ID
			}

			modelID, err := resolveModel(ctx, client, modelFlag)
			if err != nil {
				return err
			}

			chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
				Content:       promptToContent(prompt),
				WorkspaceID:   workspaceID,
				ModelConfigID: modelID,
			})
			if err != nil {
				return err
			}

			if follow {
				return watchChat(
					ctx,
					client,
					chat.ID,
					nil,
					chatWatchWriters{stdout: inv.Stdout, stderr: inv.Stderr},
					formatter.FormatID() == "json",
				)
			}

			out, err := formatter.Format(ctx, chat)
			if err != nil {
				return xerrors.Errorf("format chat: %w", err)
			}

			_, err = fmt.Fprintln(inv.Stdout, out)
			return err
		},
	}

	formatter.AttachOptions(&cmd.Options)
	return cmd
}
