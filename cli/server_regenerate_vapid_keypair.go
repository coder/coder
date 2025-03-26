//go:build !slim

package cli

import (
	"fmt"

	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/awsiamrds"
	"github.com/coder/coder/v2/coderd/notifications/push"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) newRegenerateVapidKeypairCommand() *serpent.Command {
	var (
		regenVapidKeypairDBURL  string
		regenVapidKeypairPgAuth string
	)
	regenerateVapidKeypairCommand := &serpent.Command{
		Use:    "regenerate-vapid-keypair",
		Short:  "Regenerate the VAPID keypair used for web push notifications.",
		Hidden: true, // Hide this command as it's an experimental feature
		Handler: func(inv *serpent.Invocation) error {
			var (
				ctx, cancel = inv.SignalNotifyContext(inv.Context(), StopSignals...)
				cfg         = r.createConfig()
				logger      = inv.Logger.AppendSinks(sloghuman.Sink(inv.Stderr))
			)
			if r.verbose {
				logger = logger.Leveled(slog.LevelDebug)
			}

			defer cancel()

			if regenVapidKeypairDBURL == "" {
				cliui.Infof(inv.Stdout, "Using built-in PostgreSQL (%s)", cfg.PostgresPath())
				url, closePg, err := startBuiltinPostgres(ctx, cfg, logger, "")
				if err != nil {
					return err
				}
				defer func() {
					_ = closePg()
				}()
				regenVapidKeypairDBURL = url
			}

			sqlDriver := "postgres"
			var err error
			if codersdk.PostgresAuth(regenVapidKeypairPgAuth) == codersdk.PostgresAuthAWSIAMRDS {
				sqlDriver, err = awsiamrds.Register(inv.Context(), sqlDriver)
				if err != nil {
					return xerrors.Errorf("register aws rds iam auth: %w", err)
				}
			}

			sqlDB, err := ConnectToPostgres(ctx, logger, sqlDriver, regenVapidKeypairDBURL, nil)
			if err != nil {
				return xerrors.Errorf("connect to postgres: %w", err)
			}
			defer func() {
				_ = sqlDB.Close()
			}()
			db := database.New(sqlDB)

			// Confirm that the user really wants to regenerate the VAPID keypair.
			cliui.Infof(inv.Stdout, "Regenerating VAPID keypair...")
			cliui.Infof(inv.Stdout, "This will delete all existing webpush subscriptions.")
			cliui.Infof(inv.Stdout, "Are you sure you want to continue? (y/N)")

			if resp, err := cliui.Prompt(inv, cliui.PromptOptions{
				IsConfirm: true,
				Default:   cliui.ConfirmNo,
			}); err != nil || resp != cliui.ConfirmYes {
				return xerrors.Errorf("VAPID keypair regeneration failed: %w", err)
			}

			if _, _, err := push.RegenerateVAPIDKeys(ctx, db); err != nil {
				return xerrors.Errorf("regenerate vapid keypair: %w", err)
			}

			_, _ = fmt.Fprintln(inv.Stdout, "VAPID keypair regenerated successfully.")
			return nil
		},
	}

	regenerateVapidKeypairCommand.Options.Add(
		cliui.SkipPromptOption(),
		serpent.Option{
			Env:         "CODER_PG_CONNECTION_URL",
			Flag:        "postgres-url",
			Description: "URL of a PostgreSQL database. If empty, the built-in PostgreSQL deployment will be used (Coder must not be already running in this case).",
			Value:       serpent.StringOf(&regenVapidKeypairDBURL),
		},
		serpent.Option{
			Name:        "Postgres Connection Auth",
			Description: "Type of auth to use when connecting to postgres.",
			Flag:        "postgres-connection-auth",
			Env:         "CODER_PG_CONNECTION_AUTH",
			Default:     "password",
			Value:       serpent.EnumOf(&regenVapidKeypairPgAuth, codersdk.PostgresAuthDrivers...),
		},
	)

	return regenerateVapidKeypairCommand
}
