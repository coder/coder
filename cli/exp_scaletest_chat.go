//go:build !slim

package cli

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/sloghuman"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/chat"
	"github.com/coder/coder/v2/scaletest/harness"
	"github.com/coder/coder/v2/scaletest/loadtestutil"
	"github.com/coder/serpent"
)

const (
	scaletestProviderDisplayName = "Scaletest LLM Mock"
	scaletestModelName           = "scaletest-model"
	scaletestModelDisplayName    = "Scaletest Model"
)

func (r *RootCmd) scaletestChat() *serpent.Command {
	var (
		count           int64
		workspaceID     string
		prompt          string
		turns           int64
		followUpPrompt  string
		llmMockURL      string
		tracingFlags    = &scaletestTracingFlags{}
		prometheusFlags = &scaletestPrometheusFlags{}
		timeoutStrategy = &timeoutFlags{}
		cleanupStrategy = newScaletestCleanupStrategy()
		output          = &scaletestOutputFlags{}
	)

	cmd := &serpent.Command{
		Use:   "chat",
		Short: "Run a chat scale test against the Coder API",
		Long:  "Creates N concurrent chats against a single pre-existing workspace and streams each conversation to completion.",
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()

			outputs, err := output.parse()
			if err != nil {
				return xerrors.Errorf("could not parse --output flags: %w", err)
			}

			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			_, err = requireAdmin(ctx, client)
			if err != nil {
				return err
			}

			client.HTTPClient.Transport = &codersdk.HeaderTransport{
				Transport: client.HTTPClient.Transport,
				Header:    BypassHeader,
			}

			wsID, err := uuid.Parse(workspaceID)
			if err != nil {
				return xerrors.Errorf("parse workspace-id: %w", err)
			}

			ws, err := client.Workspace(ctx, wsID)
			if err != nil {
				return xerrors.Errorf("fetch workspace: %w", err)
			}
			hasAgent := false
			for _, res := range ws.LatestBuild.Resources {
				if len(res.Agents) > 0 {
					hasAgent = true
					break
				}
			}
			if !hasAgent {
				return xerrors.Errorf("workspace %s has no agents in its latest build", ws.Name)
			}

			var modelConfigID *uuid.UUID
			if llmMockURL != "" {
				_, _ = fmt.Fprintf(inv.Stderr, "Bootstrapping mock LLM provider at %s...\n", llmMockURL)

				// Try to create a DB-backed openai-compat provider. If one
				// already exists the server returns 409 and we proceed.
				enabled := true
				_, err = client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
					Provider:    "openai-compat",
					DisplayName: scaletestProviderDisplayName,
					APIKey:      "scaletest-api-key",
					BaseURL:     llmMockURL,
					Enabled:     &enabled,
				})
				if err != nil {
					// A 409 means the provider already exists in the DB — that's fine.
					var sdkErr *codersdk.Error
					if !xerrors.As(err, &sdkErr) || sdkErr.StatusCode() != http.StatusConflict {
						return xerrors.Errorf("create scaletest chat provider: %w", err)
					}
					_, _ = fmt.Fprintf(inv.Stderr, "openai-compat provider already exists, proceeding...\n")
				} else {
					_, _ = fmt.Fprintf(inv.Stderr, "Created openai-compat provider pointing at %s\n", llmMockURL)
				}

				modelConfigs, err := client.ListChatModelConfigs(ctx)
				if err != nil {
					return xerrors.Errorf("list chat model configs: %w", err)
				}

				var existingModelConfig *codersdk.ChatModelConfig
				for i := range modelConfigs {
					if modelConfigs[i].Provider == "openai-compat" && modelConfigs[i].Model == scaletestModelName {
						existingModelConfig = &modelConfigs[i]
						break
					}
				}

				if existingModelConfig != nil {
					modelConfigID = &existingModelConfig.ID
					_, _ = fmt.Fprintf(inv.Stderr, "Reusing existing scaletest model config %s\n", existingModelConfig.ID)
				} else {
					enabled := true
					isDefault := false
					contextLimit := int64(4096)
					created, err := client.CreateChatModelConfig(ctx, codersdk.CreateChatModelConfigRequest{
						Provider:     "openai-compat",
						Model:        scaletestModelName,
						DisplayName:  scaletestModelDisplayName,
						Enabled:      &enabled,
						IsDefault:    &isDefault,
						ContextLimit: &contextLimit,
					})
					if err != nil {
						return xerrors.Errorf("create scaletest chat model config: %w", err)
					}
					modelConfigID = &created.ID
					_, _ = fmt.Fprintf(inv.Stderr, "Created scaletest model config %s\n", created.ID)
				}
			}

			reg := prometheus.NewRegistry()
			metrics := chat.NewMetrics(reg)

			logger := slog.Make(sloghuman.Sink(inv.Stdout)).Leveled(slog.LevelDebug)
			prometheusSrvClose := ServeHandler(ctx, logger, promhttp.HandlerFor(reg, promhttp.HandlerOpts{}), prometheusFlags.Address, "prometheus")
			defer prometheusSrvClose()

			tracerProvider, closeTracing, tracingEnabled, err := tracingFlags.provider(ctx)
			if err != nil {
				return xerrors.Errorf("create tracer provider: %w", err)
			}
			defer func() {
				// Allow time for traces to flush even if command context is
				// canceled. This is a no-op if tracing is not enabled.
				_, _ = fmt.Fprintln(inv.Stderr, "\nUploading traces...")
				if err := closeTracing(ctx); err != nil {
					_, _ = fmt.Fprintf(inv.Stderr, "\nError uploading traces: %+v\n", err)
				}
				// Wait for prometheus metrics to be scraped.
				_, _ = fmt.Fprintf(inv.Stderr, "Waiting %s for prometheus metrics to be scraped\n", prometheusFlags.Wait)
				<-time.After(prometheusFlags.Wait)
			}()
			tracer := tracerProvider.Tracer(scaletestTracerName)

			readyWG := &sync.WaitGroup{}
			startChan := make(chan struct{})

			th := harness.NewTestHarness(
				timeoutStrategy.wrapStrategy(harness.ConcurrentExecutionStrategy{}),
				cleanupStrategy.toStrategy(),
			)

			for i := range count {
				readyWG.Add(1)
				cfg := chat.Config{
					WorkspaceID:       wsID,
					Prompt:            prompt,
					ModelConfigID:     modelConfigID,
					Turns:             int(turns),
					FollowUpPrompt:    followUpPrompt,
					ReadyWaitGroup:    readyWG,
					StartChan:         startChan,
					Metrics:           metrics,
					MetricLabelValues: []string{},
				}
				if err := cfg.Validate(); err != nil {
					return xerrors.Errorf("validate config for runner %d: %w", i, err)
				}

				runnerClient, err := loadtestutil.DupClientCopyingHeaders(client, BypassHeader)
				if err != nil {
					return xerrors.Errorf("duplicate client for runner %d: %w", i, err)
				}
				var runner harness.Runnable = chat.NewRunner(runnerClient, cfg)
				if tracingEnabled {
					runner = &runnableTraceWrapper{
						tracer:   tracer,
						runner:   runner,
						spanName: "ChatRun",
					}
				}
				th.AddRun("chat", fmt.Sprintf("chat-%d", i), runner)
			}

			_, _ = fmt.Fprintln(inv.Stderr, "Starting chat scale test...")
			testCtx, testCancel := timeoutStrategy.toContext(ctx)
			defer testCancel()
			done := make(chan error, 1)
			go func() {
				done <- th.Run(testCtx)
			}()

			readyDone := make(chan struct{})
			go func() {
				readyWG.Wait()
				close(readyDone)
			}()
			select {
			case <-readyDone:
				_, _ = fmt.Fprintf(inv.Stderr, "All %d runners ready, starting chat storm...\n", count)
			case <-time.After(5 * time.Minute):
				return xerrors.Errorf("timed out waiting for runners to become ready")
			case <-ctx.Done():
				return ctx.Err()
			}
			close(startChan)

			if err := <-done; err != nil {
				return xerrors.Errorf("run harness: %w", err)
			}

			res := th.Results()
			for _, o := range outputs {
				if err := o.write(res, inv.Stdout); err != nil {
					return xerrors.Errorf("write output %q to %q: %w", o.format, o.path, err)
				}
			}

			_, _ = fmt.Fprintln(inv.Stderr, "Cleaning up (archiving chats)...")
			cleanupCtx, cleanupCancel := cleanupStrategy.toContext(ctx)
			defer cleanupCancel()
			if err := th.Cleanup(cleanupCtx); err != nil {
				return xerrors.Errorf("cleanup: %w", err)
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
			Flag:        "count",
			Description: "Number of concurrent chats to create.",
			Default:     "10",
			Value:       serpent.Int64Of(&count),
		},
		{
			Flag:        "workspace-id",
			Description: "UUID of the pre-existing workspace to create chats against.",
			Required:    true,
			Value:       serpent.StringOf(&workspaceID),
		},
		{
			Flag:        "prompt",
			Description: "Text prompt to send for the first turn in each chat.",
			Default:     "Reply with one short sentence.",
			Value:       serpent.StringOf(&prompt),
		},
		{
			Flag:        "turns",
			Description: "Number of user→assistant exchanges per chat conversation.",
			Default:     "10",
			Value:       serpent.Int64Of(&turns),
		},
		{
			Flag:        "follow-up-prompt",
			Description: "Text prompt to send for follow-up turns (turns 2 through N).",
			Default:     "Continue.",
			Value:       serpent.StringOf(&followUpPrompt),
		},
		{
			Flag:        "llm-mock-url",
			Description: "URL of the mock LLM server (e.g. http://127.0.0.1:8080/v1). When set, bootstraps an openai-compat chat provider and model config pointing at this URL.",
			Value:       serpent.StringOf(&llmMockURL),
		},
	}
	output.attach(&cmd.Options)
	tracingFlags.attach(&cmd.Options)
	prometheusFlags.attach(&cmd.Options)
	timeoutStrategy.attach(&cmd.Options)
	cleanupStrategy.attach(&cmd.Options)
	return cmd
}
