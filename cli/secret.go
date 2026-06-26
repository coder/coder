package cli

import (
	"fmt"
	"io"
	"strings"
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
		Short:   "Manage secrets",
		Long: FormatExamples(
			Example{
				Description: "Create a secret",
				Command:     "printf %s \"$MYCLI_API_KEY\" | coder secret create api-key --description \"API key for workspace tools\" --env API_KEY --file \"~/.api-key\"",
			},
			Example{
				Description: "Update a secret",
				Command:     "echo -n \"$NEW_SECRET_VALUE\" | coder secret update api-key --description \"Rotated API key\" --env API_KEY --file \"~/.api-key\"",
			},
			Example{
				Description: "List your secrets",
				Command:     "coder secret list",
			},
			Example{
				Description: "Show a specific secret",
				Command:     "coder secret list api-key",
			},
			Example{
				Description: "Delete a secret",
				Command:     "coder secret delete api-key",
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
		env         string
		file        string
	)

	cmd := &serpent.Command{
		Use:   "create <name>",
		Short: "Create a secret",
		Long:  "Provide the secret value with --value or non-interactive stdin (pipe or redirect).",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
		),
		Options: serpent.OptionSet{
			{
				Name:        "value",
				Flag:        "value",
				Description: "Set the secret value. For security reasons, prefer non-interactive stdin (pipe or redirect).",
				Value:       serpent.StringOf(&value),
			},
			{
				Name:        "description",
				Flag:        "description",
				Description: "Set the secret description.",
				Value:       serpent.StringOf(&description),
			},
			{
				Name:        "env",
				Flag:        "env",
				Description: "Name of the workspace environment variable that this secret will set.",
				Value:       serpent.StringOf(&env),
			},
			{
				Name:        "file",
				Flag:        "file",
				Description: "Workspace file path where this secret will be written. Must start with ~/ or /.",
				Value:       serpent.StringOf(&file),
			},
		},
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			resolvedValue, ok, err := secretValue(inv, value)
			if err != nil {
				return err
			}
			if !ok {
				if isTTYIn(inv) {
					return xerrors.New("secret value must be provided with --value or stdin via pipe or redirect")
				}
				return xerrors.New("secret value must be provided by exactly one of --value or non-interactive stdin (pipe or redirect)")
			}

			secret, err := client.CreateUserSecret(inv.Context(), codersdk.Me, codersdk.CreateUserSecretRequest{
				Name:        inv.Args[0],
				Value:       resolvedValue,
				Description: description,
				EnvName:     env,
				FilePath:    file,
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
		env         string
		file        string
	)

	cmd := &serpent.Command{
		Use:   "update <name>",
		Short: "Update a secret",
		Long: strings.Join([]string{
			"At least one of --value, --description, --env, or --file must be specified.",
			"Provide the secret value by at most one of --value or non-interactive stdin (pipe or redirect).",
		}, " "),
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
		),
		Options: serpent.OptionSet{
			{
				Name:        "value",
				Flag:        "value",
				Description: "Update the secret value. For security reasons, prefer non-interactive stdin (pipe or redirect).",
				Value:       serpent.StringOf(&value),
			},
			{
				Name:        "description",
				Flag:        "description",
				Description: "Update the secret description. Pass an empty string to clear it.",
				Value:       serpent.StringOf(&description),
			},
			{
				Name:        "env",
				Flag:        "env",
				Description: "Name of the workspace environment variable that this secret will set. Pass an empty string to clear it.",
				Value:       serpent.StringOf(&env),
			},
			{
				Name:        "file",
				Flag:        "file",
				Description: "Workspace file path where this secret will be written. Must start with ~/ or /. Pass an empty string to clear it.",
				Value:       serpent.StringOf(&file),
			},
		},
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			req := codersdk.UpdateUserSecretRequest{}
			resolvedValue, ok, err := secretValue(inv, value)
			if err != nil {
				return err
			}
			if ok {
				req.Value = &resolvedValue
			}
			if userSetOption(inv, "description") {
				req.Description = &description
			}
			if userSetOption(inv, "env") {
				req.EnvName = &env
			}
			if userSetOption(inv, "file") {
				req.FilePath = &file
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

func secretValue(inv *serpent.Invocation, value string) (string, bool, error) {
	valueProvided := userSetOption(inv, "value")
	stdinValue, stdinProvided, err := readInvocationStdin(inv)
	if err != nil {
		return "", false, err
	}

	sourceNames := make([]string, 0, 2)
	if valueProvided {
		sourceNames = append(sourceNames, "--value")
	}
	if stdinProvided {
		sourceNames = append(sourceNames, "stdin")
	}
	if len(sourceNames) > 1 {
		return "", false, xerrors.Errorf("secret value may be provided by only one source, got %s", strings.Join(sourceNames, ", "))
	}

	if valueProvided {
		return value, true, nil
	}

	if stdinProvided {
		warnSuspiciousTrailingNewline(inv.Stderr, stdinValue)
		return stdinValue, true, nil
	}

	return "", false, nil
}

func readInvocationStdin(inv *serpent.Invocation) (string, bool, error) {
	if isTTYIn(inv) {
		return "", false, nil
	}

	bytes, err := io.ReadAll(inv.Stdin)
	if err != nil {
		return "", false, xerrors.Errorf("reading stdin: %w", err)
	}
	if len(bytes) == 0 {
		return "", false, nil
	}

	return string(bytes), true, nil
}

// Shell helpers like echo usually append a line ending to piped stdin. We
// treat a single trailing LF or CRLF as suspicious, but avoid flagging values
// that are clearly multiline.
func hasSuspiciousTrailingNewline(value string) bool {
	switch {
	case strings.HasSuffix(value, "\r\n"):
		trimmed := strings.TrimSuffix(value, "\r\n")
		return !strings.ContainsAny(trimmed, "\r\n")
	case strings.HasSuffix(value, "\n"):
		trimmed := strings.TrimSuffix(value, "\n")
		return !strings.ContainsAny(trimmed, "\r\n")
	case strings.HasSuffix(value, "\r"):
		trimmed := strings.TrimSuffix(value, "\r")
		return !strings.ContainsAny(trimmed, "\r\n")
	default:
		return false
	}
}

func warnSuspiciousTrailingNewline(w io.Writer, value string) {
	if !hasSuspiciousTrailingNewline(value) {
		return
	}

	cliui.Warn(w, "secret value from stdin ends with a trailing newline")
}

type secretListRow struct {
	codersdk.UserSecret `table:"-"`

	Created     string `json:"-" table:"created"`
	Name        string `json:"-" table:"name,default_sort"`
	Updated     string `json:"-" table:"updated"`
	Env         string `json:"-" table:"env"`
	File        string `json:"-" table:"file"`
	Description string `json:"-" table:"description"`
}

func secretListRowFromSecret(secret codersdk.UserSecret) secretListRow {
	return secretListRow{
		UserSecret:  secret,
		Created:     humanize.Time(secret.CreatedAt),
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
				[]string{"name", "created", "updated", "env", "file", "description"},
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
		Long:       "Secret values are omitted from the output.",
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
		Aliases: []string{"remove", "rm"},
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
