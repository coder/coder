//go:build !slim

package cli

import (
	"context"
	"fmt"
	"net/http"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/xerrors"

	"cdr.dev/slog"

	notificationsLib "github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/createusers"
	"github.com/coder/coder/v2/scaletest/harness"
	"github.com/coder/coder/v2/scaletest/notifications"
	"github.com/coder/serpent"
)

func (r *RootCmd) scaletestNotifications() *serpent.Command {
	var (
		userCount           int64
		ownerUserPercentage float64
		notificationTimeout time.Duration
		dialTimeout         time.Duration
		noCleanup           bool
		smtpAPIURL          string

		tracingFlags = &scaletestTracingFlags{}

		// This test requires unlimited concurrency.
		timeoutStrategy = &timeoutFlags{}
		cleanupStrategy = newScaletestCleanupStrategy()
		output          = &scaletestOutputFlags{}
		prometheusFlags = &scaletestPrometheusFlags{}
	)

	cmd := &serpent.Command{
		Use:   "notifications",
		Short: "Simulate notification delivery by creating many users listening to notifications.",
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			notifyCtx, stop := signal.NotifyContext(ctx, StopSignals...)
			defer stop()
			ctx = notifyCtx

			me, err := requireAdmin(ctx, client)
			if err != nil {
				return err
			}

			client.HTTPClient = &http.Client{
				Transport: &codersdk.HeaderTransport{
					Transport: http.DefaultTransport,
					Header: map[string][]string{
						codersdk.BypassRatelimitHeader: {"true"},
					},
				},
			}

			if userCount <= 0 {
				return xerrors.Errorf("--user-count must be greater than 0")
			}

			if ownerUserPercentage < 0 || ownerUserPercentage > 100 {
				return xerrors.Errorf("--owner-user-percentage must be between 0 and 100")
			}

			if smtpAPIURL != "" && !strings.HasPrefix(smtpAPIURL, "http://") && !strings.HasPrefix(smtpAPIURL, "https://") {
				return xerrors.Errorf("--smtp-api-url must start with http:// or https://")
			}

			ownerUserCount := int64(float64(userCount) * ownerUserPercentage / 100)
			if ownerUserCount == 0 && ownerUserPercentage > 0 {
				ownerUserCount = 1
			}
			regularUserCount := userCount - ownerUserCount

			_, _ = fmt.Fprintf(inv.Stderr, "Distribution plan:\n")
			_, _ = fmt.Fprintf(inv.Stderr, "  Total users: %d\n", userCount)
			_, _ = fmt.Fprintf(inv.Stderr, "  Owner users: %d (%.1f%%)\n", ownerUserCount, ownerUserPercentage)
			_, _ = fmt.Fprintf(inv.Stderr, "  Regular users: %d (%.1f%%)\n", regularUserCount, 100.0-ownerUserPercentage)

			outputs, err := output.parse()
			if err != nil {
				return xerrors.Errorf("could not parse --output flags")
			}

			tracerProvider, closeTracing, tracingEnabled, err := tracingFlags.provider(ctx)
			if err != nil {
				return xerrors.Errorf("create tracer provider: %w", err)
			}
			tracer := tracerProvider.Tracer(scaletestTracerName)

			reg := prometheus.NewRegistry()
			metrics := notifications.NewMetrics(reg)

			logger := inv.Logger
			prometheusSrvClose := ServeHandler(ctx, logger, promhttp.HandlerFor(reg, promhttp.HandlerOpts{}), prometheusFlags.Address, "prometheus")
			defer prometheusSrvClose()

			defer func() {
				_, _ = fmt.Fprintln(inv.Stderr, "\nUploading traces...")
				if err := closeTracing(ctx); err != nil {
					_, _ = fmt.Fprintf(inv.Stderr, "\nError uploading traces: %+v\n", err)
				}
				// Wait for prometheus metrics to be scraped
				_, _ = fmt.Fprintf(inv.Stderr, "Waiting %s for prometheus metrics to be scraped\n", prometheusFlags.Wait)
				<-time.After(prometheusFlags.Wait)
			}()

			_, _ = fmt.Fprintln(inv.Stderr, "Creating users...")

			dialBarrier := &sync.WaitGroup{}
			ownerWatchBarrier := &sync.WaitGroup{}
			dialBarrier.Add(int(userCount))
			ownerWatchBarrier.Add(int(ownerUserCount))

			expectedNotificationIDs := map[uuid.UUID]struct{}{
				notificationsLib.TemplateUserAccountCreated: {},
				notificationsLib.TemplateUserAccountDeleted: {},
			}

			triggerTimes := make(map[uuid.UUID]chan time.Time, len(expectedNotificationIDs))
			for id := range expectedNotificationIDs {
				triggerTimes[id] = make(chan time.Time, 1)
			}

			configs := make([]notifications.Config, 0, userCount)
			for range ownerUserCount {
				config := notifications.Config{
					User: createusers.Config{
						OrganizationID: me.OrganizationIDs[0],
					},
					Roles:                    []string{codersdk.RoleOwner},
					NotificationTimeout:      notificationTimeout,
					DialTimeout:              dialTimeout,
					DialBarrier:              dialBarrier,
					ReceivingWatchBarrier:    ownerWatchBarrier,
					ExpectedNotificationsIDs: expectedNotificationIDs,
					Metrics:                  metrics,
					SMTPApiURL:               smtpAPIURL,
				}
				if err := config.Validate(); err != nil {
					return xerrors.Errorf("validate config: %w", err)
				}
				configs = append(configs, config)
			}
			for range regularUserCount {
				config := notifications.Config{
					User: createusers.Config{
						OrganizationID: me.OrganizationIDs[0],
					},
					Roles:                 []string{},
					NotificationTimeout:   notificationTimeout,
					DialTimeout:           dialTimeout,
					DialBarrier:           dialBarrier,
					ReceivingWatchBarrier: ownerWatchBarrier,
					Metrics:               metrics,
					SMTPApiURL:            smtpAPIURL,
				}
				if err := config.Validate(); err != nil {
					return xerrors.Errorf("validate config: %w", err)
				}
				configs = append(configs, config)
			}

			go triggerUserNotifications(
				ctx,
				logger,
				client,
				me.OrganizationIDs[0],
				dialBarrier,
				dialTimeout,
				triggerTimes,
			)

			th := harness.NewTestHarness(timeoutStrategy.wrapStrategy(harness.ConcurrentExecutionStrategy{}), cleanupStrategy.toStrategy())

			for i, config := range configs {
				id := strconv.Itoa(i)
				name := fmt.Sprintf("notifications-%s", id)
				var runner harness.Runnable = notifications.NewRunner(client, config)
				if tracingEnabled {
					runner = &runnableTraceWrapper{
						tracer:   tracer,
						spanName: name,
						runner:   runner,
					}
				}

				th.AddRun(name, id, runner)
			}

			_, _ = fmt.Fprintln(inv.Stderr, "Running notification delivery scaletest...")
			testCtx, testCancel := timeoutStrategy.toContext(ctx)
			defer testCancel()
			err = th.Run(testCtx)
			if err != nil {
				return xerrors.Errorf("run test harness (harness failure, not a test failure): %w", err)
			}

			// If the command was interrupted, skip stats.
			if notifyCtx.Err() != nil {
				return notifyCtx.Err()
			}

			res := th.Results()

			if err := computeNotificationLatencies(ctx, logger, triggerTimes, res, metrics); err != nil {
				return xerrors.Errorf("compute notification latencies: %w", err)
			}

			for _, o := range outputs {
				err = o.write(res, inv.Stdout)
				if err != nil {
					return xerrors.Errorf("write output %q to %q: %w", o.format, o.path, err)
				}
			}

			if !noCleanup {
				_, _ = fmt.Fprintln(inv.Stderr, "\nCleaning up...")
				cleanupCtx, cleanupCancel := cleanupStrategy.toContext(ctx)
				defer cleanupCancel()
				err = th.Cleanup(cleanupCtx)
				if err != nil {
					return xerrors.Errorf("cleanup tests: %w", err)
				}
			}

			if res.TotalFail > 0 {
				return xerrors.New("load test failed, see above for more details")
			}

			return nil
		},
	}

	cmd.Options = serpent.OptionSet{
		{
			Flag:          "user-count",
			FlagShorthand: "c",
			Env:           "CODER_SCALETEST_NOTIFICATION_USER_COUNT",
			Description:   "Required: Total number of users to create.",
			Value:         serpent.Int64Of(&userCount),
			Required:      true,
		},
		{
			Flag:        "owner-user-percentage",
			Env:         "CODER_SCALETEST_NOTIFICATION_OWNER_USER_PERCENTAGE",
			Default:     "20.0",
			Description: "Percentage of users to assign Owner role to (0-100).",
			Value:       serpent.Float64Of(&ownerUserPercentage),
		},
		{
			Flag:        "notification-timeout",
			Env:         "CODER_SCALETEST_NOTIFICATION_TIMEOUT",
			Default:     "5m",
			Description: "How long to wait for notifications after triggering.",
			Value:       serpent.DurationOf(&notificationTimeout),
		},
		{
			Flag:        "dial-timeout",
			Env:         "CODER_SCALETEST_DIAL_TIMEOUT",
			Default:     "2m",
			Description: "Timeout for dialing the notification websocket endpoint.",
			Value:       serpent.DurationOf(&dialTimeout),
		},
		{
			Flag:        "no-cleanup",
			Env:         "CODER_SCALETEST_NO_CLEANUP",
			Description: "Do not clean up resources after the test completes.",
			Value:       serpent.BoolOf(&noCleanup),
		},
		{
			Flag:        "smtp-api-url",
			Env:         "CODER_SCALETEST_SMTP_API_URL",
			Description: "SMTP mock HTTP API address.",
			Value:       serpent.StringOf(&smtpAPIURL),
		},
	}

	tracingFlags.attach(&cmd.Options)
	timeoutStrategy.attach(&cmd.Options)
	cleanupStrategy.attach(&cmd.Options)
	output.attach(&cmd.Options)
	prometheusFlags.attach(&cmd.Options)
	return cmd
}

