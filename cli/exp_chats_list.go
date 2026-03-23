package cli

import (
	"fmt"
	"strings"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

type ChatListRow struct {
	codersdk.Chat `table:"-"`

	ID      string              `json:"-" table:"id,nosort"`
	Title   string              `json:"-" table:"title"`
	Status  codersdk.ChatStatus `json:"-" table:"status"`
	Created time.Time           `json:"-" table:"created"`
	Updated time.Time           `json:"-" table:"updated"`
}

func chatListRowFromChat(chat codersdk.Chat) ChatListRow {
	return ChatListRow{
		Chat:    chat,
		ID:      chat.ID.String(),
		Title:   chat.Title,
		Status:  chat.Status,
		Created: chat.CreatedAt,
		Updated: chat.UpdatedAt,
	}
}

func (r *RootCmd) chatsList() *serpent.Command {
	var (
		search    string
		archived  bool
		limit     int64
		formatter = cliui.NewOutputFormatter(
			cliui.ChangeFormatterData(
				cliui.TableFormat([]ChatListRow{}, []string{"id", "title", "status", "created", "updated"}),
				func(data any) (any, error) {
					chats, ok := data.([]codersdk.Chat)
					if !ok {
						return nil, xerrors.Errorf("expected type %T, got %T", []codersdk.Chat{}, data)
					}

					rows := make([]ChatListRow, len(chats))
					for i, chat := range chats {
						rows[i] = chatListRowFromChat(chat)
					}
					return rows, nil
				},
			),
			cliui.JSONFormat(),
		)
	)

	cmd := &serpent.Command{
		Use:   "list",
		Short: "List chats.",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(0),
		),
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			queryParts := make([]string, 0, 2)
			if trimmedSearch := strings.TrimSpace(search); trimmedSearch != "" {
				queryParts = append(queryParts, trimmedSearch)
			}
			if archived {
				queryParts = append(queryParts, "archived:true")
			}

			chats, err := client.ListChats(inv.Context(), &codersdk.ListChatsOptions{
				Query: strings.Join(queryParts, " "),
				Pagination: codersdk.Pagination{
					Limit: int(limit),
				},
			})
			if err != nil {
				return xerrors.Errorf("list chats: %w", err)
			}

			out, err := formatter.Format(inv.Context(), chats)
			if err != nil {
				return xerrors.Errorf("format chats: %w", err)
			}
			if out == "" {
				cliui.Infof(inv.Stderr, "No chats found.")
				return nil
			}

			_, err = fmt.Fprintln(inv.Stdout, out)
			return err
		},
	}

	cmd.Options = append(cmd.Options,
		serpent.Option{
			Name:          "search",
			Description:   "Structured search query (for example archived:true).",
			Flag:          "search",
			FlagShorthand: "q",
			Value:         serpent.StringOf(&search),
		},
		serpent.Option{
			Name:        "archived",
			Description: "Show only archived chats.",
			Flag:        "archived",
			Value:       serpent.BoolOf(&archived),
		},
		serpent.Option{
			Name:        "limit",
			Description: "Maximum number of chats to return.",
			Flag:        "limit",
			Default:     "25",
			Value:       serpent.Int64Of(&limit),
		},
	)
	formatter.AttachOptions(&cmd.Options)
	return cmd
}
