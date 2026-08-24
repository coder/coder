package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/dustin/go-humanize"
	"github.com/dustin/go-humanize/english"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/coderd/util/ptr"
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
				Description: "Import secrets from a file",
				Command:     "coder secret import ./secrets.env",
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
			r.secretImport(),
			r.secretEnable(),
			r.secretDisable(),
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
		enabled     bool
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
				Description: "Workspace file path where this secret will be written. Must start with ~/ or /. Deployment administrators can turn off file path delivery.",
				Value:       serpent.StringOf(&file),
			},
			{
				Name:        "enabled",
				Flag:        "enabled",
				Description: "Whether the secret is eligible for injection into workspaces. An enabled secret must set an allowed target; pass --enabled=false to store a secret without injecting it.",
				Default:     "true",
				Value:       serpent.BoolOf(&enabled),
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

			req := codersdk.CreateUserSecretRequest{
				Name:        inv.Args[0],
				Value:       resolvedValue,
				Description: description,
				EnvName:     env,
				FilePath:    file,
			}
			if userSetOption(inv, "enabled") {
				req.Enabled = ptr.Ref(enabled)
			}

			secret, err := client.CreateUserSecret(inv.Context(), codersdk.Me, req)
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
		enabled     bool
	)

	cmd := &serpent.Command{
		Use:   "update <name>",
		Short: "Update a secret",
		Long: strings.Join([]string{
			"At least one of --value, --description, --env, --file, or --enabled must be specified.",
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
				Description: "Workspace file path where this secret will be written. Must start with ~/ or /. Deployment administrators can turn off file path delivery. Pass an empty string to clear a stored path.",
				Value:       serpent.StringOf(&file),
			},
			{
				Name:        "enabled",
				Flag:        "enabled",
				Description: "Whether the secret is eligible for injection into workspaces. An enabled secret must keep an allowed target; pass --enabled=false to stop injecting it without deleting it.",
				Value:       serpent.BoolOf(&enabled),
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
			if userSetOption(inv, "enabled") {
				req.Enabled = ptr.Ref(enabled)
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

var secretsFileFormats = []string{
	string(codersdk.SecretsFileFormatEnv),
	string(codersdk.SecretsFileFormatJSON),
	string(codersdk.SecretsFileFormatYAML),
}

func (r *RootCmd) secretImport() *serpent.Command {
	var inputFormat string

	cmd := &serpent.Command{
		Use:   "import <file>",
		Short: "Import secrets from a file",
		Long: strings.Join([]string{
			"Every key in the file becomes a secret.",
			"Keys allowed as environment variable names are injected into workspaces under the same name.",
			"The import is all or nothing, and existing secrets are never overwritten.",
			"Pass - to read the file from non-interactive stdin (pipe or redirect).",
		}, " "),
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
		),
		Options: serpent.OptionSet{
			{
				Name:        "input-format",
				Flag:        "input-format",
				Description: "Format of the secrets file. Inferred from the file extension when unset, and required when reading from stdin.",
				Value:       serpent.EnumOf(&inputFormat, secretsFileFormats...),
			},
		},
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			path := inv.Args[0]
			// serpent.EnumOf matches case-insensitively but keeps the input
			// verbatim, and the parser only accepts lowercase formats.
			format := codersdk.SecretsFileFormat(strings.ToLower(inputFormat))
			if format == "" {
				format, err = secretsFileFormatFromPath(path)
				if err != nil {
					return err
				}
			}

			content, err := readSecretsFile(inv, path)
			if err != nil {
				return err
			}
			// Parse and validate before sending so that a file picked by mistake,
			// such as a private key, never leaves the machine.
			requests, err := codersdk.ParseSecretsFile(format, string(content))
			if err != nil {
				return xerrors.Errorf("parse %q: %w", path, err)
			}
			if err := validateImportedSecrets(requests); err != nil {
				return xerrors.Errorf("validate %q: %w", path, err)
			}

			secrets, err := client.ImportUserSecrets(inv.Context(), codersdk.Me, codersdk.ImportUserSecretsRequest{
				Format:  format,
				Content: string(content),
			})
			if err != nil {
				return xerrors.Errorf("import secrets from %q: %w", path, err)
			}

			_, _ = fmt.Fprintf(inv.Stdout, "Imported %s.\n", english.Plural(len(secrets), "secret", ""))
			warnSecretsWithoutEnvName(inv.Stderr, secrets)
			return nil
		},
	}

	return cmd
}

// secretsFileFormatFromPath infers the format from the file extension.
// Extensions that do not map to a format, such as ".env.local", require
// --input-format.
func secretsFileFormatFromPath(path string) (codersdk.SecretsFileFormat, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".env":
		return codersdk.SecretsFileFormatEnv, nil
	case ".json":
		return codersdk.SecretsFileFormatJSON, nil
	case ".yaml", ".yml":
		return codersdk.SecretsFileFormatYAML, nil
	default:
		return "", xerrors.Errorf("cannot infer the secrets file format from %q, set --input-format to one of: %s", path, strings.Join(secretsFileFormats, ", "))
	}
}

// readSecretsFile reads the file at path, or stdin when path is "-". It never
// reads more than one byte past the limit the server accepts.
func readSecretsFile(inv *serpent.Invocation, path string) ([]byte, error) {
	reader := inv.Stdin
	if path == "-" && isTTYIn(inv) {
		return nil, xerrors.New("secrets file must be provided via non-interactive stdin (pipe or redirect)")
	}
	if path != "-" {
		file, err := os.Open(path)
		if err != nil {
			return nil, xerrors.Errorf("open secrets file: %w", err)
		}
		defer file.Close()
		reader = file
	}

	content, err := io.ReadAll(io.LimitReader(reader, codersdk.MaxSecretsFileBytes+1))
	if err != nil {
		return nil, xerrors.Errorf("read secrets file: %w", err)
	}
	if len(content) > codersdk.MaxSecretsFileBytes {
		return nil, xerrors.Errorf("secrets file exceeds the maximum allowed size of %d bytes", codersdk.MaxSecretsFileBytes)
	}
	if !utf8.Valid(content) {
		return nil, xerrors.New("secrets file must contain valid UTF-8")
	}
	return content, nil
}

func validateImportedSecrets(requests []codersdk.CreateUserSecretRequest) error {
	var validationErrors []string
	for i, request := range requests {
		for _, validation := range codersdk.ValidateCreateUserSecretRequest(request) {
			validationErrors = append(validationErrors, fmt.Sprintf("secret %d (%q) %s: %s", i+1, request.Name, validation.Field, validation.Detail))
		}
	}
	if len(validationErrors) > 0 {
		return xerrors.New(strings.Join(validationErrors, "; "))
	}
	return nil
}

// warnSecretsWithoutEnvName reports imported secrets whose key is not a valid
// environment variable name. They are stored with an empty env name and are
// never injected into workspaces until one is set.
func warnSecretsWithoutEnvName(w io.Writer, secrets []codersdk.UserSecret) {
	names := make([]string, 0, len(secrets))
	for _, secret := range secrets {
		if secret.EnvName == "" {
			names = append(names, strconv.Quote(secret.Name))
		}
	}
	if len(names) == 0 {
		return
	}

	cliui.Warn(w,
		fmt.Sprintf("%s imported without an environment variable name: %s",
			english.Plural(len(names), "secret", ""), strings.Join(names, ", ")),
		"Set each with `coder secret update <name> --env <ENV_NAME>` to inject it into workspaces.",
	)
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

	Created       string `json:"-" table:"created"`
	Name          string `json:"-" table:"name,default_sort"`
	Updated       string `json:"-" table:"updated"`
	Env           string `json:"-" table:"env"`
	File          string `json:"-" table:"file"`
	EnabledIntent string `json:"-" table:"enabled"`
	Description   string `json:"-" table:"description"`
}

func secretListRowFromSecret(secret codersdk.UserSecret) secretListRow {
	return secretListRow{
		UserSecret:    secret,
		Created:       humanize.Time(secret.CreatedAt),
		Name:          secret.Name,
		Updated:       humanize.Time(secret.UpdatedAt),
		Env:           secret.EnvName,
		File:          secret.FilePath,
		EnabledIntent: fmt.Sprintf("%t (stored intent)", secret.Enabled),
		Description:   secret.Description,
	}
}

func (r *RootCmd) secretEnable() *serpent.Command {
	return r.secretEnabledSetter(secretEnabledStateEnabled)
}

func (r *RootCmd) secretDisable() *serpent.Command {
	return r.secretEnabledSetter(secretEnabledStateDisabled)
}

// secretEnabledState distinguishes the two `coder secret enable` and
// `coder secret disable` subcommands without using a bare bool, which
// revive's flag-parameter rule treats as a control coupling.
type secretEnabledState int

const (
	secretEnabledStateEnabled secretEnabledState = iota
	secretEnabledStateDisabled
)

// secretEnabledSetter builds the `coder secret enable` and `coder secret
// disable` subcommands. Both are a one-field PATCH that flips the enabled
// state. Disabling stops injection for new sessions but leaves the secret
// in place so it can be re-enabled later; existing sessions keep injected
// values until the workspace's agent manifest is refetched.
func (r *RootCmd) secretEnabledSetter(state secretEnabledState) *serpent.Command {
	var (
		verb       string
		participle string
		short      string
		enabled    bool
	)
	switch state {
	case secretEnabledStateEnabled:
		verb = "enable"
		participle = "Enabled"
		short = "Mark a secret enabled for its allowed injection targets"
		enabled = true
	case secretEnabledStateDisabled:
		verb = "disable"
		participle = "Disabled"
		short = "Disable a secret without removing it"
		enabled = false
	}

	cmd := &serpent.Command{
		Use:   fmt.Sprintf("%s <name>", verb),
		Short: short,
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
		),
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			name := inv.Args[0]
			secret, err := client.UpdateUserSecret(inv.Context(), codersdk.Me, name, codersdk.UpdateUserSecretRequest{
				Enabled: ptr.Ref(enabled),
			})
			if err != nil {
				return xerrors.Errorf("%s secret %q: %w", verb, name, err)
			}

			_, _ = fmt.Fprintf(inv.Stdout, "%s secret %s.\n", participle, cliui.Keyword(secret.Name))
			return nil
		},
	}

	return cmd
}

func (r *RootCmd) secretList() *serpent.Command {
	formatter := cliui.NewOutputFormatter(
		cliui.ChangeFormatterData(
			cliui.TableFormat(
				[]secretListRow{},
				[]string{"name", "created", "updated", "env", "file", "enabled", "description"},
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
		Long:       "Secret values are omitted from the output. Enabled is stored intent; deployment policy can block file path delivery.",
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