func computeNotificationLatencies(
	ctx context.Context,
	logger slog.Logger,
	expectedNotifications map[uuid.UUID]chan time.Time,
	results harness.Results,
	metrics *notifications.Metrics,
) error {
	triggerTimes := make(map[uuid.UUID]time.Time)
	for notificationID, triggerTimeChan := range expectedNotifications {
		select {
		case triggerTime := <-triggerTimeChan:
			triggerTimes[notificationID] = triggerTime
			logger.Info(ctx, "received trigger time",
				slog.F("notification_id", notificationID),
				slog.F("trigger_time", triggerTime))
		default:
			logger.Warn(ctx, "no trigger time received for notification",
				slog.F("notification_id", notificationID))
		}
	}

	if len(triggerTimes) == 0 {
		logger.Warn(ctx, "no trigger times available, skipping latency computation")
		return nil
	}

	var totalLatencies int
	for runID, runResult := range results.Runs {
		if runResult.Error != nil {
			logger.Debug(ctx, "skipping failed run for latency computation",
				slog.F("run_id", runID))
			continue
		}

		if runResult.Metrics == nil {
			continue
		}

		// Process websocket notifications.
		if wsReceiptTimes, ok := runResult.Metrics[notifications.WebsocketNotificationReceiptTimeMetric].(map[uuid.UUID]time.Time); ok {
			for notificationID, receiptTime := range wsReceiptTimes {
				if triggerTime, ok := triggerTimes[notificationID]; ok {
					latency := receiptTime.Sub(triggerTime)
					metrics.RecordLatency(latency, notificationID.String(), notifications.NotificationTypeWebsocket)
					totalLatencies++
					logger.Debug(ctx, "computed websocket latency",
						slog.F("run_id", runID),
						slog.F("notification_id", notificationID),
						slog.F("latency", latency))
				}
			}
		}

		// Process SMTP notifications
		if smtpReceiptTimes, ok := runResult.Metrics[notifications.SMTPNotificationReceiptTimeMetric].(map[uuid.UUID]time.Time); ok {
			for notificationID, receiptTime := range smtpReceiptTimes {
				if triggerTime, ok := triggerTimes[notificationID]; ok {
					latency := receiptTime.Sub(triggerTime)
					metrics.RecordLatency(latency, notificationID.String(), notifications.NotificationTypeSMTP)
					totalLatencies++
					logger.Debug(ctx, "computed SMTP latency",
						slog.F("run_id", runID),
						slog.F("notification_id", notificationID),
						slog.F("latency", latency))
				}
			}
		}
	}

	logger.Info(ctx, "finished computing notification latencies",
		slog.F("total_runs", results.TotalRuns),
		slog.F("total_latencies_computed", totalLatencies))

	return nil
}

