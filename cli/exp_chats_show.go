package cli

import (
	"fmt"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

type chatShowRow struct {
	codersdk.Chat `table:"-"`

	ID          string              `json:"-" table:"id,nosort"`
	Title       string              `json:"-" table:"title"`
	Status      codersdk.ChatStatus `json:"-" table:"status"`
	WorkspaceID string              `json:"-" table:"workspace id"`
	Created     time.Time           `json:"-" table:"created"`
	Updated     time.Time           `json:"-" table:"updated"`
	LastError   string              `json:"-" table:"last error"`
}

func chatShowRowFromChat(chat codersdk.Chat) chatShowRow {
	workspaceID := ""
	if chat.WorkspaceID != nil {
		workspaceID = chat.WorkspaceID.String()
	}

	lastError := ""
	if chat.LastError != nil {
		lastError = *chat.LastError
	}

	return chatShowRow{
		Chat:        chat,
		ID:          chat.ID.String(),
		Title:       chat.Title,
		Status:      chat.Status,
		WorkspaceID: workspaceID,
		Created:     chat.CreatedAt,
		Updated:     chat.UpdatedAt,
		LastError:   lastError,
	}
}

func (r *RootCmd) chatsShow() *serpent.Command {
	formatter := cliui.NewOutputFormatter(
		cliui.ChangeFormatterData(
			cliui.TableFormat([]chatShowRow{}, []string{"id", "title", "status", "workspace id", "created", "updated", "last error"}),
			func(data any) (any, error) {
				chat, ok := data.(codersdk.Chat)
				if !ok {
					return nil, xerrors.Errorf("expected type %T, got %T", codersdk.Chat{}, data)
				}
				return []chatShowRow{chatShowRowFromChat(chat)}, nil
			},
		),
		cliui.JSONFormat(),
	)

	cmd := &serpent.Command{
		Use:   "show <chat-id>",
		Short: "Show details for a chat.",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
		),
		Handler: func(inv *serpent.Invocation) error {
			chatID, err := parseChatID(inv)
			if err != nil {
				return err
			}

			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			expClient := codersdk.NewExperimentalClient(client)

			chat, err := expClient.GetChat(inv.Context(), chatID)
			if err != nil {
				return xerrors.Errorf("get chat %s: %w", chatID, err)
			}

			out, err := formatter.Format(inv.Context(), chat)
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
