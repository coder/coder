package cli

import (
	"fmt"
	"time"

	"github.com/dustin/go-humanize"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/pretty"
	"github.com/coder/serpent"
)

func (r *RootCmd) secrets() *serpent.Command {
	cmd := &serpent.Command{
		Use:     "secret",
		Aliases: []string{"secrets"},
		Short:   "Manage personal secrets",
		Long: FormatExamples(
			Example{
				Description: "Create a secret",
				Command:     "coder secret create openai-key --value \"$SECRET_VALUE\" --description \"Personal OPENAI_API key\" --inject-env OPEN_AI_KEY --inject-file \"~/.openai-key\"",
			},
			Example{
				Description: "Update a secret",
				Command:     "coder secret update openai-key --value \"$NEW_SECRET_VALUE\" --description \"Updated description\" --inject-env NEW_ENV_NAME --inject-file \"~/.new-path\"",
			},
			Example{
				Description: "List your secrets",
				Command:     "coder secret list",
			},
			Example{
				Description: "Show a specific secret",
				Command:     "coder secret list openai-key",
			},
			Example{
				Description: "Delete a secret",
				Command:     "coder secret delete openai-key",
			},
		),
		Handler: func(inv *serpent.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Children: []*serpent.Command{
			r.secretCreate(),
			r.secretUpdate(),
			r.secretList(),
			r.secretDelete(),
		},
	}

	return cmd
}

func (r *RootCmd) secretCreate() *serpent.Command {
	var (
		value       string
		description string
		injectEnv   string
		injectFile  string
	)

	cmd := &serpent.Command{
		Use:   "create <name>",
		Short: "Create a secret",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
		),
		Options: serpent.OptionSet{
			{
				Name:        "value",
				Flag:        "value",
				Description: "Set the secret value. This flag is required.",
				Value:       serpent.StringOf(&value),
				Required:    true,
			},
			{
				Name:        "description",
				Flag:        "description",
				Description: "Set the secret description.",
				Value:       serpent.StringOf(&description),
			},
			{
				Name:        "inject-env",
				Flag:        "inject-env",
				Description: "Inject the secret into workspaces as an environment variable.",
				Value:       serpent.StringOf(&injectEnv),
			},
			{
				Name:        "inject-file",
				Flag:        "inject-file",
				Description: "Inject the secret into workspaces as a file.",
				Value:       serpent.StringOf(&injectFile),
			},
		},
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			secret, err := client.CreateUserSecret(inv.Context(), codersdk.Me, codersdk.CreateUserSecretRequest{
				Name:        inv.Args[0],
				Value:       value,
				Description: description,
				EnvName:     injectEnv,
				FilePath:    injectFile,
			})
			if err != nil {
				return xerrors.Errorf("create secret %q: %w", inv.Args[0], err)
			}

			_, _ = fmt.Fprintf(inv.Stdout, "Created secret %s.\n", cliui.Keyword(secret.Name))
			return nil
		},
	}

	return cmd
}

func (r *RootCmd) secretUpdate() *serpent.Command {
	var (
		value       string
		description string
		injectEnv   string
		injectFile  string
	)

	cmd := &serpent.Command{
		Use:   "update <name>",
		Short: "Update a secret",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
		),
		Options: serpent.OptionSet{
			{
				Name:        "value",
				Flag:        "value",
				Description: "Update the secret value.",
				Value:       serpent.StringOf(&value),
			},
			{
				Name:        "description",
				Flag:        "description",
				Description: "Update the secret description. Pass an empty string to clear it.",
				Value:       serpent.StringOf(&description),
			},
			{
				Name:        "inject-env",
				Flag:        "inject-env",
				Description: "Update the environment variable injection target. Pass an empty string to clear it.",
				Value:       serpent.StringOf(&injectEnv),
			},
			{
				Name:        "inject-file",
				Flag:        "inject-file",
				Description: "Update the file injection target. Pass an empty string to clear it.",
				Value:       serpent.StringOf(&injectFile),
			},
		},
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			req := codersdk.UpdateUserSecretRequest{}
			if userSetOption(inv, "value") {
				req.Value = &value
			}
			if userSetOption(inv, "description") {
				req.Description = &description
			}
			if userSetOption(inv, "inject-env") {
				req.EnvName = &injectEnv
			}
			if userSetOption(inv, "inject-file") {
				req.FilePath = &injectFile
			}

			secret, err := client.UpdateUserSecret(inv.Context(), codersdk.Me, inv.Args[0], req)
			if err != nil {
				return xerrors.Errorf("update secret %q: %w", inv.Args[0], err)
			}

			_, _ = fmt.Fprintf(inv.Stdout, "Updated secret %s.\n", cliui.Keyword(secret.Name))
			return nil
		},
	}

	return cmd
}