// triggerUserNotifications waits for all test users to connect,
// then creates and deletes a test user to trigger notification events for testing.
func triggerUserNotifications(
	ctx context.Context,
	logger slog.Logger,
	client *codersdk.Client,
	orgID uuid.UUID,
	dialBarrier *sync.WaitGroup,
	dialTimeout time.Duration,
	expectedNotifications map[uuid.UUID]chan time.Time,
) {
	logger.Info(ctx, "waiting for all users to connect")

	// Wait for all users to connect
	waitCtx, cancel := context.WithTimeout(ctx, dialTimeout+30*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		dialBarrier.Wait()
		close(done)
	}()

	select {
	case <-done:
		logger.Info(ctx, "all users connected")
	case <-waitCtx.Done():
		if waitCtx.Err() == context.DeadlineExceeded {
			logger.Error(ctx, "timeout waiting for users to connect")
		} else {
			logger.Info(ctx, "context canceled while waiting for users")
		}
		return
	}

	const (
		triggerUsername = "scaletest-trigger-user"
		triggerEmail    = "scaletest-trigger@example.com"
	)

	logger.Info(ctx, "creating test user to test notifications",
		slog.F("username", triggerUsername),
		slog.F("email", triggerEmail),
		slog.F("org_id", orgID))

	testUser, err := client.CreateUserWithOrgs(ctx, codersdk.CreateUserRequestWithOrgs{
		OrganizationIDs: []uuid.UUID{orgID},
		Username:        triggerUsername,
		Email:           triggerEmail,
		Password:        "test-password-123",
	})
	if err != nil {
		logger.Error(ctx, "create test user", slog.Error(err))
		return
	}
	expectedNotifications[notificationsLib.TemplateUserAccountCreated] <- time.Now()

	err = client.DeleteUser(ctx, testUser.ID)
	if err != nil {
		logger.Error(ctx, "delete test user", slog.Error(err))
		return
	}
	expectedNotifications[notificationsLib.TemplateUserAccountDeleted] <- time.Now()
	close(expectedNotifications[notificationsLib.TemplateUserAccountCreated])
	close(expectedNotifications[notificationsLib.TemplateUserAccountDeleted])
}
