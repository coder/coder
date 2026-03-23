package cli

import (
	"fmt"
	"io"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

type chatMessageOutputRow struct {
	ID       int64                    `json:"-" table:"id,default_sort"`
	ChatID   string                   `json:"-" table:"chat id"`
	Delivery string                   `json:"-" table:"delivery"`
	Role     codersdk.ChatMessageRole `json:"-" table:"role"`
	Text     string                   `json:"-" table:"text"`
}

func readSendPrompt(inv *serpent.Invocation) (string, error) {
	if len(inv.Args) > 1 {
		return strings.Join(inv.Args[1:], " "), nil
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

func chatMessageOutputRowFromResponse(resp codersdk.CreateChatMessageResponse) (chatMessageOutputRow, error) {
	switch {
	case resp.Message != nil:
		return chatMessageOutputRow{
			ID:       resp.Message.ID,
			ChatID:   resp.Message.ChatID.String(),
			Delivery: "delivered",
			Role:     resp.Message.Role,
			Text:     chatMessageText(resp.Message.Content),
		}, nil
	case resp.QueuedMessage != nil:
		return chatMessageOutputRow{
			ID:       resp.QueuedMessage.ID,
			ChatID:   resp.QueuedMessage.ChatID.String(),
			Delivery: "queued",
			Role:     codersdk.ChatMessageRoleUser,
			Text:     chatMessageText(resp.QueuedMessage.Content),
		}, nil
	default:
		return chatMessageOutputRow{}, xerrors.New("chat message response did not include a message")
	}
}

func chatMessageResponseID(resp codersdk.CreateChatMessageResponse) (*int64, error) {
	switch {
	case resp.Message != nil:
		return &resp.Message.ID, nil
	case resp.QueuedMessage != nil:
		return &resp.QueuedMessage.ID, nil
	default:
		return nil, xerrors.New("chat message response did not include a message ID")
	}
}

func (r *RootCmd) chatsSend() *serpent.Command {
	var (
		modelFlag string
		follow    bool
		formatter = cliui.NewOutputFormatter(
			cliui.ChangeFormatterData(
				cliui.TableFormat([]chatMessageOutputRow{}, []string{"id", "delivery", "role", "text"}),
				func(data any) (any, error) {
					resp, ok := data.(codersdk.CreateChatMessageResponse)
					if !ok {
						return nil, xerrors.Errorf("expected codersdk.CreateChatMessageResponse, got %T", data)
					}

					row, err := chatMessageOutputRowFromResponse(resp)
					if err != nil {
						return nil, err
					}
					return []chatMessageOutputRow{row}, nil
				},
			),
			cliui.JSONFormat(),
		)
	)

	cmd := &serpent.Command{
		Use:        "send <chat-id> [prompt]",
		Short:      "Send a message to an existing chat.",
		Middleware: serpent.RequireRangeArgs(1, -1),
		Options: serpent.OptionSet{
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
				Description:   "Watch the chat after sending the message.",
				Value:         serpent.BoolOf(&follow),
			},
		},
		Handler: func(inv *serpent.Invocation) error {
			chatID, err := parseChatID(inv)
			if err != nil {
				return err
			}

			prompt, err := readSendPrompt(inv)
			if err != nil {
				return err
			}

			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			expClient := codersdk.NewExperimentalClient(client)

			ctx := inv.Context()
			modelID, err := resolveModel(ctx, expClient, modelFlag)
			if err != nil {
				return err
			}

			resp, err := expClient.CreateChatMessage(ctx, chatID, codersdk.CreateChatMessageRequest{
				Content:       promptToContent(prompt),
				ModelConfigID: modelID,
			})
			if err != nil {
				return err
			}

			if follow {
				afterID, err := chatMessageResponseID(resp)
				if err != nil {
					return err
				}

				return watchChat(
					ctx,
					expClient,
					chatID,
					afterID,
					chatWatchWriters{stdout: inv.Stdout, stderr: inv.Stderr},
					formatter.FormatID() == "json",
				)
			}

			out, err := formatter.Format(ctx, resp)
			if err != nil {
				return xerrors.Errorf("format chat message: %w", err)
			}

			_, err = fmt.Fprintln(inv.Stdout, out)
			return err
		},
	}

	formatter.AttachOptions(&cmd.Options)
	return cmd
}
