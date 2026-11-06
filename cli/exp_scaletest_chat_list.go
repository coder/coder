//go:build !slim

package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/chat"
	"github.com/coder/coder/v2/scaletest/chatlist"
	"github.com/coder/coder/v2/scaletest/harness"
	"github.com/coder/coder/v2/scaletest/loadtestutil"
	"github.com/coder/serpent"
)

func (r *RootCmd) scaletestChatList() *serpent.Command {
	var (
		chats                   int64
		listRequests            int64
		pageSize                int64
		concurrency             int64
		username                string
		llmMockURL              string
		providerPropagationWait time.Duration
		tracingFlags            = &scaletestTracingFlags{}
		prometheusFlags         = &scaletestPrometheusFlags{}
		timeoutStrategy         = &timeoutFlags{}
		output                  = &scaletestOutputFlags{}
	)

	cmd := &serpent.Command{
		Use:   "chat-list",
		Short: "Generate chat list load for a single user with many chats.",
		Long: "Ensures a single user owns the requested number of idle chats, then issues a fixed number of " +
			"chat list requests as that user. Chats are labeled and reused by later runs, so the same data set " +
			"can be measured across deployments with different experiments enabled.",
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			ctx, stop := inv.SignalNotifyContext(ctx, StopSignals...)
			defer stop()

			outputs, err := output.parse()
			if err != nil {
				return xerrors.Errorf("could not parse --output flags: %w", err)
			}
			switch {
			case chats < 1:
				return xerrors.Errorf("--chats must be at least 1")
			case listRequests < 1:
				return xerrors.Errorf("--list-requests must be at least 1")
			case pageSize < 1:
				return xerrors.Errorf("--page-size must be at least 1")
			case concurrency < 1:
				return xerrors.Errorf("--concurrency must be at least 1")
			}

			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}
			me, err := RequireAdmin(ctx, client)
			if err != nil {
				return err
			}
			organizationID := me.OrganizationIDs[0]
			logger := inv.Logger

			modelForOrg, err := chat.EnsureScaletestChatModel(ctx, client, logger, llmMockURL, providerPropagationWait)
			if err != nil {
				return err
			}
			modelID, err := modelForOrg(organizationID)
			if err != nil {
				return xerrors.Errorf("ensure scaletest model config for organization %s: %w", organizationID, err)
			}

			user, err := chatlist.EnsureUser(ctx, client, organizationID, username)
			if err != nil {
				return err
			}
			token, err := client.CreateToken(ctx, user.ID.String(), codersdk.CreateTokenRequest{
				TokenName: fmt.Sprintf("scaletest-chat-list-%d", time.Now().UnixNano()),
			})
			if err != nil {
				return xerrors.Errorf("create token for %q: %w", username, err)
			}
			defer func() {
				keyID, _, _ := strings.Cut(token.Key, "-")
				if err := client.DeleteAPIKey(ctx, user.ID.String(), keyID); err != nil {
					_, _ = fmt.Fprintf(inv.Stderr, "Failed to delete scaletest token: %v\n", err)
				}
			}()
			newUserClient := func() (*codersdk.Client, error) {
				c, err := loadtestutil.DupClientCopyingHeaders(client, BypassHeader)
				if err != nil {
					return nil, err
				}
				c.SessionTokenProvider = codersdk.FixedSessionTokenProvider{SessionToken: token.Key}
				return c, nil
			}

			setupClient, err := newUserClient()
			if err != nil {
				return xerrors.Errorf("create user client: %w", err)
			}
			existing, created, err := chatlist.EnsureChats(ctx, setupClient, logger, organizationID, modelID, int(chats))
			if err != nil {
				return xerrors.Errorf("ensure chats: %w", err)
			}
			_, _ = fmt.Fprintf(inv.Stderr, "User %q has %d scaletest chats (%d reused, %d created).\n", username, existing+created, existing, created)

			reg := prometheus.NewRegistry()
			metrics := chatlist.NewMetrics(reg)
			prometheusSrvClose := ServeHandler(ctx, logger, promhttp.HandlerFor(reg, promhttp.HandlerOpts{}), prometheusFlags.Address, "prometheus")

			tracerProvider, closeTracing, tracingEnabled, err := tracingFlags.provider(ctx)
			if err != nil {
				prometheusSrvClose()
				return xerrors.Errorf("create tracer provider: %w", err)
			}
			defer func() {
				if tracingEnabled {
					_, _ = fmt.Fprintln(inv.Stderr, "Uploading traces...")
				}
				if err := closeTracing(ctx); err != nil {
					_, _ = fmt.Fprintf(inv.Stderr, "Error uploading traces: %+v\n", err)
				}
				_, _ = fmt.Fprintf(inv.Stderr, "Waiting %s for prometheus metrics to be scraped\n", prometheusFlags.Wait)
				<-time.After(prometheusFlags.Wait)
				prometheusSrvClose()
			}()
			tracer := tracerProvider.Tracer(scaletestTracerName)

			th := harness.NewTestHarness(timeoutStrategy.wrapStrategy(harness.ConcurrentExecutionStrategy{}), harness.ConcurrentExecutionStrategy{})
			for i := int64(0); i < concurrency; i++ {
				cfg := chatlist.Config{
					ListRequests: int(listRequests),
					PageSize:     int(pageSize),
					Metrics:      metrics,
				}
				if err := cfg.Validate(); err != nil {
					return xerrors.Errorf("validate config: %w", err)
				}
				runnerClient, err := newUserClient()
				if err != nil {
					return xerrors.Errorf("create runner client: %w", err)
				}
				var runner harness.Runnable = chatlist.NewRunner(runnerClient, cfg)
				if tracingEnabled {
					runner = &runnableTraceWrapper{
						tracer:   tracer,
						runner:   runner,
						spanName: fmt.Sprintf("chat-list/%d", i),
					}
				}
				th.AddRun("chat-list", fmt.Sprintf("%d", i), runner)
			}

			_, _ = fmt.Fprintf(inv.Stderr, "Running %d list runners x %d requests (page size %d)...\n", concurrency, listRequests, pageSize)
			testCtx, testCancel := timeoutStrategy.toContext(ctx)
			defer testCancel()
			if err := th.Run(testCtx); err != nil {
				return xerrors.Errorf("run test harness: %w", err)
			}

			res := th.Results()
			for _, o := range outputs {
				if err := o.write(res, inv.Stdout); err != nil {
					return xerrors.Errorf("write output %q to %q: %w", o.format, o.path, err)
				}
			}
			if res.TotalFail > 0 {
				return xerrors.Errorf("scale test failed: %d/%d runs failed", res.TotalFail, res.TotalRuns)
			}
			_, _ = fmt.Fprintf(inv.Stderr, "Scale test passed: %d/%d runs succeeded\n", res.TotalPass, res.TotalRuns)
			return nil
		},
	}

	cmd.Options = serpent.OptionSet{
		{
			Flag:        "chats",
			Description: "Number of unarchived chats the scaletest user must own. Existing scaletest chats are reused and only the shortfall is created.",
			Value:       serpent.Int64Of(&chats),
			Required:    true,
		},
		{
			Flag:        "list-requests",
			Description: "Number of sequential chat list requests each runner issues.",
			Default:     "50",
			Value:       serpent.Int64Of(&listRequests),
		},
		{
			Flag:        "page-size",
			Description: "Page size for each chat list request. The default matches the agents page.",
			Default:     "50",
			Value:       serpent.Int64Of(&pageSize),
		},
		{
			Flag:        "concurrency",
			Description: "Number of runners issuing list requests in parallel.",
			Default:     "1",
			Value:       serpent.Int64Of(&concurrency),
		},
		{
			Flag:        "username",
			Description: "User that owns the chats and issues the list requests. Created if it does not exist.",
			Default:     "scaletest-chat-list",
			Value:       serpent.StringOf(&username),
		},
		{
			Flag:        "llm-mock-url",
			Description: "URL of the mock LLM server (e.g. http://127.0.0.1:8080/v1). Used to create or update the Scaletest LLM Mock provider and model config that the chats reference. Chats are created idle, so the mock receives no requests.",
			Value:       serpent.StringOf(&llmMockURL),
			Required:    true,
		},
		{
			Flag:        "provider-propagation-wait",
			Description: "Time to wait after creating or updating the mock LLM provider so every coderd replica's cached provider config expires.",
			Default:     chat.DefaultProviderPropagationWait.String(),
			Value:       serpent.DurationOf(&providerPropagationWait),
			Hidden:      true,
		},
	}
	output.attach(&cmd.Options)
	tracingFlags.attach(&cmd.Options)
	prometheusFlags.attach(&cmd.Options)
	timeoutStrategy.attach(&cmd.Options)
	return cmd
}
