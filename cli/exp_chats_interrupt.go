package cli

import (
	"fmt"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

type ChatInterruptRow struct {
	codersdk.Chat `table:"-"`

	ID        string              `json:"-" table:"id,nosort"`
	Title     string              `json:"-" table:"title"`
	Status    codersdk.ChatStatus `json:"-" table:"status"`
	UpdatedAt time.Time           `json:"-" table:"updated at"`
}

func chatInterruptRowFromChat(chat codersdk.Chat) ChatInterruptRow {
	return ChatInterruptRow{
		Chat:      chat,
		ID:        chat.ID.String(),
		Title:     chat.Title,
		Status:    chat.Status,
		UpdatedAt: chat.UpdatedAt,
	}
}

func (r *RootCmd) chatsInterrupt() *serpent.Command {
	formatter := cliui.NewOutputFormatter(
		cliui.ChangeFormatterData(
			cliui.TableFormat([]ChatInterruptRow{}, []string{"id", "title", "status", "updated at"}),
			func(data any) (any, error) {
				chat, ok := data.(codersdk.Chat)
				if !ok {
					return nil, xerrors.Errorf("expected type %T, got %T", codersdk.Chat{}, data)
				}
				return []ChatInterruptRow{chatInterruptRowFromChat(chat)}, nil
			},
		),
		cliui.JSONFormat(),
	)

	cmd := &serpent.Command{
		Use:   "interrupt <chat-id>",
		Short: "Interrupt a running chat.",
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

			chat, err := client.InterruptChat(inv.Context(), chatID)
			if err != nil {
				return xerrors.Errorf("interrupt chat %s: %w", chatID, err)
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
