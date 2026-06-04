//go:build !slim

package cli

import (
	"os/signal"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/sloghuman"
	agplcli "github.com/coder/coder/v2/cli"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/awsiamrds"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/scaletest/agentfake"
	"github.com/coder/serpent"
)

// AGPLExperimental shadows the embedded RootCmd.AGPLExperimental to inject the
// enterprise-only agentfake scaletest subcommand into the scaletest subtree.
func (r *RootCmd) AGPLExperimental() []*serpent.Command {
	cmds := r.RootCmd.AGPLExperimental()
	for _, cmd := range cmds {
		if cmd.Use == "scaletest" {
			cmd.Children = append(cmd.Children, r.scaletestAgentFake())
		}
	}
	return cmds
}

func (r *RootCmd) scaletestAgentFake() *serpent.Command {
	var (
		template                string
		owner                   string
		prometheusAddress       string
		expectedAgents          int64
		expectedAgentsTolerance int64
		postgresURL             string
		postgresAuth            string
		connReportInterval      time.Duration
		connReportDuration      time.Duration
	)

	cmd := &serpent.Command{
		Use:   "agentfake",
		Short: "Run fake external agents against workspaces of the given template.",
		Long: agplcli.FormatExamples(
			agplcli.Example{
				Description: "Connect a fake agent for every external-agent workspace built from the template named " +
					"\"agentfake-runner\".",
				Command: "coder exp scaletest agentfake --template agentfake-runner",
			},
		) + "\n\n" +
			"Enumerates external-agent workspaces matching --template (optionally filtered by --owner), " +
			"fetches each workspace agent's external-agent credentials, and supervises one in-process fake " +
			"agent per token until the command is interrupted.\n\n" +
			"Requires a session token whose user is template-admin (or higher) on a deployment licensed " +
			"for the workspace external-agent feature, and a Postgres connection URL (with credentials " +
			"encoded into the URL) that points at the same database instance coderd is using. Intended " +
			"to run inside the same network as coderd, not from operator machines outside the cluster. " +
			"The workspace listing and external-agent feature are gated server-side. Pair with " +
			"`coder exp scaletest create-workspaces --no-wait-for-agents` to seed the workspaces this " +
			"command will pick up. Workspaces created after this command starts are NOT picked up; " +
			"rerun the command after seeding more.\n\n" +
			"Exposes Prometheus metrics (Go runtime and process collectors) at /metrics on " +
			"--prometheus-address (default 0.0.0.0:21112).",
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			notifyCtx, stop := signal.NotifyContext(ctx, agplcli.StopSignals...)
			defer stop()
			ctx = notifyCtx

			if _, err := agplcli.RequireAdmin(ctx, client); err != nil {
				return err
			}

			if template == "" {
				return xerrors.New("--template is required")
			}
			if postgresURL == "" {
				return xerrors.New("--postgres-url (CODER_PG_CONNECTION_URL) is required")
			}
			if expectedAgents > 0 && expectedAgentsTolerance < 0 {
				return xerrors.New("--expected-agents-tolerance must be non-negative")
			}

			logger := inv.Logger.AppendSinks(sloghuman.Sink(inv.Stderr))
			if ok, _ := inv.ParsedFlags().GetBool("verbose"); ok {
				logger = logger.Leveled(slog.LevelDebug)
			}

			sqlDriver := "postgres"
			if codersdk.PostgresAuth(postgresAuth) == codersdk.PostgresAuthAWSIAMRDS {
				var err error
				sqlDriver, err = awsiamrds.Register(ctx, sqlDriver)
				if err != nil {
					return xerrors.Errorf("register aws rds iam auth: %w", err)
				}
			}
			sqlDB, err := agplcli.ConnectToPostgres(ctx, logger, sqlDriver, postgresURL, nil)
			if err != nil {
				return xerrors.Errorf("dial postgres: %w", err)
			}
			defer sqlDB.Close()
			db := database.New(sqlDB)

			prometheusSrvClose := agplcli.ServeHandler(ctx, logger,
				promhttp.Handler(), prometheusAddress, "prometheus")
			defer prometheusSrvClose()

			metrics := agentfake.NewMetrics(prometheus.DefaultRegisterer)

			mgr := agentfake.NewManager(logger, client.URL, client, db, agentfake.ManagerOptions{
				Template:                 template,
				Owner:                    owner,
				Metrics:                  metrics,
				ExpectedAgents:           expectedAgents,
				ExpectedAgentsTolerance:  expectedAgentsTolerance,
				ConnectionReportInterval: connReportInterval,
				ConnectionReportDuration: connReportDuration,
			})
			defer mgr.Close()

			if err := mgr.Run(ctx); err != nil {
				return xerrors.Errorf("run agentfake manager: %w", err)
			}
			return nil
		},
	}

	cmd.Options = serpent.OptionSet{
		{
			Flag:        "template",
			Env:         "CODER_SCALETEST_AGENTFAKE_TEMPLATE",
			Description: "Name of the template whose external-agent workspaces should be supervised. Required.",
			Value:       serpent.StringOf(&template),
		},
		{
			Flag:        "owner",
			Env:         "CODER_SCALETEST_AGENTFAKE_OWNER",
			Description: "Optional workspace-owner filter (username). When empty, all owners' workspaces of the template are included.",
			Value:       serpent.StringOf(&owner),
		},
		{
			Flag:        "prometheus-address",
			Env:         "CODER_SCALETEST_AGENTFAKE_PROMETHEUS_ADDRESS",
			Default:     "0.0.0.0:21112",
			Description: "Address on which to expose Prometheus metrics (Go runtime + process collectors) at /metrics.",
			Value:       serpent.StringOf(&prometheusAddress),
		},
		{
			Flag:        "expected-agents",
			Env:         "CODER_SCALETEST_AGENTFAKE_EXPECTED_AGENTS",
			Default:     "0",
			Description: "Expected number of agents to enumerate. When non-zero, the command polls until the workspace count is within expected ± expected-agents-tolerance before enumerating.",
			Value:       serpent.Int64Of(&expectedAgents),
		},
		{
			Flag:        "expected-agents-tolerance",
			Env:         "CODER_SCALETEST_AGENTFAKE_EXPECTED_AGENTS_TOLERANCE",
			Default:     "0",
			Description: "Acceptable variance around --expected-agents. Ignored when --expected-agents is 0.",
			Value:       serpent.Int64Of(&expectedAgentsTolerance),
		},
		{
			Flag:        "connection-report-interval",
			Env:         "CODER_SCALETEST_AGENTFAKE_CONNECTION_REPORT_INTERVAL",
			Description: "Idle gap between synthetic SSH connect events per fake agent. Zero disables connection reporting.",
			Default:     "30s",
			Value:       serpent.DurationOf(&connReportInterval),
		},
		{
			Flag:        "connection-report-duration",
			Env:         "CODER_SCALETEST_AGENTFAKE_CONNECTION_REPORT_DURATION",
			Description: "Synthetic SSH session length per fake agent. Zero disables connection reporting.",
			Default:     "5s",
			Value:       serpent.DurationOf(&connReportDuration),
		},
		{
			Flag:        "postgres-url",
			Env:         "CODER_PG_CONNECTION_URL",
			Description: "URL of the Postgres database that the target coderd is using. Required; used to bulk-fetch external-agent tokens for the enumerated workspaces in a single query. The same connection string the coder server pods consume (e.g. the coder-db-url secret in scaletest deployments).",
			Value:       serpent.StringOf(&postgresURL),
		},
		serpent.Option{
			Name:        "Postgres Connection Auth",
			Description: "Type of auth to use when connecting to postgres.",
			Flag:        "postgres-connection-auth",
			Env:         "CODER_PG_CONNECTION_AUTH",
			Default:     "password",
			Value:       serpent.EnumOf(&postgresAuth, codersdk.PostgresAuthDrivers...),
		},
	}

	return cmd
}
