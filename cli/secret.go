package cli

import (
	"fmt"
	"io"
	"os"
	"runtime"
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
				Command:     "MYCLI_API_KEY=\"$NEW_SECRET_VALUE\" coder secret update api-key --value-env MYCLI_API_KEY --description \"Rotated API key\" --env API_KEY --file \"~/.api-key\"",
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
		value               string
		valueEnv            string
		description         string
		env                 string
		file                string
		trimTrailingNewline bool
	)

	cmd := &serpent.Command{
		Use:   "create <name>",
		Short: "Create a secret",
		Long:  "Provide the secret value with --value, --value-env, or non-interactive stdin (pipe or redirect).",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
		),
		Options: serpent.OptionSet{
			{
				Name:        "value",
				Flag:        "value",
				Description: "Set the secret value. For security reasons, prefer --value-env or non-interactive stdin (pipe or redirect).",
				Value:       serpent.StringOf(&value),
			},
			{
				Name:        "value-env",
				Flag:        "value-env",
				Description: "Read the secret value from the named environment variable.",
				Value:       serpent.StringOf(&valueEnv),
			},
			{
				Name:        "trim-trailing-newline",
				Flag:        "trim-trailing-newline",
				Description: "Trim a single trailing newline from stdin-provided secret values.",
				Value:       serpent.BoolOf(&trimTrailingNewline),
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
				Description: "Inject the secret into workspaces as an environment variable.",
				Value:       serpent.StringOf(&env),
			},
			{
				Name:        "file",
				Flag:        "file",
				Description: "Inject the secret into workspaces as a file.",
				Value:       serpent.StringOf(&file),
			},
		},
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			resolvedValue, ok, err := secretValue(inv, value, valueEnv, trimTrailingNewline)
			if err != nil {
				return err
			}
			if !ok {
				if isTTYIn(inv) {
					return xerrors.New("secret value must be provided with --value, --value-env, or stdin via pipe or redirect")
				}
				return xerrors.New("secret value must be provided by exactly one of --value, --value-env, or non-interactive stdin (pipe or redirect)")
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
		value               string
		valueEnv            string
		description         string
		env                 string
		file                string
		trimTrailingNewline bool
	)

	cmd := &serpent.Command{
		Use:   "update <name>",
		Short: "Update a secret",
		Long: strings.Join([]string{
			"At least one of --value, --value-env, --description, --env, or --file must be specified.",
			"Provide the secret value by at most one of --value, --value-env, or non-interactive stdin (pipe or redirect).",
		}, " "),
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
		),
		Options: serpent.OptionSet{
			{
				Name:        "value",
				Flag:        "value",
				Description: "Update the secret value. For security reasons, prefer --value-env or non-interactive stdin (pipe or redirect).",
				Value:       serpent.StringOf(&value),
			},
			{
				Name:        "value-env",
				Flag:        "value-env",
				Description: "Read the updated secret value from the named environment variable.",
				Value:       serpent.StringOf(&valueEnv),
			},
			{
				Name:        "trim-trailing-newline",
				Flag:        "trim-trailing-newline",
				Description: "Trim a single trailing newline from stdin-provided secret values.",
				Value:       serpent.BoolOf(&trimTrailingNewline),
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
				Description: "Update the environment variable injection target. Pass an empty string to clear it.",
				Value:       serpent.StringOf(&env),
			},
			{
				Name:        "file",
				Flag:        "file",
				Description: "Update the file injection target. Pass an empty string to clear it.",
				Value:       serpent.StringOf(&file),
			},
		},
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			req := codersdk.UpdateUserSecretRequest{}
			resolvedValue, ok, err := secretValue(inv, value, valueEnv, trimTrailingNewline)
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

var promptSecretTrimTrailingNewline = func(inv *serpent.Invocation) (bool, bool, error) {
	promptInv, cleanup, err := controllingTTYInvocation(inv)
	if err != nil {
		return false, false, nil
	}
	defer cleanup()

	warnTrailingNewline(promptInv.Stderr)
	answer, err := cliui.Prompt(promptInv, cliui.PromptOptions{
		Text:      "Trim the trailing newline from the stdin secret value?",
		IsConfirm: true,
		Default:   cliui.ConfirmNo,
	})
	if err != nil {
		return false, true, err
	}

	return answer == cliui.ConfirmYes, true, nil
}

