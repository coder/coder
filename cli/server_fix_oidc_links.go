//go:build !slim

package cli

import (
	"fmt"
	"net/http"
	"strings"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/sloghuman"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/coderd/authlink"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/awsiamrds"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) newFixOIDCLinksCommand() *serpent.Command {
	var (
		pgURL         string
		pgAuth        string
		issuerURL     string
		dryRun        bool
		forceResetAll bool
	)
	fixOIDCLinksCmd := &serpent.Command{
		Use:   "fix-oidc-links",
		Short: "Reset OIDC linked IDs that do not match the expected issuer, allowing users to re-authenticate.",
		Handler: func(inv *serpent.Invocation) error {
			var (
				ctx, cancel = inv.SignalNotifyContext(inv.Context(), StopSignals...)
				logger      = inv.Logger.AppendSinks(sloghuman.Sink(inv.Stderr))
			)
			if r.verbose {
				logger = logger.Leveled(slog.LevelDebug)
			}
			defer cancel()

			issuerURL = strings.TrimSpace(issuerURL)
			if forceResetAll && issuerURL != "" {
				return xerrors.New("--force-reset-all and --issuer-url are mutually exclusive")
			}
			if !forceResetAll && issuerURL == "" {
				return xerrors.Errorf("the --%s flag is required, set it to the OIDC issuer URL (e.g. https://accounts.google.com)", "issuer-url")
			}

			var issuer string
			if forceResetAll {
				// Use an unmatchable issuer so the existing analysis shows
				// all links as "mismatched" and the reset clears everything.
				issuer = authlink.UnmatchableIssuer
			} else {
				// Resolve the canonical issuer from OIDC discovery.
				cliui.Infof(inv.Stdout, "Resolving OIDC issuer from %q...", issuerURL)
				// TODO: The default client might not be configured with the right certs to make this request.
				resolved, err := authlink.ResolveIssuer(ctx, http.DefaultClient, issuerURL)
				if err != nil {
					return xerrors.Errorf("resolve issuer: %w", err)
				}
				issuer = resolved
				_, _ = fmt.Fprintf(inv.Stdout, "Resolved OIDC issuer: %q\n\n", issuer)
			}

			// Connect to the database.
			if pgURL == "" {
				return xerrors.New("the --postgres-url flag is required")
			}

			sqlDriver := "postgres"
			if codersdk.PostgresAuth(pgAuth) == codersdk.PostgresAuthAWSIAMRDS {
				var err error
				sqlDriver, err = awsiamrds.Register(inv.Context(), sqlDriver)
				if err != nil {
					return xerrors.Errorf("register aws rds iam auth: %w", err)
				}
			}

			sqlDB, err := ConnectToPostgres(ctx, logger, sqlDriver, pgURL, nil)
			if err != nil {
				return xerrors.Errorf("connect to postgres: %w", err)
			}
			defer func() {
				_ = sqlDB.Close()
			}()
			db := database.New(sqlDB)

			// Run analysis.
			analysis, err := authlink.AnalyzeOIDCLinks(ctx, db, issuer)
			if err != nil {
				return xerrors.Errorf("analyze OIDC links: %w", err)
			}
			authlink.PrintAnalysis(inv.Stdout, analysis, issuer)
			_, _ = fmt.Fprintln(inv.Stdout)

			if dryRun {
				return nil
			}

			mismatchedTotal := analysis.MismatchedTotal()
			if mismatchedTotal == 0 {
				_, _ = fmt.Fprintln(inv.Stdout, "Nothing to do. All OIDC links match the expected issuer.")
				return nil
			}

			// Molly guard.
			_, _ = fmt.Fprintf(inv.Stdout, "This will reset %d linked IDs to allow affected users to re-authenticate.\n", mismatchedTotal)
			if _, err := cliui.Prompt(inv, cliui.PromptOptions{
				Text:      "Are you sure you want to continue?",
				IsConfirm: true,
				Default:   cliui.ConfirmNo,
			}); err != nil {
				return err
			}
			_, _ = fmt.Fprintln(inv.Stdout)

			// Execute the reset.
			count, err := authlink.ResetMismatchedOIDCLinks(ctx, db, issuer)
			if err != nil {
				return xerrors.Errorf("reset mismatched OIDC links: %w", err)
			}
			cliui.Infof(inv.Stdout, "Reset %d linked IDs.", count)
			_, _ = fmt.Fprintln(inv.Stdout)

			// Print updated analysis.
			analysis, err = authlink.AnalyzeOIDCLinks(ctx, db, issuer)
			if err != nil {
				return xerrors.Errorf("re-analyze OIDC links: %w", err)
			}
			authlink.PrintAnalysis(inv.Stdout, analysis, issuer)
			return nil
		},
	}

	fixOIDCLinksCmd.Options.Add(
		cliui.SkipPromptOption(),
		serpent.Option{
			Env:         "CODER_PG_CONNECTION_URL",
			Flag:        "postgres-url",
			Description: "URL of a PostgreSQL database. If empty, the built-in PostgreSQL deployment will be used (Coder must not be already running in this case).",
			Value:       serpent.StringOf(&pgURL),
		},
		serpent.Option{
			Name:        "Postgres Connection Auth",
			Description: "Type of auth to use when connecting to postgres.",
			Flag:        "postgres-connection-auth",
			Env:         "CODER_PG_CONNECTION_AUTH",
			Default:     "password",
			Value:       serpent.EnumOf(&pgAuth, codersdk.PostgresAuthDrivers...),
		},
		serpent.Option{
			Env:         "CODER_OIDC_ISSUER_URL",
			Flag:        "issuer-url",
			Description: "The OIDC issuer URL. The canonical issuer is resolved via OIDC discovery.",
			Value:       serpent.StringOf(&issuerURL),
		},
		serpent.Option{
			Flag:          "dry-run",
			FlagShorthand: "n",
			Env:           "CODER_FIX_OIDC_LINKS_DRY_RUN",
			Description:   "Print analysis only, do not modify the database.",
			Value:         serpent.BoolOf(&dryRun),
		},
		serpent.Option{
			Flag:        "force-reset-all",
			Env:         "CODER_FIX_OIDC_LINKS_FORCE_RESET_ALL",
			Description: "Reset all OIDC linked IDs, not just those with a mismatched issuer. Mutually exclusive with --issuer-url.",
			Value:       serpent.BoolOf(&forceResetAll),
		},
	)

	return fixOIDCLinksCmd
}