type secretListRow struct {
	codersdk.UserSecret `table:"-"`

	Name        string `json:"-" table:"name,default_sort"`
	Updated     string `json:"-" table:"updated"`
	Env         string `json:"-" table:"env"`
	File        string `json:"-" table:"file"`
	Description string `json:"-" table:"description"`
}

func secretListRowFromSecret(secret codersdk.UserSecret) secretListRow {
	return secretListRow{
		UserSecret:  secret,
		Name:        secret.Name,
		Updated:     humanize.Time(secret.UpdatedAt),
		Env:         secret.EnvName,
		File:        secret.FilePath,
		Description: secret.Description,
	}
}

func (r *RootCmd) secretList() *serpent.Command {
	formatter := cliui.NewOutputFormatter(
		cliui.ChangeFormatterData(
			cliui.TableFormat(
				[]secretListRow{},
				[]string{"name", "updated", "env", "file", "description"},
			),
			func(data any) (any, error) {
				switch rows := data.(type) {
				case []secretListRow:
					return rows, nil
				case secretListRow:
					return []secretListRow{rows}, nil
				default:
					return nil, xerrors.Errorf("expected []secretListRow or secretListRow, got %T", data)
				}
			},
		),
		cliui.ChangeFormatterData(
			cliui.JSONFormat(),
			func(data any) (any, error) {
				switch rows := data.(type) {
				case []secretListRow:
					secrets := make([]codersdk.UserSecret, len(rows))
					for i := range rows {
						secrets[i] = rows[i].UserSecret
					}
					return secrets, nil
				case secretListRow:
					return []codersdk.UserSecret{rows.UserSecret}, nil
				default:
					return nil, xerrors.Errorf("expected []secretListRow or secretListRow, got %T", data)
				}
			},
		),
	)

	cmd := &serpent.Command{
		Use:        "list [name]",
		Aliases:    []string{"ls"},
		Short:      "List secrets, or show one by name",
		Middleware: serpent.RequireRangeArgs(0, 1),
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			var data any
			if len(inv.Args) == 1 {
				secret, err := client.UserSecretByName(inv.Context(), codersdk.Me, inv.Args[0])
				if err != nil {
					return xerrors.Errorf("get secret %q: %w", inv.Args[0], err)
				}
				data = secretListRowFromSecret(secret)
			} else {
				secrets, err := client.UserSecrets(inv.Context(), codersdk.Me)
				if err != nil {
					return xerrors.Errorf("list secrets: %w", err)
				}

				rows := make([]secretListRow, len(secrets))
				for i := range secrets {
					rows[i] = secretListRowFromSecret(secrets[i])
				}
				data = rows
			}

			out, err := formatter.Format(inv.Context(), data)
			if err != nil {
				return xerrors.Errorf("format secrets: %w", err)
			}
			if out == "" {
				cliui.Infof(inv.Stderr, "No secrets found.")
				return nil
			}

			_, err = fmt.Fprintln(inv.Stdout, out)
			return err
		},
	}

	formatter.AttachOptions(&cmd.Options)
	return cmd
}

func (r *RootCmd) secretDelete() *serpent.Command {
	cmd := &serpent.Command{
		Use:     "delete <name>",
		Aliases: []string{"remove"},
		Short:   "Delete a secret",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
		),
		Options: serpent.OptionSet{
			cliui.SkipPromptOption(),
		},
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			name := inv.Args[0]
			_, err = cliui.Prompt(inv, cliui.PromptOptions{
				Text:      fmt.Sprintf("Delete secret %s?", pretty.Sprint(cliui.DefaultStyles.Code, name)),
				IsConfirm: true,
				Default:   cliui.ConfirmNo,
			})
			if err != nil {
				return err
			}

			if err = client.DeleteUserSecret(inv.Context(), codersdk.Me, name); err != nil {
				return xerrors.Errorf("delete secret %q: %w", name, err)
			}

			_, _ = fmt.Fprintf(inv.Stdout, "Deleted secret %s at %s.\n", cliui.Keyword(name), cliui.Timestamp(time.Now()))
			return nil
		},
	}

	return cmd
}
