//go:build !slim

package cli

import (
	"bytes"
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

	"cdr.dev/slog/v3"
	notificationsLib "github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/harness"
	"github.com/coder/coder/v2/scaletest/loadtestutil"
	"github.com/coder/coder/v2/scaletest/notifications"
	"github.com/coder/serpent"
)

func (r *RootCmd) scaletestNotifications() *serpent.Command {
	var (
		userCount               int64
		templateAdminPercentage float64
		templateDeletionCount   int64
		notificationTimeout     time.Duration
		smtpRequestTimeout      time.Duration
		dialTimeout             time.Duration
		noCleanup               bool
		usernameInfix           string
		smtpAPIURL              string

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

			me, err := RequireAdmin(ctx, client)
			if err != nil {
				return err
			}

			if userCount <= 0 {
				return xerrors.Errorf("--user-count must be greater than 0")
			}

			if templateAdminPercentage < 0 || templateAdminPercentage > 100 {
				return xerrors.Errorf("--template-admin-percentage must be between 0 and 100")
			}

			if templateDeletionCount < 1 {
				return xerrors.Errorf("--template-deletion-count must be at least 1")
			}

			if smtpAPIURL != "" && !strings.HasPrefix(smtpAPIURL, "http://") && !strings.HasPrefix(smtpAPIURL, "https://") {
				return xerrors.Errorf("--smtp-api-url must start with http:// or https://")
			}

			templateAdminCount := int64(float64(userCount) * templateAdminPercentage / 100)
			if templateAdminCount == 0 && templateAdminPercentage > 0 {
				templateAdminCount = 1
			}
			regularUserCount := userCount - templateAdminCount
			totalExpectedNotifications := templateAdminCount * templateDeletionCount

			_, _ = fmt.Fprintf(inv.Stderr, "Distribution plan:\n")
			_, _ = fmt.Fprintf(inv.Stderr, "  Total users: %d\n", userCount)
			_, _ = fmt.Fprintf(inv.Stderr, "  Template admins: %d (%.1f%%)\n", templateAdminCount, templateAdminPercentage)
			_, _ = fmt.Fprintf(inv.Stderr, "  Regular users: %d (%.1f%%)\n", regularUserCount, 100.0-templateAdminPercentage)
			_, _ = fmt.Fprintf(inv.Stderr, "  Template deletions: %d\n", templateDeletionCount)
			_, _ = fmt.Fprintf(inv.Stderr, "  Notifications per template admin: %d\n", templateDeletionCount)
			_, _ = fmt.Fprintf(inv.Stderr, "  Total expected notifications: %d\n", totalExpectedNotifications)

			outputs, err := output.parse()
			if err != nil {
				return xerrors.Errorf("parse --output flags: %w", err)
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

			var adminReuse, regularReuse []notificationReuseUser
			_, _ = fmt.Fprintln(inv.Stderr, "Reusing existing scaletest users...")
			// Bound token lifetime to just beyond the run so tokens orphaned by a
			// hard kill expire quickly rather than at the deployment default.
			tokenLifetime := notificationTimeout + dialTimeout + time.Hour
			adminReuse, regularReuse, err = selectNotificationReuseUsers(ctx, client, usernameInfix, int(templateAdminCount), int(regularUserCount), tokenLifetime)
			if err != nil {
				return err
			}

			dialBarrier := &sync.WaitGroup{}
			templateAdminWatchBarrier := &sync.WaitGroup{}
			dialBarrier.Add(int(userCount))
			templateAdminWatchBarrier.Add(int(templateAdminCount))

			triggerCh := make(chan time.Time, 1)

			smtpHTTPTransport := &http.Transport{
				MaxConnsPerHost:     512,
				MaxIdleConnsPerHost: 512,
				IdleConnTimeout:     60 * time.Second,
			}
			smtpHTTPClient := &http.Client{
				Transport: smtpHTTPTransport,
			}

			configs := make([]notifications.Config, 0, userCount)
			for i := range int(templateAdminCount) {
				config := notifications.Config{
					NotificationTimeout:   notificationTimeout,
					DialTimeout:           dialTimeout,
					DialBarrier:           dialBarrier,
					ReceivingWatchBarrier: templateAdminWatchBarrier,
					IsTemplateAdmin:       true,
					ExpectedDeletions:     int(templateDeletionCount),
					Metrics:               metrics,
					SMTPApiURL:            smtpAPIURL,
					SMTPRequestTimeout:    smtpRequestTimeout,
					SMTPHttpClient:        smtpHTTPClient,
					SessionToken:          adminReuse[i].sessionToken,
					PreCreatedUser:        adminReuse[i].user,
				}
				if err := config.Validate(); err != nil {
					return xerrors.Errorf("validate config: %w", err)
				}
				configs = append(configs, config)
			}
			for i := range int(regularUserCount) {
				config := notifications.Config{
					NotificationTimeout:   notificationTimeout,
					DialTimeout:           dialTimeout,
					DialBarrier:           dialBarrier,
					ReceivingWatchBarrier: templateAdminWatchBarrier,
					Metrics:               metrics,
					SessionToken:          regularReuse[i].sessionToken,
					PreCreatedUser:        regularReuse[i].user,
				}
				if err := config.Validate(); err != nil {
					return xerrors.Errorf("validate config: %w", err)
				}
				configs = append(configs, config)
			}

			go triggerNotifications(
				ctx,
				logger,
				client,
				me.OrganizationIDs[0],
				dialBarrier,
				dialTimeout,
				int(templateDeletionCount),
				triggerCh,
			)

			th := harness.NewTestHarness(timeoutStrategy.wrapStrategy(harness.ConcurrentExecutionStrategy{}), cleanupStrategy.toStrategy())

			for i, config := range configs {
				id := strconv.Itoa(i)
				name := fmt.Sprintf("notifications-%s", id)
				// use an independent client for each Runner, so they don't reuse TCP connections. This can lead to
				// requests being unbalanced among Coder instances.
				runnerClient, err := loadtestutil.DupClientCopyingHeaders(client, BypassHeader)
				if err != nil {
					return xerrors.Errorf("create runner client: %w", err)
				}
				var runner harness.Runnable = notifications.NewRunner(runnerClient, config)
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

			// triggerNotifications sends exactly one result; block to avoid a race.
			triggerTime := <-triggerCh
			if err := computeNotificationLatencies(ctx, logger, triggerTime, res, metrics); err != nil {
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
			Flag:        "template-admin-percentage",
			Env:         "CODER_SCALETEST_NOTIFICATION_TEMPLATE_ADMIN_PERCENTAGE",
			Default:     "20.0",
			Description: "Percentage of users to assign Template Admin role to (0-100).",
			Value:       serpent.Float64Of(&templateAdminPercentage),
		},
		{
			Flag:        "template-deletion-count",
			Env:         "CODER_SCALETEST_NOTIFICATION_TEMPLATE_DELETION_COUNT",
			Default:     "1",
			Description: "Number of templates to create and then delete to trigger notifications. Each deletion notifies every template admin, so the total number of notifications is this value multiplied by the number of template admins.",
			Value:       serpent.Int64Of(&templateDeletionCount),
		},
		{
			Flag:        "notification-timeout",
			Env:         "CODER_SCALETEST_NOTIFICATION_TIMEOUT",
			Default:     "10m",
			Description: "How long to wait for notifications after triggering.",
			Value:       serpent.DurationOf(&notificationTimeout),
		},
		{
			Flag:        "smtp-request-timeout",
			Env:         "CODER_SCALETEST_SMTP_REQUEST_TIMEOUT",
			Default:     "5m",
			Description: "Timeout for SMTP requests.",
			Value:       serpent.DurationOf(&smtpRequestTimeout),
		},
		{
			Flag:        "dial-timeout",
			Env:         "CODER_SCALETEST_DIAL_TIMEOUT",
			Default:     "10m",
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
			Flag:        "username-infix",
			Env:         "CODER_SCALETEST_NOTIFICATION_USERNAME_INFIX",
			Description: "Username infix identifying the user pool to reuse. It must match the --username-infix used by the create-users run that provisioned the pool: for example asdf selects users named scaletest-asdf-<random>-<id> so notifications reuses only its own pool and does not compete with other load generators. Leave empty to select any scaletest- user.",
			Value:       serpent.StringOf(&usernameInfix),
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

// deletionNotificationID is the only notification this generator triggers.
var deletionNotificationID = notificationsLib.TemplateTemplateDeleted.String()

func computeNotificationLatencies(
	ctx context.Context,
	logger slog.Logger,
	triggerTime time.Time,
	results harness.Results,
	metrics *notifications.Metrics,
) error {
	if triggerTime.IsZero() {
		logger.Warn(ctx, "no trigger time available, skipping latency computation")
		return nil
	}

	logger.Info(ctx, "computing notification latencies", slog.F("trigger_time", triggerTime))

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

		// Measured against the batch trigger time (see triggerNotifications).
		if wsReceiptTimes, ok := runResult.Metrics[notifications.WebsocketNotificationReceiptTimeMetric].([]time.Time); ok {
			for _, receiptTime := range wsReceiptTimes {
				latency := receiptTime.Sub(triggerTime)
				metrics.RecordLatency(latency, deletionNotificationID, notifications.NotificationTypeWebsocket)
				totalLatencies++
				logger.Debug(ctx, "computed websocket latency",
					slog.F("run_id", runID),
					slog.F("latency", latency))
			}
		}

		if smtpReceiptTimes, ok := runResult.Metrics[notifications.SMTPNotificationReceiptTimeMetric].([]time.Time); ok {
			for _, receiptTime := range smtpReceiptTimes {
				latency := receiptTime.Sub(triggerTime)
				metrics.RecordLatency(latency, deletionNotificationID, notifications.NotificationTypeSMTP)
				totalLatencies++
				logger.Debug(ctx, "computed SMTP latency",
					slog.F("run_id", runID),
					slog.F("latency", latency))
			}
		}
	}

	logger.Info(ctx, "finished computing notification latencies",
		slog.F("total_runs", results.TotalRuns),
		slog.F("total_latencies_computed", totalLatencies))

	return nil
}

const notificationsTemplatePrefix = "scaletest-test-template-"

// deleteScaletestNotificationTemplates best-effort deletes this generator's
// templates by name prefix, logging failures instead of returning them.
func deleteScaletestNotificationTemplates(ctx context.Context, logger slog.Logger, client *codersdk.Client, orgID uuid.UUID) {
	templates, err := client.Templates(ctx, codersdk.TemplateFilter{
		OrganizationID: orgID,
		FuzzyName:      notificationsTemplatePrefix,
	})
	if err != nil {
		logger.Error(ctx, "list scaletest templates for cleanup", slog.Error(err))
		return
	}
	for _, tmpl := range templates {
		// FuzzyName is a substring match, so guard against unrelated templates.
		if !strings.HasPrefix(tmpl.Name, notificationsTemplatePrefix) {
			continue
		}
		if err := client.DeleteTemplate(ctx, tmpl.ID); err != nil {
			logger.Error(ctx, "delete scaletest template", slog.Error(err), slog.F("template_id", tmpl.ID), slog.F("name", tmpl.Name))
			continue
		}
		logger.Info(ctx, "deleted scaletest template", slog.F("template_id", tmpl.ID), slog.F("name", tmpl.Name))
	}
}

// triggerNotifications waits for all users to connect, then creates and deletes
// deletionCount templates. Each deletion notifies every template admin.
func triggerNotifications(
	ctx context.Context,
	logger slog.Logger,
	client *codersdk.Client,
	orgID uuid.UUID,
	dialBarrier *sync.WaitGroup,
	dialTimeout time.Duration,
	deletionCount int,
	triggerCh chan<- time.Time,
) {
	// Always send exactly one batch-start time so the caller can block for it; a
	// failure before any deletion sends the zero time.
	var batchStart time.Time
	defer func() { triggerCh <- batchStart }()

	// Clean up on any exit. These deletes also fire notifications, but the test
	// is done by now so they are harmless.
	defer func() {
		// Fresh context so cleanup runs even if ctx was canceled (e.g. interrupt).
		cleanupCtx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		deleteScaletestNotificationTemplates(cleanupCtx, logger, client, orgID)
	}()

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

	logger.Info(ctx, "creating test templates to trigger notifications", slog.F("count", deletionCount))

	// The echo provisioner ignores contents, so reuse one empty archive.
	file, err := client.Upload(ctx, codersdk.ContentTypeTar, bytes.NewReader([]byte{}))
	if err != nil {
		logger.Error(ctx, "upload test template", slog.Error(err))
		return
	}
	logger.Info(ctx, "test template uploaded", slog.F("file_id", file.ID))

	// Create all templates first so the deletions happen back to back.
	templateIDs := make([]uuid.UUID, 0, deletionCount)
	for i := range deletionCount {
		version, err := client.CreateTemplateVersion(ctx, orgID, codersdk.CreateTemplateVersionRequest{
			StorageMethod: codersdk.ProvisionerStorageMethodFile,
			FileID:        file.ID,
			Provisioner:   codersdk.ProvisionerTypeEcho,
		})
		if err != nil {
			logger.Error(ctx, "create test template version", slog.Error(err), slog.F("index", i))
			return
		}

		templateName := fmt.Sprintf("%s%d", notificationsTemplatePrefix, i)
		testTemplate, err := client.CreateTemplate(ctx, orgID, codersdk.CreateTemplateRequest{
			Name:        templateName,
			Description: "scaletest-test-template",
			VersionID:   version.ID,
		})
		if err != nil {
			logger.Error(ctx, "create test template", slog.Error(err), slog.F("index", i), slog.F("name", templateName))
			return
		}
		templateIDs = append(templateIDs, testTemplate.ID)
	}
	logger.Info(ctx, "test templates created", slog.F("count", len(templateIDs)))

	// Capture batchStart once, just before the deletions; every receipt is
	// measured against it. The ms-scale drift is fine because delivery takes
	// minutes at scale.
	batchStart = time.Now()
	for _, templateID := range templateIDs {
		if err := client.DeleteTemplate(ctx, templateID); err != nil {
			logger.Error(ctx, "delete test template", slog.Error(err), slog.F("template_id", templateID))
			return
		}
		logger.Info(ctx, "test template deleted", slog.F("template_id", templateID))
	}
}

type notificationReuseUser struct {
	user         codersdk.User
	sessionToken string
}

// selectNotificationReuseUsers selects existing scaletest users belonging to the
// pool identified by usernameInfix and mints a token for each, erroring if the
// pool lacks enough template admins or regular users. usernameInfix is inserted
// between the mandatory "scaletest-" root and the rest of the username (empty
// selects any scaletest- user), so it isolates this run from other load
// generators that reuse scaletest users concurrently.
func selectNotificationReuseUsers(ctx context.Context, client *codersdk.Client, usernameInfix string, adminCount, regularCount int, tokenLifetime time.Duration) (admins, regulars []notificationReuseUser, err error) {
	// The scaletest- root is always kept so selection matches the users that
	// create-users provisions; usernameInfix, when set, narrows to that pool.
	searchPrefix := loadtestutil.ScaleTestPrefix + "-"
	if usernameInfix != "" {
		searchPrefix += usernameInfix + "-"
	}
	users, err := getScaletestUsersWithPrefix(ctx, client, searchPrefix)
	if err != nil {
		return nil, nil, xerrors.Errorf("list scaletest users: %w", err)
	}

	var adminUsers, regularUsers []codersdk.User
	for _, u := range users {
		if userHasRole(u, codersdk.RoleTemplateAdmin) {
			adminUsers = append(adminUsers, u)
		} else {
			regularUsers = append(regularUsers, u)
		}
	}

	if len(adminUsers) < adminCount || len(regularUsers) < regularCount {
		total := adminCount + regularCount
		var pct float64
		if total > 0 {
			pct = float64(adminCount) / float64(total) * 100
		}
		hint := fmt.Sprintf("coder exp scaletest create-users --count %d --template-admin-percentage %.2f --no-cleanup", total, pct)
		if usernameInfix != "" {
			hint = fmt.Sprintf("coder exp scaletest create-users --count %d --template-admin-percentage %.2f --username-infix %q --no-cleanup", total, pct, usernameInfix)
		}
		return nil, nil, xerrors.Errorf(
			"not enough scaletest users to reuse: found %d template admins and %d regular users, need %d and %d. "+
				"Create them first, for example: %s",
			len(adminUsers), len(regularUsers), adminCount, regularCount, hint)
	}

	// Token names are auto-generated by the server; we only need the returned key.
	mint := func(selected []codersdk.User) ([]notificationReuseUser, error) {
		reuse := make([]notificationReuseUser, 0, len(selected))
		for _, u := range selected {
			res, err := client.CreateToken(ctx, u.ID.String(), codersdk.CreateTokenRequest{
				Lifetime: tokenLifetime,
			})
			if err != nil {
				return nil, xerrors.Errorf("mint token for user %q: %w", u.Username, err)
			}
			reuse = append(reuse, notificationReuseUser{user: u, sessionToken: res.Key})
		}
		return reuse, nil
	}

	if admins, err = mint(adminUsers[:adminCount]); err != nil {
		return nil, nil, err
	}
	if regulars, err = mint(regularUsers[:regularCount]); err != nil {
		return nil, nil, err
	}

	return admins, regulars, nil
}

func userHasRole(u codersdk.User, roleName string) bool {
	for _, role := range u.Roles {
		if role.Name == roleName {
			return true
		}
	}
	return false
}
