//go:build !slim

package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/signal"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/httpmw"
	notificationsLib "github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/createusers"
	"github.com/coder/coder/v2/scaletest/harness"
	"github.com/coder/coder/v2/scaletest/loadtestutil"
	"github.com/coder/coder/v2/scaletest/notifications"
	"github.com/coder/quartz"
	"github.com/coder/serpent"
)

// scaletestSetupIdleConnTimeout keeps pooled setup connections warm between
// requests so the bounded pool is reused across many user creations rather than
// re-dialed per request.
const scaletestSetupIdleConnTimeout = 60 * time.Second

// boundPool returns a transport configuration that caps the connection pool at
// limit, so a phase making many concurrent requests reuses a bounded set of
// connections instead of opening one per request.
func boundPool(limit int) func(*http.Transport) {
	return func(t *http.Transport) {
		t.MaxIdleConns = limit
		t.MaxIdleConnsPerHost = limit
		t.MaxConnsPerHost = limit
		t.IdleConnTimeout = scaletestSetupIdleConnTimeout
	}
}

// progressInterval is how often a long setup or cleanup phase reports progress.
const progressInterval = 15 * time.Second

// progress periodically reports how far a long phase has got. Reuse setup and
// cleanup can each run for minutes at scale, and an operator who reads silence as
// a hang may kill the process, which is the one exit that leaves promoted users
// and live tokens behind.
//
// The zero value reports nothing, which is what tests that do not care want.
type progress struct {
	w     io.Writer
	clock quartz.Clock
	verb  string
}

func newProgress(w io.Writer, verb string) progress {
	return progress{w: w, clock: quartz.NewReal(), verb: verb}
}

// watch reports on a timer until the returned function is called, which waits for
// the reporter to finish so its output cannot interleave with the caller's.
//
// The timer matters more than the count: if a phase wedges on one slow request
// the count stops advancing, and only a ticking line separates that from slow
// progress.
func (p progress) watch(ctx context.Context, total int, completed *atomic.Int64) (stop func()) {
	if p.w == nil || total == 0 {
		return func() {}
	}
	clock := p.clock
	if clock == nil {
		clock = quartz.NewReal()
	}
	ctx, cancel := context.WithCancel(ctx)
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		started := clock.Now()
		ticker := clock.NewTicker(progressInterval, "progress")
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_, _ = fmt.Fprintf(p.w, "  %s %d/%d users, %s elapsed\n",
					p.verb, completed.Load(), total, clock.Since(started).Round(time.Second))
			}
		}
	}()
	return func() {
		cancel()
		<-stopped
	}
}

