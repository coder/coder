//go:build !slim

package cli

import (
	"bytes"
	"context"
	"encoding/base64"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/coder/coder/v2/cli"
	"github.com/coder/coder/v2/cli/clibase"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/dbcrypt"

	"golang.org/x/xerrors"
)

func (r *RootCmd) dbcryptCmd() *clibase.Cmd {
	dbcryptCmd := &clibase.Cmd{
		Use:   "dbcrypt",
		Short: "Manage database encryption.",
		Handler: func(inv *clibase.Invocation) error {
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

func (*RootCmd) dbcryptRotateCmd() *clibase.Cmd {
	var (
		vals = new(codersdk.DeploymentValues)
		opts = vals.Options()
	)
	cmd := &clibase.Cmd{
		Use:   "rotate",
		Short: "Rotate database encryption keys.",
		Options: clibase.OptionSet{
			*opts.ByName("Postgres Connection URL"),
			*opts.ByName("External Token Encryption Keys"),
		},
		Middleware: clibase.Chain(
			clibase.RequireNArgs(0),
		),
		Handler: func(inv *clibase.Invocation) error {
			ctx, cancel := context.WithCancel(inv.Context())
			defer cancel()
			logger := slog.Make(sloghuman.Sink(inv.Stdout))
			if ok, _ := inv.ParsedFlags().GetBool("verbose"); ok {
				logger = logger.Leveled(slog.LevelDebug)
			}

			if vals.PostgresURL == "" {
				return xerrors.Errorf("no database configured")
			}

			switch len(vals.ExternalTokenEncryptionKeys) {
			case 0:
				return xerrors.Errorf("no external token encryption keys provided")
			case 1:
				logger.Info(ctx, "only one key provided, data will be re-encrypted with the same key")
			}

			keys := make([][]byte, 0, len(vals.ExternalTokenEncryptionKeys))
			var newKey []byte
			for idx, ek := range vals.ExternalTokenEncryptionKeys {
				dk, err := base64.StdEncoding.DecodeString(ek)
				if err != nil {
					return xerrors.Errorf("key must be base64-encoded")
				}
				if idx == 0 {
					newKey = dk
				} else if bytes.Equal(dk, newKey) {
					return xerrors.Errorf("old key at index %d is the same as the new key", idx)
				}
				keys = append(keys, dk)
			}

			ciphers, err := dbcrypt.NewCiphers(keys...)
			if err != nil {
				return xerrors.Errorf("create ciphers: %w", err)
			}

			sqlDB, err := cli.ConnectToPostgres(inv.Context(), logger, "postgres", vals.PostgresURL.Value())
			if err != nil {
				return xerrors.Errorf("connect to postgres: %w", err)
			}
			defer func() {
				_ = sqlDB.Close()
			}()
			logger.Info(ctx, "connected to postgres")
			if err := dbcrypt.Rotate(ctx, logger, sqlDB, ciphers); err != nil {
				return xerrors.Errorf("rotate ciphers: %w", err)
			}
			logger.Info(ctx, "operation completed successfully")
			return nil
		},
	}
	return cmd
}

func (*RootCmd) dbcryptDecryptCmd() *clibase.Cmd {
	var (
		vals = new(codersdk.DeploymentValues)
		opts = vals.Options()
	)
	cmd := &clibase.Cmd{
		Use:   "decrypt",
		Short: "Decrypt a previously encrypted database.",
		Options: clibase.OptionSet{
			*opts.ByName("Postgres Connection URL"),
			*opts.ByName("External Token Encryption Keys"),
		},
		Middleware: clibase.Chain(
			clibase.RequireNArgs(0),
		),
		Handler: func(inv *clibase.Invocation) error {
			ctx, cancel := context.WithCancel(inv.Context())
			defer cancel()
			logger := slog.Make(sloghuman.Sink(inv.Stdout))
			if ok, _ := inv.ParsedFlags().GetBool("verbose"); ok {
				logger = logger.Leveled(slog.LevelDebug)
			}

			if vals.PostgresURL == "" {
				return xerrors.Errorf("no database configured")
			}

			switch len(vals.ExternalTokenEncryptionKeys) {
			case 0:
				return xerrors.Errorf("no external token encryption keys provided")
			case 1:
				logger.Info(ctx, "only one key provided, data will be re-encrypted with the same key")
			}

			keys := make([][]byte, 0, len(vals.ExternalTokenEncryptionKeys))
			var newKey []byte
			for idx, ek := range vals.ExternalTokenEncryptionKeys {
				dk, err := base64.StdEncoding.DecodeString(ek)
				if err != nil {
					return xerrors.Errorf("key must be base64-encoded")
				}
				if idx == 0 {
					newKey = dk
				} else if bytes.Equal(dk, newKey) {
					return xerrors.Errorf("old key at index %d is the same as the new key", idx)
				}
				keys = append(keys, dk)
			}

			ciphers, err := dbcrypt.NewCiphers(keys...)
			if err != nil {
				return xerrors.Errorf("create ciphers: %w", err)
			}

			sqlDB, err := cli.ConnectToPostgres(inv.Context(), logger, "postgres", vals.PostgresURL.Value())
			if err != nil {
				return xerrors.Errorf("connect to postgres: %w", err)
			}
			defer func() {
				_ = sqlDB.Close()
			}()
			logger.Info(ctx, "connected to postgres")
			if err := dbcrypt.Decrypt(ctx, logger, sqlDB, ciphers); err != nil {
				return xerrors.Errorf("rotate ciphers: %w", err)
			}
			logger.Info(ctx, "operation completed successfully")
			return nil
		},
	}
	return cmd
}

func (*RootCmd) dbcryptDeleteCmd() *clibase.Cmd {
	var (
		vals = new(codersdk.DeploymentValues)
		opts = vals.Options()
	)
	cmd := &clibase.Cmd{
		Use:   "delete",
		Short: "Delete all encrypted data from the database. THIS IS A DESTRUCTIVE OPERATION.",
		Options: clibase.OptionSet{
			*opts.ByName("Postgres Connection URL"),
		},
		Middleware: clibase.Chain(
			clibase.RequireNArgs(0),
		),
		Handler: func(inv *clibase.Invocation) error {
			ctx, cancel := context.WithCancel(inv.Context())
			defer cancel()
			logger := slog.Make(sloghuman.Sink(inv.Stdout))
			if ok, _ := inv.ParsedFlags().GetBool("verbose"); ok {
				logger = logger.Leveled(slog.LevelDebug)
			}

			if vals.PostgresURL == "" {
				return xerrors.Errorf("no database configured")
			}

			if _, err := cliui.Prompt(inv, cliui.PromptOptions{
				Text:      "This will delete all encrypted data from the database. Are you sure you want to continue?",
				IsConfirm: true,
			}); err != nil {
				return err
			}

			sqlDB, err := cli.ConnectToPostgres(inv.Context(), logger, "postgres", vals.PostgresURL.Value())
			if err != nil {
				return xerrors.Errorf("connect to postgres: %w", err)
			}
			defer func() {
				_ = sqlDB.Close()
			}()
			logger.Info(ctx, "connected to postgres")
			if err := dbcrypt.Delete(ctx, logger, sqlDB); err != nil {
				return xerrors.Errorf("delete encrypted data: %w", err)
			}
			logger.Info(ctx, "operation completed successfully")
			return nil
		},
	}
	return cmd
}
