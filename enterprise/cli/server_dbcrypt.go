//go:build !slim
package cli
import (
	"errors"
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/coder/coder/v2/cli"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/coderd/database/awsiamrds"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/dbcrypt"
	"github.com/coder/serpent"
)
func (r *RootCmd) dbcryptCmd() *serpent.Command {
	dbcryptCmd := &serpent.Command{
		Use:   "dbcrypt",
		Short: "Manage database encryption.",
		Handler: func(inv *serpent.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
	}
	dbcryptCmd.AddSubcommands(
		r.dbcryptDecryptCmd(),
		r.dbcryptDeleteCmd(),
		r.dbcryptRotateCmd(),
	)
	return dbcryptCmd
}
func (*RootCmd) dbcryptRotateCmd() *serpent.Command {
	var flags rotateFlags
	cmd := &serpent.Command{
		Use:   "rotate",
		Short: "Rotate database encryption keys.",
		Handler: func(inv *serpent.Invocation) error {
			ctx, cancel := context.WithCancel(inv.Context())
			defer cancel()
			logger := slog.Make(sloghuman.Sink(inv.Stdout))
			if ok, _ := inv.ParsedFlags().GetBool("verbose"); ok {
				logger = logger.Leveled(slog.LevelDebug)
			}
			if err := flags.valid(); err != nil {
				return err
			}
			ks := [][]byte{}
			dk, err := base64.StdEncoding.DecodeString(flags.New)
			if err != nil {
				return fmt.Errorf("decode new key: %w", err)
			}
			ks = append(ks, dk)
			for _, k := range flags.Old {
				dk, err := base64.StdEncoding.DecodeString(k)
				if err != nil {
					return fmt.Errorf("decode old key: %w", err)
				}
				ks = append(ks, dk)
			}
			ciphers, err := dbcrypt.NewCiphers(ks...)
			if err != nil {
				return fmt.Errorf("create ciphers: %w", err)
			}
			var act string
			switch len(flags.Old) {
			case 0:
				act = "Data will be encrypted with the new key."
			default:
				act = "Data will be decrypted with all available keys and re-encrypted with new key."
			}
			msg := fmt.Sprintf("%s\n\n- New key: %s\n- Old keys: %s\n\nRotate external token encryption keys?\n",
				act,
				flags.New,
				strings.Join(flags.Old, ", "),
			)
			if _, err := cliui.Prompt(inv, cliui.PromptOptions{Text: msg, IsConfirm: true}); err != nil {
				return err
			}
			sqlDriver := "postgres"
			if codersdk.PostgresAuth(flags.PostgresAuth) == codersdk.PostgresAuthAWSIAMRDS {
				sqlDriver, err = awsiamrds.Register(inv.Context(), sqlDriver)
				if err != nil {
					return fmt.Errorf("register aws rds iam auth: %w", err)
				}
			}
			sqlDB, err := cli.ConnectToPostgres(inv.Context(), logger, sqlDriver, flags.PostgresURL, nil)
			if err != nil {
				return fmt.Errorf("connect to postgres: %w", err)
			}
			defer func() {
				_ = sqlDB.Close()
			}()
			logger.Info(ctx, "connected to postgres")
			if err := dbcrypt.Rotate(ctx, logger, sqlDB, ciphers); err != nil {
				return fmt.Errorf("rotate ciphers: %w", err)
			}
			logger.Info(ctx, "operation completed successfully")
			return nil
		},
	}
	flags.attach(&cmd.Options)
	return cmd
}
func (*RootCmd) dbcryptDecryptCmd() *serpent.Command {
	var flags decryptFlags
	cmd := &serpent.Command{
		Use:   "decrypt",
		Short: "Decrypt a previously encrypted database.",
		Handler: func(inv *serpent.Invocation) error {
			ctx, cancel := context.WithCancel(inv.Context())
			defer cancel()
			logger := slog.Make(sloghuman.Sink(inv.Stdout))
			if ok, _ := inv.ParsedFlags().GetBool("verbose"); ok {
				logger = logger.Leveled(slog.LevelDebug)
			}
			if err := flags.valid(); err != nil {
				return err
			}
			ks := make([][]byte, 0, len(flags.Keys))
			for _, k := range flags.Keys {
				dk, err := base64.StdEncoding.DecodeString(k)
				if err != nil {
					return fmt.Errorf("decode key: %w", err)
				}
				ks = append(ks, dk)
			}
			ciphers, err := dbcrypt.NewCiphers(ks...)
			if err != nil {
				return fmt.Errorf("create ciphers: %w", err)
			}
			if _, err := cliui.Prompt(inv, cliui.PromptOptions{
				Text:      "This will decrypt all encrypted data in the database. Are you sure you want to continue?",
				IsConfirm: true,
			}); err != nil {
				return err
			}
			sqlDriver := "postgres"
			if codersdk.PostgresAuth(flags.PostgresAuth) == codersdk.PostgresAuthAWSIAMRDS {
				sqlDriver, err = awsiamrds.Register(inv.Context(), sqlDriver)
				if err != nil {
					return fmt.Errorf("register aws rds iam auth: %w", err)
				}
			}
			sqlDB, err := cli.ConnectToPostgres(inv.Context(), logger, sqlDriver, flags.PostgresURL, nil)
			if err != nil {
				return fmt.Errorf("connect to postgres: %w", err)
			}
			defer func() {
				_ = sqlDB.Close()
			}()
			logger.Info(ctx, "connected to postgres")
			if err := dbcrypt.Decrypt(ctx, logger, sqlDB, ciphers); err != nil {
				return fmt.Errorf("rotate ciphers: %w", err)
			}
			logger.Info(ctx, "operation completed successfully")
			return nil
		},
	}
	flags.attach(&cmd.Options)
	return cmd
}
func (*RootCmd) dbcryptDeleteCmd() *serpent.Command {
	var flags deleteFlags
	cmd := &serpent.Command{
		Use:   "delete",
		Short: "Delete all encrypted data from the database. THIS IS A DESTRUCTIVE OPERATION.",
		Handler: func(inv *serpent.Invocation) error {
			ctx, cancel := context.WithCancel(inv.Context())
			defer cancel()
			logger := slog.Make(sloghuman.Sink(inv.Stdout))
			if ok, _ := inv.ParsedFlags().GetBool("verbose"); ok {
				logger = logger.Leveled(slog.LevelDebug)
			}
			if err := flags.valid(); err != nil {
				return err
			}
			msg := `All encrypted data will be deleted from the database:
- Encrypted user OAuth access and refresh tokens
- Encrypted user Git authentication access and refresh tokens
Are you sure you want to continue?`
			if _, err := cliui.Prompt(inv, cliui.PromptOptions{
				Text:      msg,
				IsConfirm: true,
			}); err != nil {
				return err
			}
			var err error
			sqlDriver := "postgres"
			if codersdk.PostgresAuth(flags.PostgresAuth) == codersdk.PostgresAuthAWSIAMRDS {
				sqlDriver, err = awsiamrds.Register(inv.Context(), sqlDriver)
				if err != nil {
					return fmt.Errorf("register aws rds iam auth: %w", err)
				}
			}
			sqlDB, err := cli.ConnectToPostgres(inv.Context(), logger, sqlDriver, flags.PostgresURL, nil)
			if err != nil {
				return fmt.Errorf("connect to postgres: %w", err)
			}
			defer func() {
				_ = sqlDB.Close()
			}()
			logger.Info(ctx, "connected to postgres")
			if err := dbcrypt.Delete(ctx, logger, sqlDB); err != nil {
				return fmt.Errorf("delete encrypted data: %w", err)
			}
			logger.Info(ctx, "operation completed successfully")
			return nil
		},
	}
	flags.attach(&cmd.Options)
	return cmd
}
type rotateFlags struct {
	PostgresURL  string
	PostgresAuth string
	New          string
	Old          []string
}
func (f *rotateFlags) attach(opts *serpent.OptionSet) {
	*opts = append(
		*opts,
		serpent.Option{
			Flag:        "postgres-url",
			Env:         "CODER_PG_CONNECTION_URL",
			Description: "The connection URL for the Postgres database.",
			Value:       serpent.StringOf(&f.PostgresURL),
		},
		serpent.Option{
			Name:        "Postgres Connection Auth",
			Description: "Type of auth to use when connecting to postgres.",
			Flag:        "postgres-connection-auth",
			Env:         "CODER_PG_CONNECTION_AUTH",
			Default:     "password",
			Value:       serpent.EnumOf(&f.PostgresAuth, codersdk.PostgresAuthDrivers...),
		},
		serpent.Option{
			Flag:        "new-key",
			Env:         "CODER_EXTERNAL_TOKEN_ENCRYPTION_ENCRYPT_NEW_KEY",
			Description: "The new external token encryption key. Must be base64-encoded.",
			Value:       serpent.StringOf(&f.New),
		},
		serpent.Option{
			Flag:        "old-keys",
			Env:         "CODER_EXTERNAL_TOKEN_ENCRYPTION_ENCRYPT_OLD_KEYS",
			Description: "The old external token encryption keys. Must be a comma-separated list of base64-encoded keys.",
			Value:       serpent.StringArrayOf(&f.Old),
		},
		cliui.SkipPromptOption(),
	)
}
func (f *rotateFlags) valid() error {
	if f.PostgresURL == "" {
		return fmt.Errorf("no database configured")
	}
	if f.New == "" {
		return fmt.Errorf("no new key provided")
	}
	if val, err := base64.StdEncoding.DecodeString(f.New); err != nil {
		return fmt.Errorf("new key must be base64-encoded")
	} else if len(val) != 32 {
		return fmt.Errorf("new key must be exactly 32 bytes in length")
	}
	for i, k := range f.Old {
		if val, err := base64.StdEncoding.DecodeString(k); err != nil {
			return fmt.Errorf("old key at index %d must be base64-encoded", i)
		} else if len(val) != 32 {
			return fmt.Errorf("old key at index %d must be exactly 32 bytes in length", i)
		}
		// Pedantic, but typos here will ruin your day.
		if k == f.New {
			return fmt.Errorf("old key at index %d is the same as the new key", i)
		}
	}
	return nil
}
type decryptFlags struct {
	PostgresURL  string
	PostgresAuth string
	Keys         []string
}
func (f *decryptFlags) attach(opts *serpent.OptionSet) {
	*opts = append(
		*opts,
		serpent.Option{
			Flag:        "postgres-url",
			Env:         "CODER_PG_CONNECTION_URL",
			Description: "The connection URL for the Postgres database.",
			Value:       serpent.StringOf(&f.PostgresURL),
		},
		serpent.Option{
			Name:        "Postgres Connection Auth",
			Description: "Type of auth to use when connecting to postgres.",
			Flag:        "postgres-connection-auth",
			Env:         "CODER_PG_CONNECTION_AUTH",
			Default:     "password",
			Value:       serpent.EnumOf(&f.PostgresAuth, codersdk.PostgresAuthDrivers...),
		},
		serpent.Option{
			Flag:        "keys",
			Env:         "CODER_EXTERNAL_TOKEN_ENCRYPTION_DECRYPT_KEYS",
			Description: "Keys required to decrypt existing data. Must be a comma-separated list of base64-encoded keys.",
			Value:       serpent.StringArrayOf(&f.Keys),
		},
		cliui.SkipPromptOption(),
	)
}
func (f *decryptFlags) valid() error {
	if f.PostgresURL == "" {
		return fmt.Errorf("no database configured")
	}
	if len(f.Keys) == 0 {
		return fmt.Errorf("no keys provided")
	}
	for i, k := range f.Keys {
		if val, err := base64.StdEncoding.DecodeString(k); err != nil {
			return fmt.Errorf("key at index %d must be base64-encoded", i)
		} else if len(val) != 32 {
			return fmt.Errorf("key at index %d must be exactly 32 bytes in length", i)
		}
	}
	return nil
}
type deleteFlags struct {
	PostgresURL  string
	PostgresAuth string
	Confirm      bool
}
func (f *deleteFlags) attach(opts *serpent.OptionSet) {
	*opts = append(
		*opts,
		serpent.Option{
			Flag:        "postgres-url",
			Env:         "CODER_EXTERNAL_TOKEN_ENCRYPTION_POSTGRES_URL",
			Description: "The connection URL for the Postgres database.",
			Value:       serpent.StringOf(&f.PostgresURL),
		},
		serpent.Option{
			Name:        "Postgres Connection Auth",
			Description: "Type of auth to use when connecting to postgres.",
			Flag:        "postgres-connection-auth",
			Env:         "CODER_PG_CONNECTION_AUTH",
			Default:     "password",
			Value:       serpent.EnumOf(&f.PostgresAuth, codersdk.PostgresAuthDrivers...),
		},
		cliui.SkipPromptOption(),
	)
}
func (f *deleteFlags) valid() error {
	if f.PostgresURL == "" {
		return fmt.Errorf("no database configured")
	}
	return nil
}