func (r *RootCmd) scaletestNotifications() *serpent.Command {
	var (
		userCount               int64
		templateAdminPercentage float64
		notificationTimeout     time.Duration
		smtpRequestTimeout      time.Duration
		dialTimeout             time.Duration
		setupConcurrency        int64
		reuseUsers              bool
		noCleanup               bool
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
		Short: "Simulate notification delivery by connecting many users that listen for notifications.",
		Long: "By default this creates dedicated scaletest users and deletes them afterwards.\n" +
			"\n" +
			"With --reuse-users it instead reuses existing scaletest users, promoting a share of\n" +
			"them to template admin so they receive the triggered notifications, and restores them\n" +
			"afterwards. Reuse avoids the account-lifecycle notifications that creating and deleting\n" +
			"users generates, so it measures only the notifications under test.\n" +
			"\n" +
			"WARNING: --reuse-users is for dedicated or throwaway scaletest environments only. It\n" +
			"promotes real accounts to template admin and mints API tokens on them for the duration\n" +
			"of the run. Never use it against a live Coder deployment. Run only one instance of this\n" +
			"command against an environment at a time.",
		Handler: func(inv *serpent.Invocation) (retErr error) {
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

			if setupConcurrency <= 0 {
				return xerrors.Errorf("--setup-concurrency must be greater than 0")
			}

			if noCleanup && reuseUsers {
				return xerrors.Errorf("--no-cleanup cannot be used with --reuse-users: it would leave real users holding the template-admin role and a live API token")
			}

			if templateAdminPercentage < 0 || templateAdminPercentage > 100 {
				return xerrors.Errorf("--template-admin-percentage must be between 0 and 100")
			}

			if smtpAPIURL != "" && !strings.HasPrefix(smtpAPIURL, "http://") && !strings.HasPrefix(smtpAPIURL, "https://") {
				return xerrors.Errorf("--smtp-api-url must start with http:// or https://")
			}

			templateAdminCount := int64(float64(userCount) * templateAdminPercentage / 100)
			if templateAdminCount == 0 && templateAdminPercentage > 0 {
				templateAdminCount = 1
			}
			regularUserCount := userCount - templateAdminCount

			_, _ = fmt.Fprintf(inv.Stderr, "Distribution plan:\n")
			_, _ = fmt.Fprintf(inv.Stderr, "  Total users: %d\n", userCount)
			_, _ = fmt.Fprintf(inv.Stderr, "  Template admins: %d (%.1f%%)\n", templateAdminCount, templateAdminPercentage)
			_, _ = fmt.Fprintf(inv.Stderr, "  Regular users: %d (%.1f%%)\n", regularUserCount, 100.0-templateAdminPercentage)

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

			dialBarrier := &sync.WaitGroup{}
			templateAdminWatchBarrier := &sync.WaitGroup{}
			dialBarrier.Add(int(userCount))
			templateAdminWatchBarrier.Add(int(templateAdminCount))

			expectedNotificationIDs := map[uuid.UUID]struct{}{
				notificationsLib.TemplateTemplateDeleted: {},
			}

			triggerTimes := make(map[uuid.UUID]chan time.Time, len(expectedNotificationIDs))
			for id := range expectedNotificationIDs {
				triggerTimes[id] = make(chan time.Time, 1)
			}

			smtpHTTPTransport := &http.Transport{
				MaxConnsPerHost:     512,
				MaxIdleConnsPerHost: 512,
				IdleConnTimeout:     60 * time.Second,
			}
			smtpHTTPClient := &http.Client{
				Transport: smtpHTTPTransport,
			}

			// One shared client for all user setup (creation, login, role assignment,
			// reuse selection) and cleanup, with a connection pool bounded by
			// --setup-concurrency. This replaces the previous per-runner client, so the
			// number of setup connections stays bounded instead of scaling with
			// --user-count. Each runner still dials its own websocket on a separate
			// per-user client.
			setupClient, err := loadtestutil.DupClientConfiguringTransport(client, BypassHeader, boundPool(int(setupConcurrency)))
			if err != nil {
				return xerrors.Errorf("create setup client: %w", err)
			}

			// buildConfig assembles a runner config that is common to both modes. The
			// receiving users (template admins) watch for the triggered notification and
			// get the SMTP settings; regular users only hold a connection open.
			buildConfig := func(receiving bool) notifications.Config {
				config := notifications.Config{
					NotificationTimeout:   notificationTimeout,
					DialTimeout:           dialTimeout,
					DialBarrier:           dialBarrier,
					ReceivingWatchBarrier: templateAdminWatchBarrier,
					Metrics:               metrics,
				}
				if receiving {
					config.ExpectedNotificationsIDs = expectedNotificationIDs
					config.SMTPApiURL = smtpAPIURL
					config.SMTPRequestTimeout = smtpRequestTimeout
					config.SMTPHttpClient = smtpHTTPClient
				}
				return config
			}

			configs := make([]notifications.Config, 0, userCount)
			if reuseUsers {
				// Reuse mode: select existing scaletest users, promote a share to template
				// admin, and mint a token per user to connect as. Restore them afterwards.
				// See the command's long help for the dedicated-environment warning.
				reuse, err := setupReuseUsers(ctx, inv, setupClient, metrics, me.ID,
					int(userCount), int(templateAdminCount), int(setupConcurrency),
					dialTimeout+notificationTimeout+time.Hour)
				if err != nil {
					return err
				}
				if !noCleanup {
					// Restore even on failure: a partial setup may already have promoted
					// users or minted tokens. Detached from ctx so an interrupt that ends
					// the run does not also skip the restore.
					defer func() {
						cleanupCtx, cleanupCancel := cleanupStrategy.toContext(context.WithoutCancel(ctx))
						defer cleanupCancel()
						_, _ = fmt.Fprintln(inv.Stderr, "\nRestoring reused users...")
						if rerr := restoreUsers(cleanupCtx, inv.Stderr, setupClient, metrics, reuse.users, int(setupConcurrency)); rerr != nil {
							logger.Error(ctx, "failed to restore reused users", slog.Error(rerr))
							retErr = errors.Join(retErr, xerrors.Errorf("restore reused users: %w", rerr))
						}
					}()
				}
				_, _ = fmt.Fprintf(inv.Stderr, "Preparing %d users with %d concurrent workers...\n", len(reuse.users), setupConcurrency)
				if err := reuse.prepare(ctx); err != nil {
					return xerrors.Errorf("prepare reused users: %w", err)
				}
				for i := range reuse.users {
					config := buildConfig(reuse.users[i].promoted)
					config.PreCreatedUser = &reuse.users[i].user
					config.SessionToken = reuse.users[i].sessionToken
					if err := config.Validate(); err != nil {
						return xerrors.Errorf("validate config: %w", err)
					}
					configs = append(configs, config)
				}
			} else {
				_, _ = fmt.Fprintln(inv.Stderr, "Creating users...")
				for i := int64(0); i < userCount; i++ {
					config := buildConfig(i < templateAdminCount)
					config.User = createusers.Config{OrganizationID: me.OrganizationIDs[0]}
					if i < templateAdminCount {
						config.Roles = []string{codersdk.RoleTemplateAdmin}
					} else {
						config.Roles = []string{}
					}
					if err := config.Validate(); err != nil {
						return xerrors.Errorf("validate config: %w", err)
					}
					configs = append(configs, config)
				}
			}

			go triggerNotifications(
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
				var runner harness.Runnable = notifications.NewRunner(setupClient, config)
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

			// In reuse mode the runners create nothing, so harness cleanup is a no-op;
			// reused users are restored by the deferred restore registered during setup.
			if !noCleanup && !reuseUsers {
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
			Flag:        "setup-concurrency",
			Env:         "CODER_SCALETEST_NOTIFICATION_SETUP_CONCURRENCY",
			Default:     "10",
			Description: "Maximum number of concurrent connections used to create users, log them in, and assign roles. Bounds the shared setup connection pool so it does not grow with --user-count. Raise it to speed up setup for large runs.",
			Value:       serpent.Int64Of(&setupConcurrency),
		},
		{
			Flag:        "reuse-users",
			Env:         "CODER_SCALETEST_NOTIFICATION_REUSE_USERS",
			Description: "Reuse existing scaletest users instead of creating new ones, promoting a share to template admin and restoring them afterwards. DEDICATED OR THROWAWAY SCALETEST ENVIRONMENTS ONLY: it promotes real accounts and mints API tokens on them. Cannot be combined with --no-cleanup.",
			Value:       serpent.BoolOf(&reuseUsers),
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

// triggerNotifications waits for all test users to connect,
// then creates and deletes a test template to trigger notification events for testing.
func triggerNotifications(
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

	logger.Info(ctx, "creating test template to test notifications")

	// Upload empty template file.
	file, err := client.Upload(ctx, codersdk.ContentTypeTar, bytes.NewReader([]byte{}))
	if err != nil {
		logger.Error(ctx, "upload test template", slog.Error(err))
		return
	}
	logger.Info(ctx, "test template uploaded", slog.F("file_id", file.ID))

	// Create template version.
	version, err := client.CreateTemplateVersion(ctx, orgID, codersdk.CreateTemplateVersionRequest{
		StorageMethod: codersdk.ProvisionerStorageMethodFile,
		FileID:        file.ID,
		Provisioner:   codersdk.ProvisionerTypeEcho,
	})
	if err != nil {
		logger.Error(ctx, "create test template version", slog.Error(err))
		return
	}
	logger.Info(ctx, "test template version created", slog.F("template_version_id", version.ID))

	// Create template.
	testTemplate, err := client.CreateTemplate(ctx, orgID, codersdk.CreateTemplateRequest{
		Name:        "scaletest-test-template",
		Description: "scaletest-test-template",
		VersionID:   version.ID,
	})
	if err != nil {
		logger.Error(ctx, "create test template", slog.Error(err))
		return
	}
	logger.Info(ctx, "test template created", slog.F("template_id", testTemplate.ID))

	// Delete template to trigger notification.
	err = client.DeleteTemplate(ctx, testTemplate.ID)
	if err != nil {
		logger.Error(ctx, "delete test template", slog.Error(err))
		return
	}
	logger.Info(ctx, "test template deleted", slog.F("template_id", testTemplate.ID))

	// Record expected notification.
	expectedNotifications[notificationsLib.TemplateTemplateDeleted] <- time.Now()
	close(expectedNotifications[notificationsLib.TemplateTemplateDeleted])
}

// reuseUser is an existing scaletest user the run connects as instead of
// creating a new one. promoted marks the users designated as template admins for
// the run; sessionToken and tokenID are filled in by prepare.
type reuseUser struct {
	user         codersdk.User
	promoted     bool
	sessionToken string
	tokenID      string
}

// reuseState holds the users selected for a reuse run and the parameters needed
// to prepare and restore them.
type reuseState struct {
	client        *codersdk.Client
	metrics       *notifications.Metrics
	stderr        io.Writer
	users         []reuseUser
	tokenName     string
	tokenLifetime time.Duration
	concurrency   int
}

// setupReuseUsers selects userCount eligible existing scaletest users and marks
// the first adminCount as template admins for the run. It only selects; the
// caller registers cleanup and then calls prepare, so a partial preparation is
// still restored.
func setupReuseUsers(ctx context.Context, inv *serpent.Invocation, client *codersdk.Client, metrics *notifications.Metrics, excludeID uuid.UUID, userCount, adminCount, concurrency int, tokenLifetime time.Duration) (*reuseState, error) {
	_, _ = fmt.Fprintf(inv.Stderr, "Selecting %d existing users (%d template admins)...\n", userCount, adminCount)
	selected, err := selectExistingUsers(ctx, client, excludeID, userCount)
	if err != nil {
		return nil, err
	}
	users := make([]reuseUser, len(selected))
	for i, u := range selected {
		users[i] = reuseUser{user: u, promoted: i < adminCount}
	}
	return &reuseState{
		client:        client,
		metrics:       metrics,
		stderr:        inv.Stderr,
		users:         users,
		tokenName:     fmt.Sprintf("%s-notifications", loadtestutil.ScaleTestPrefix),
		tokenLifetime: tokenLifetime,
		concurrency:   concurrency,
	}, nil
}

// prepare promotes the designated users to template admin and mints a token for
// every user to connect as, at most concurrency at a time. The first failure
// cancels the rest.
func (s *reuseState) prepare(ctx context.Context) error {
	var completed atomic.Int64
	stopProgress := newProgress(s.stderr, "Prepared").watch(ctx, len(s.users), &completed)
	defer stopProgress()

	eg, egCtx := errgroup.WithContext(ctx)
	eg.SetLimit(s.concurrency)
	for i := range s.users {
		u := &s.users[i]
		eg.Go(func() error {
			if err := prepareReuseUser(egCtx, s.client, s.tokenName, s.tokenLifetime, u); err != nil {
				s.metrics.AddError("prepare_user")
				return xerrors.Errorf("prepare user %q: %w", u.user.ID, err)
			}
			completed.Add(1)
			return nil
		})
	}
	return eg.Wait()
}

// prepareReuseUser promotes the user to template admin when designated and mints
// a run-named, lifetime-bounded API token to connect as.
func prepareReuseUser(ctx context.Context, client *codersdk.Client, tokenName string, tokenLifetime time.Duration, u *reuseUser) error {
	if u.promoted {
		// Selection excludes users that already hold template-admin, so append it to
		// whatever roles they have. Restore removes only this role.
		roles := currentRoleNames(u.user)
		if _, err := client.UpdateUserRoles(ctx, u.user.ID.String(), codersdk.UpdateRoles{
			Roles: append(roles, codersdk.RoleTemplateAdmin),
		}); err != nil {
			return xerrors.Errorf("promote to template admin: %w", err)
		}
	}

	// Name the token after the run and bound its lifetime so a token orphaned by a
	// hard kill is identifiable and expires on its own.
	tokenRes, err := client.CreateToken(ctx, u.user.ID.String(), codersdk.CreateTokenRequest{
		TokenName: tokenName,
		Lifetime:  tokenLifetime,
	})
	if err != nil {
		return xerrors.Errorf("create token with lifetime %s (deployment max-token-lifetime must allow it): %w", tokenLifetime, err)
	}
	tokenID, _, err := httpmw.SplitAPIToken(tokenRes.Key)
	if err != nil {
		return xerrors.Errorf("parse minted token: %w", err)
	}
	u.sessionToken = tokenRes.Key
	u.tokenID = tokenID
	return nil
}

// restoreUsers revokes minted tokens and demotes the users this run promoted,
// best-effort: every user is attempted even when one fails, and the errors are
// joined. Reused accounts are never deleted.
func restoreUsers(ctx context.Context, stderr io.Writer, client *codersdk.Client, metrics *notifications.Metrics, users []reuseUser, concurrency int) error {
	var completed atomic.Int64
	stopProgress := newProgress(stderr, "Restored").watch(ctx, len(users), &completed)
	defer stopProgress()

	var (
		mu   sync.Mutex
		errs error
	)
	var eg errgroup.Group
	eg.SetLimit(concurrency)
	for i := range users {
		u := &users[i]
		eg.Go(func() error {
			if err := restoreReuseUser(ctx, client, u); err != nil {
				metrics.AddError("restore_user")
				mu.Lock()
				errs = errors.Join(errs, err)
				mu.Unlock()
			}
			completed.Add(1)
			return nil
		})
	}
	_ = eg.Wait()
	return errs
}

// restoreReuseUser revokes the user's minted token and removes the template-admin
// role if this run promoted them. It returns the joined errors so one failure
// does not skip the other step.
func restoreReuseUser(ctx context.Context, client *codersdk.Client, u *reuseUser) error {
	var errs error
	if u.tokenID != "" {
		if err := client.DeleteAPIKey(ctx, u.user.ID.String(), u.tokenID); err != nil {
			errs = errors.Join(errs, xerrors.Errorf("revoke token for user %q: %w", u.user.ID, err))
		}
	}
	if !u.promoted {
		return errs
	}
	remaining := slices.DeleteFunc(currentRoleNames(u.user), func(role string) bool {
		return role == codersdk.RoleTemplateAdmin
	})
	if _, err := client.UpdateUserRoles(ctx, u.user.ID.String(), codersdk.UpdateRoles{Roles: remaining}); err != nil {
		errs = errors.Join(errs, xerrors.Errorf("demote user %q: %w", u.user.ID, err))
	}
	return errs
}

// selectExistingUsers returns userCount existing scaletest users eligible for
// reuse, failing fast when too few are present. Eligible means an active,
// password-login user whose username has the scaletest prefix, excluding the
// caller, service accounts, owners, and existing template admins.
func selectExistingUsers(ctx context.Context, client *codersdk.Client, excludeID uuid.UUID, userCount int) ([]codersdk.User, error) {
	const pageSize = 1000
	pool := make([]codersdk.User, 0, userCount)
	for offset := 0; len(pool) < userCount; offset += pageSize {
		resp, err := client.Users(ctx, codersdk.UsersRequest{
			Status:    codersdk.UserStatusActive,
			LoginType: []codersdk.LoginType{codersdk.LoginTypePassword},
			// Narrow server-side so a deployment with many users does not page all of
			// them back. The exact prefix check below still decides eligibility.
			Search: loadtestutil.ScaleTestPrefix + "-",
			Pagination: codersdk.Pagination{
				Limit:  pageSize,
				Offset: offset,
			},
		})
		if err != nil {
			return nil, xerrors.Errorf("list users: %w", err)
		}
		for _, u := range resp.Users {
			if u.ID == excludeID || u.IsServiceAccount {
				continue
			}
			if !strings.HasPrefix(u.Username, loadtestutil.ScaleTestPrefix+"-") {
				continue
			}
			if userHasRole(u, codersdk.RoleOwner) || userHasRole(u, codersdk.RoleTemplateAdmin) {
				continue
			}
			pool = append(pool, u)
			if len(pool) == userCount {
				break
			}
		}
		if len(resp.Users) < pageSize {
			break
		}
	}

	if len(pool) < userCount {
		return nil, xerrors.Errorf("notifications scaletest requires %d eligible users but only %d are present: "+
			"eligible means an active %s- user with password login that is not the caller, a service account, "+
			"an owner, or already a template admin. Run without --reuse-users to create users instead",
			userCount, len(pool), loadtestutil.ScaleTestPrefix)
	}
	return pool, nil
}

func userHasRole(user codersdk.User, role string) bool {
	return slices.ContainsFunc(user.Roles, func(r codersdk.SlimRole) bool {
		return r.Name == role
	})
}

func currentRoleNames(user codersdk.User) []string {
	names := make([]string, len(user.Roles))
	for i, r := range user.Roles {
		names[i] = r.Name
	}
	return names
}