//nolint:revive // The bool mirrors the CLI flag; avoiding it would add indirection without improving this flow.
func secretValue(inv *serpent.Invocation, value string, valueEnv string, trimTrailingNewline bool) (string, bool, error) {
	valueSource := optionValueSource(inv, "value")
	valueEnvSource := optionValueSource(inv, "value-env")
	stdinValue, stdinProvided, err := readInvocationStdin(inv)
	if err != nil {
		return "", false, err
	}

	sourceNames := make([]string, 0, 3)
	if valueSource != serpent.ValueSourceNone && valueSource != serpent.ValueSourceDefault {
		sourceNames = append(sourceNames, "--value")
	}
	if valueEnvSource != serpent.ValueSourceNone && valueEnvSource != serpent.ValueSourceDefault {
		sourceNames = append(sourceNames, "--value-env")
	}
	if stdinProvided {
		sourceNames = append(sourceNames, "stdin")
	}
	if len(sourceNames) > 1 {
		return "", false, xerrors.Errorf("secret value may be provided by only one source, got %s", strings.Join(sourceNames, ", "))
	}

	if valueSource != serpent.ValueSourceNone && valueSource != serpent.ValueSourceDefault {
		return value, true, nil
	}

	if valueEnvSource != serpent.ValueSourceNone && valueEnvSource != serpent.ValueSourceDefault {
		if valueEnv == "" {
			return "", false, xerrors.New("environment variable name must be provided with --value-env")
		}

		envValue, ok := os.LookupEnv(valueEnv)
		if !ok {
			return "", false, xerrors.Errorf("environment variable %q is not set", valueEnv)
		}

		return envValue, true, nil
	}

	if stdinProvided {
		resolvedStdinValue, err := resolveInvocationStdinValue(inv, stdinValue, trimTrailingNewline)
		if err != nil {
			return "", false, err
		}
		return resolvedStdinValue, true, nil
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

//nolint:revive // The bool directly reflects the CLI flag and keeps the helper simple.
func resolveInvocationStdinValue(inv *serpent.Invocation, value string, trimTrailingNewline bool) (string, error) {
	trimmedValue, suspiciousTrailingNewline := trimSingleTrailingNewline(value)
	if suspiciousTrailingNewline {
		if trimTrailingNewline {
			return trimmedValue, nil
		}

		trim, prompted, err := promptSecretTrimTrailingNewline(inv)
		if err != nil {
			return "", err
		}
		if trim {
			return trimmedValue, nil
		}
		if !prompted {
			warnTrailingNewline(inv.Stderr)
		}
	}

	return value, nil
}

func optionValueSource(inv *serpent.Invocation, name string) serpent.ValueSource {
	opt := inv.Command.Options.ByName(name)
	if opt == nil {
		return serpent.ValueSourceNone
	}
	return opt.ValueSource
}

// Shell helpers like echo usually append a line ending to piped stdin. We
// treat a single trailing LF or CRLF as suspicious, but avoid flagging values
// that are clearly multiline.
func trimSingleTrailingNewline(value string) (string, bool) {
	switch {
	case strings.HasSuffix(value, "\r\n"):
		trimmed := strings.TrimSuffix(value, "\r\n")
		return trimmed, !strings.ContainsAny(trimmed, "\r\n")
	case strings.HasSuffix(value, "\n"):
		trimmed := strings.TrimSuffix(value, "\n")
		return trimmed, !strings.ContainsAny(trimmed, "\r\n")
	case strings.HasSuffix(value, "\r"):
		trimmed := strings.TrimSuffix(value, "\r")
		return trimmed, !strings.ContainsAny(trimmed, "\r\n")
	default:
		return value, false
	}
}

func warnTrailingNewline(w io.Writer) {
	cliui.Warn(
		w,
		"stdin ends with a trailing newline.",
		"Using echo often appends an unintended newline to the secret value.",
		"Use printf %s or rerun with --trim-trailing-newline to remove it.",
	)
}

func controllingTTYInvocation(inv *serpent.Invocation) (*serpent.Invocation, func(), error) {
	inPath, outPath := controllingTTYPaths()
	inFile, err := os.OpenFile(inPath, os.O_RDWR, 0)
	if err != nil {
		return nil, nil, err
	}
	outFile, err := os.OpenFile(outPath, os.O_RDWR, 0)
	if err != nil {
		_ = inFile.Close()
		return nil, nil, err
	}

	promptInv := *inv
	promptInv.Stdin = inFile
	promptInv.Stdout = outFile
	promptInv.Stderr = outFile

	return &promptInv, func() {
		_ = inFile.Close()
		_ = outFile.Close()
	}, nil
}

func controllingTTYPaths() (inputPath string, outputPath string) {
	if runtime.GOOS == "windows" {
		return "CONIN$", "CONOUT$"
	}
	return "/dev/tty", "/dev/tty"
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
