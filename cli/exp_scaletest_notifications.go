//go:build !slim

package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
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
	"github.com/coder/coder/v2/cryptorand"
	"github.com/coder/coder/v2/scaletest/createusers"
	"github.com/coder/coder/v2/scaletest/harness"
	"github.com/coder/coder/v2/scaletest/loadtestutil"
	"github.com/coder/coder/v2/scaletest/notifications"
	"github.com/coder/quartz"
	"github.com/coder/serpent"
)

// notificationsPrefix prefixes every artifact this command names: the trigger
// template and the minted API tokens. Residue from a hard kill is greppable by it,
// and it is a subset of what "coder exp scaletest cleanup" reclaims.
const notificationsPrefix = loadtestutil.ScaleTestPrefix + "-notifications-"

// runArtifactName returns the name this run gives its artifacts. One name covers
// the template and the tokens: both belong to the run, and one string means one
// thing to grep for.
func runArtifactName(runID string) string {
	return notificationsPrefix + runID
}

// scaletestIdleConnTimeout keeps pooled connections warm across the setup and
// SMTP request phases instead of re-dialing per request.
const scaletestIdleConnTimeout = 60 * time.Second

// progressInterval is how often a long setup or cleanup phase reports progress.
const progressInterval = 15 * time.Second

// defaultPhaseTimeout bounds the setup and cleanup phases. Both flags take their
// default from here so they cannot drift apart: the two phases do the same amount
// of per-user work, so a value that suits one suits the other.
const defaultPhaseTimeout = 30 * time.Minute

func (r *RootCmd) scaletestNotifications() *serpent.Command {
	var (
		userCount               int64
		templateAdminPercentage float64
		setupConcurrency        int64
		setupTimeout            time.Duration
		testTimeout             time.Duration
		smtpRequestTimeout      time.Duration
		dialTimeout             time.Duration
		cleanupTimeout          time.Duration
		noCleanup               bool
		reuseUsers              bool
		smtpAPIURL              string

		tracingFlags    = &scaletestTracingFlags{}
		output          = &scaletestOutputFlags{}
		prometheusFlags = &scaletestPrometheusFlags{}
	)

	cmd := &serpent.Command{
		Use:   "notifications",
		Short: "Simulate notification delivery by connecting many users that listen for notifications.",
		Long: "By default this creates dedicated scaletest users and deletes them afterwards. With\n" +
			"--reuse-users it instead reuses existing scaletest users, promoting a share of them to\n" +
			"template admin so they receive the triggered notifications, and restores them afterwards.\n" +
			"Reuse avoids the account-lifecycle notifications that creating users generates, so it\n" +
			"measures only the notifications under test. Promoted users receive real inbox entries,\n" +
			"and real email where the deployment has SMTP configured, which cleanup does not remove.\n" +
			"\n" +
			"Run only one instance of this command against a deployment at a time. Concurrent runs\n" +
			"select from the same scaletest user pool, so they can promote and then restore each\n" +
			"other's users mid-run, and each run measures notifications the other triggered. The\n" +
			"same applies to running it alongside another scaletest command that creates or deletes\n" +
			"scaletest users, which can remove the users this run is connected as.",
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

			// 0 is rejected rather than clamped: with no template admins nothing watches
			// for the notification, so the run would record zero latencies and exit 0.
			if templateAdminPercentage <= 0 || templateAdminPercentage > 100 {
				return xerrors.Errorf("--template-admin-percentage must be greater than 0 and at most 100")
			}

			// Unlike the shared scaletest concurrency flags, 0 is not "unlimited"
			// here: setup and cleanup are deliberately bounded so they cannot open a
			// connection per user.
			if setupConcurrency <= 0 {
				return xerrors.Errorf("--setup-concurrency must be greater than 0")
			}

			// Reuse promotes real accounts and mints tokens on them. Skipping cleanup
			// would leave both in place with no tooling to find them, and the usual
			// recovery command deletes scaletest users outright, which destroys the pool
			// reuse depends on. Creating users is the mode where leaving residue behind
			// is a coherent request.
			if noCleanup && reuseUsers {
				return xerrors.Errorf("--no-cleanup cannot be used with --reuse-users: it would leave real users holding the template-admin role and a live API token")
			}

			if setupTimeout <= 0 {
				return xerrors.Errorf("--setup-timeout must be greater than 0")
			}

			if cleanupTimeout <= 0 {
				return xerrors.Errorf("--cleanup-timeout must be greater than 0")
			}

			if testTimeout <= 0 {
				return xerrors.Errorf("--timeout must be greater than 0")
			}

			if dialTimeout <= 0 {
				return xerrors.Errorf("--dial-timeout must be greater than 0")
			}

			// The connect phase must finish inside the overall budget, or the last runner
			// releases the dial barrier with no time left to trigger and observe. This
			// rejects only the degenerate case where connecting may consume the entire
			// budget; values close to it still leave little room, so pick a dial timeout
			// well under --timeout.
			if dialTimeout >= testTimeout {
				return xerrors.Errorf("--dial-timeout (%s) must be less than --timeout (%s), leaving time to trigger and deliver notifications", dialTimeout, testTimeout)
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
			// Report the split actually used, which the minimum-one bump above may have
			// moved away from the requested percentage.
			actualAdminPercentage := float64(templateAdminCount) / float64(userCount) * 100
			_, _ = fmt.Fprintf(inv.Stderr, "  Template admins: %d (%.1f%%)\n", templateAdminCount, actualAdminPercentage)
			_, _ = fmt.Fprintf(inv.Stderr, "  Regular users: %d (%.1f%%)\n", regularUserCount, 100.0-actualAdminPercentage)

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
				IdleConnTimeout:     scaletestIdleConnTimeout,
			}
			smtpHTTPClient := &http.Client{
				Transport: smtpHTTPTransport,
			}

			// One HTTP client for every websocket handshake. It carries the CLI's TLS and
			// proxy configuration, which the websocket library would otherwise ignore in
			// favor of http.DefaultClient. Deliberately not the setup client: that one
			// caps MaxConnsPerHost to the setup concurrency, which would throttle the
			// dials this test exists to make. A handshake hands its TCP connection to the
			// caller and never returns it to the pool, so sharing one client still gives
			// every runner its own connection.
			dialClient, err := loadtestutil.DupClientCopyingHeaders(client, BypassHeader)
			if err != nil {
				return xerrors.Errorf("create dial client: %w", err)
			}
			dialHTTPClient := dialClient.HTTPClient

			// Setup and cleanup run at most setupConcurrency requests at a time, so size
			// the pool to match and keep connections warm instead of re-dialing per
			// request.
			//
			// This bounds each client separately. The create path also has createusers
			// duplicate the client per login, so it briefly holds more sockets than this
			// number suggests.
			concurrency := int(setupConcurrency)
			setupClient, err := loadtestutil.DupClientConfiguringTransport(client, BypassHeader, boundPool(concurrency))
			if err != nil {
				return xerrors.Errorf("create setup client: %w", err)
			}
			orgID := me.OrganizationIDs[0]

			// Identify everything this run creates so residue from a hard kill is
			// greppable, and bound the token lifetime to a little beyond the run so
			// orphans expire in hours rather than the deployment default.
			runID, err := cryptorand.String(8)
			if err != nil {
				return xerrors.Errorf("generate run id: %w", err)
			}
			runName := runArtifactName(runID)
			tokenLifetime := testTimeout + cleanupTimeout + time.Hour

			run := &scaletestRun{
				logger:        logger,
				stderr:        inv.Stderr,
				metrics:       metrics,
				client:        client,
				setupClient:   setupClient,
				orgID:         orgID,
				concurrency:   concurrency,
				noCleanup:     noCleanup,
				reuseUsers:    reuseUsers,
				callerID:      me.ID,
				userCount:     int(userCount),
				adminCount:    int(templateAdminCount),
				tokenName:     runName,
				tokenLifetime: tokenLifetime,
				templateName:  runName,
			}

			// Always clean up whatever the run created, even on interrupt, timeout, or a
			// later failure. Registered before setup starts so a partial setup is torn
			// down too, and detached from ctx so the interrupt that ended the run does
			// not also kill the cleanup.
			if !noCleanup {
				defer func() {
					// Tell the operator what is happening and how to leave: killing the
					// process is the one exit that leaves promoted users, live tokens, and a
					// stranded template behind.
					_, _ = fmt.Fprintf(inv.Stderr,
						"\nCleaning up %d users and the trigger template, bounded by --cleanup-timeout=%s.\n"+
							"This can take several minutes at scale and reports progress every %s.\n"+
							"Please do not kill this process: users may be left with the template-admin\n"+
							"role and live API tokens. Interrupt again to abort cleanup deliberately.\n",
						len(run.users), cleanupTimeout, progressInterval)

					cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
					defer cleanupCancel()

					// A second interrupt aborts the cleanup. Without this an operator who
					// believes a slow cleanup is stuck has only SIGKILL, which is worse: it
					// stops cleanup at an arbitrary point with no way to report what remains.
					abort := make(chan os.Signal, 1)
					signal.Notify(abort, StopSignals...)
					defer signal.Stop(abort)
					go func() {
						select {
						case <-abort:
							_, _ = fmt.Fprintf(inv.Stderr,
								"\nAborting cleanup. Residue from this run is named %q:\n"+
									"  API tokens: token name %q on the affected users\n"+
									"  Template:   %q in organization %s\n"+
									"  Users:      may still hold the %s role\n"+
									"Run \"coder exp scaletest cleanup\" to remove scaletest users, or revoke the tokens by name.\n",
								runName, runName, runName, orgID, codersdk.RoleTemplateAdmin)
							cleanupCancel()
						case <-cleanupCtx.Done():
						}
					}()

					if cerr := run.cleanup(cleanupCtx); cerr != nil {
						logger.Error(ctx, "failed to clean up", slog.Error(cerr))
						retErr = errors.Join(retErr, xerrors.Errorf("clean up: %w", cerr))
					}
				}()
			}

			// Setup is all-or-nothing: on any failure, the deferred cleanup tears down
			// whatever it managed to create before the run aborts.
			setupCtx, setupCancel := context.WithTimeout(ctx, setupTimeout)
			err = run.setup(setupCtx)
			setupCancel()
			if err != nil {
				return xerrors.Errorf("set up: %w", err)
			}
			preparedUsers := run.users

			_, _ = fmt.Fprintf(inv.Stderr, "Set up %d users (%d template admins)\n", len(preparedUsers), templateAdminCount)

			dialBarrier := &sync.WaitGroup{}
			templateAdminWatchBarrier := &sync.WaitGroup{}
			dialBarrier.Add(len(preparedUsers))
			templateAdminWatchBarrier.Add(int(templateAdminCount))

			// --timeout is the single budget for the measured phase: connecting every
			// runner, triggering the notifications, and observing them arrive. Both the
			// trigger and every runner derive from it, so there is no per-run timeout
			// nested inside it that could expire first. WithCancelCause lets a trigger
			// failure end the run immediately with its own error, instead of leaving
			// every runner to wait out the budget and report a deadline instead.
			cancelCtx, cancelTest := context.WithCancelCause(ctx)
			defer cancelTest(nil)
			testCtx, testTimeoutCancel := context.WithTimeout(cancelCtx, testTimeout)
			defer testTimeoutCancel()

			triggerDone := make(chan struct{})
			go func() {
				defer close(triggerDone)
				if err := triggerNotifications(
					testCtx,
					logger,
					client,
					orgID,
					runName,
					dialBarrier,
					triggerTimes,
				); err != nil {
					logger.Error(ctx, "failed to trigger notifications", slog.Error(err))
					cancelTest(xerrors.Errorf("trigger notifications: %w", err))
				}
			}()

			// The runners are not harness.Cleanable: this command owns the user
			// lifecycle and tears it down in bulk above, so th.Cleanup is never called
			// and the cleanup strategy passed here is inert.
			th := harness.NewTestHarness(harness.ConcurrentExecutionStrategy{}, harness.LinearExecutionStrategy{})

			for i, pu := range preparedUsers {
				id := strconv.Itoa(i)
				name := fmt.Sprintf("notifications-%s", id)

				config := notifications.Config{
					PreCreatedUser:        pu.user,
					SessionToken:          pu.sessionToken,
					URL:                   client.URL,
					DialHTTPClient:        dialHTTPClient,
					DialTimeout:           dialTimeout,
					DialBarrier:           dialBarrier,
					ReceivingWatchBarrier: templateAdminWatchBarrier,
					Metrics:               metrics,
				}
				if pu.isAdmin {
					config.ExpectedNotificationIDs = expectedNotificationIDs
					config.SMTPApiURL = smtpAPIURL
					config.SMTPRequestTimeout = smtpRequestTimeout
					config.SMTPHttpClient = smtpHTTPClient
				}
				if err := config.Validate(); err != nil {
					return xerrors.Errorf("validate config: %w", err)
				}

				var runner harness.Runnable = notifications.NewRunner(config)
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
			err = th.Run(testCtx)
			if err != nil {
				return xerrors.Errorf("run test harness (harness failure, not a test failure): %w", err)
			}

			// Wait for the trigger goroutine before reading results. A runner only needs
			// the template delete to have happened to receive its notification, so
			// th.Run can return before the trigger records its time. Reading results
			// first would then find no trigger time and report zero latencies.
			<-triggerDone

			// A trigger failure cancels testCtx with its own cause. Report that instead
			// of the N per-runner deadline errors it produces, which point at delivery.
			if cause := context.Cause(testCtx); cause != nil && !errors.Is(cause, context.Canceled) && !errors.Is(cause, context.DeadlineExceeded) {
				return cause
			}

			// If the command was interrupted, skip stats.
			if notifyCtx.Err() != nil {
				return notifyCtx.Err()
			}

			res := th.Results()

			if err := computeNotificationLatencies(ctx, logger, metrics, triggerTimes, res); err != nil {
				return xerrors.Errorf("compute notification latencies: %w", err)
			}

			for _, o := range outputs {
				err = o.write(res, inv.Stdout)
				if err != nil {
					return xerrors.Errorf("write output %q to %q: %w", o.format, o.path, err)
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
			Description:   "Required: Total number of users to run as. Users are created and deleted by default; pass --reuse-users to reuse existing scaletest users instead.",
			Value:         serpent.Int64Of(&userCount),
			Required:      true,
		},
		{
			Flag:        "template-admin-percentage",
			Env:         "CODER_SCALETEST_NOTIFICATION_TEMPLATE_ADMIN_PERCENTAGE",
			Default:     "20.0",
			Description: "Percentage of users to assign the Template Admin role to, which decides how many receive the notification under test. Must be greater than 0 and at most 100.",
			Value:       serpent.Float64Of(&templateAdminPercentage),
		},
		{
			Flag:        "setup-concurrency",
			Env:         "CODER_SCALETEST_NOTIFICATION_SETUP_CONCURRENCY",
			Default:     "10",
			Description: "Number of concurrent workers used to set up users before the test and to clean them up afterwards. Bounds how many connections those phases hold regardless of user count. Must be greater than 0.",
			Value:       serpent.Int64Of(&setupConcurrency),
		},
		{
			Flag:        "timeout",
			Env:         "CODER_SCALETEST_NOTIFICATION_TIMEOUT",
			Default:     "30m",
			Description: "Overall budget for the measured phase: connecting every runner, triggering the notifications, and waiting for them to arrive. Every runner is canceled when it expires. Must be greater than 0; unlike the shared scaletest timeout flags, 0 is not accepted as unlimited, which is why this uses its own environment variable.",
			Value:       serpent.DurationOf(&testTimeout),
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
			Env:         "CODER_SCALETEST_NOTIFICATION_DIAL_TIMEOUT",
			Default:     "10m",
			Description: "Timeout for dialing the notification websocket endpoint. Must be greater than 0 and less than --timeout; like the other timeouts here it does not accept 0 as unlimited, which is why it uses its own environment variable.",
			Value:       serpent.DurationOf(&dialTimeout),
		},
		{
			Flag:        "no-cleanup",
			Env:         "CODER_SCALETEST_NO_CLEANUP",
			Description: "Do not clean up resources after the test completes.",
			Value:       serpent.BoolOf(&noCleanup),
		},
		{
			Flag:        "setup-timeout",
			Env:         "CODER_SCALETEST_NOTIFICATION_SETUP_TIMEOUT",
			Default:     defaultPhaseTimeout.String(),
			Description: "Timeout for the setup phase, covering user creation or selection and clearing stale trigger templates. Defaults to the same value as --cleanup-timeout, which undoes the same work. Must be greater than 0.",
			Value:       serpent.DurationOf(&setupTimeout),
		},
		{
			Flag:        "cleanup-timeout",
			Env:         "CODER_SCALETEST_NOTIFICATION_CLEANUP_TIMEOUT",
			Default:     defaultPhaseTimeout.String(),
			Description: "Timeout for the whole cleanup phase, covering users and the trigger template. Defaults to the same value as --setup-timeout, which creates the same work. Must be greater than 0; unlike the shared scaletest cleanup flags, 0 is not accepted as unlimited, which is why this uses its own environment variable.",
			Value:       serpent.DurationOf(&cleanupTimeout),
		},
		{
			Flag:        "reuse-users",
			Env:         "CODER_SCALETEST_NOTIFICATION_REUSE_USERS",
			Description: "Reuse existing scaletest users instead of creating new ones. Avoids the account-lifecycle notifications that user creation generates, so only the notifications under test are measured. Reused users are promoted to template admin for the run and restored afterwards, but the notifications they receive are not removed.",
			Value:       serpent.BoolOf(&reuseUsers),
		},
		{
			Flag:        "smtp-api-url",
			Env:         "CODER_SCALETEST_SMTP_API_URL",
			Description: "SMTP mock HTTP API address.",
			Value:       serpent.StringOf(&smtpAPIURL),
		},
	}

	tracingFlags.attach(&cmd.Options)
	output.attach(&cmd.Options)
	prometheusFlags.attach(&cmd.Options)
	return cmd
}

func computeNotificationLatencies(
	ctx context.Context,
	logger slog.Logger,
	metrics *notifications.Metrics,
	expectedNotifications map[uuid.UUID]chan time.Time,
	results harness.Results,
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

// triggerNotifications waits for all test users to connect, then creates and
// deletes a template to trigger notification events for testing. The template
// name carries a per-run suffix so a template stranded by a hard kill cannot make
// later runs fail on a name conflict, and so two runs do not collide.
//
// Any error is returned rather than only logged: every runner is parked waiting
// for a notification that can now never arrive, so the caller must fail the run
// instead of letting the whole fleet wait out the budget.
func triggerNotifications(
	ctx context.Context,
	logger slog.Logger,
	client *codersdk.Client,
	orgID uuid.UUID,
	templateName string,
	dialBarrier *sync.WaitGroup,
	expectedNotifications map[uuid.UUID]chan time.Time,
) error {
	triggered := newTriggerRecorder(expectedNotifications)

	logger.Info(ctx, "waiting for all users to connect")

	// Wait for every runner to connect. Bounded by ctx alone: it already carries
	// the overall budget, and --dial-timeout is validated to leave headroom inside
	// it, so a separately derived window would only add a second deadline that can
	// expire while runners are still legitimately dialing.
	done := make(chan struct{})
	go func() {
		dialBarrier.Wait()
		close(done)
	}()

	select {
	case <-done:
		logger.Info(ctx, "all users connected")
	case <-ctx.Done():
		return xerrors.Errorf("wait for users to connect: %w", ctx.Err())
	}

	logger.Info(ctx, "creating test template to test notifications")

	// Upload empty template file.
	file, err := client.Upload(ctx, codersdk.ContentTypeTar, bytes.NewReader([]byte{}))
	if err != nil {
		return xerrors.Errorf("upload test template: %w", err)
	}
	logger.Info(ctx, "test template uploaded", slog.F("file_id", file.ID))

	// Create template version.
	version, err := client.CreateTemplateVersion(ctx, orgID, codersdk.CreateTemplateVersionRequest{
		StorageMethod: codersdk.ProvisionerStorageMethodFile,
		FileID:        file.ID,
		Provisioner:   codersdk.ProvisionerTypeEcho,
	})
	if err != nil {
		return xerrors.Errorf("create test template version: %w", err)
	}
	logger.Info(ctx, "test template version created", slog.F("template_version_id", version.ID))

	// Create template.
	testTemplate, err := client.CreateTemplate(ctx, orgID, codersdk.CreateTemplateRequest{
		Name:        templateName,
		Description: templateName,
		VersionID:   version.ID,
	})
	if err != nil {
		return xerrors.Errorf("create test template: %w", err)
	}
	logger.Info(ctx, "test template created", slog.F("template_id", testTemplate.ID))

	// Delete template to trigger notification.
	if err := client.DeleteTemplate(ctx, testTemplate.ID); err != nil {
		return xerrors.Errorf("delete test template: %w", err)
	}
	logger.Info(ctx, "test template deleted", slog.F("template_id", testTemplate.ID))

	// Deleting the template is the action that produces TemplateTemplateDeleted, so
	// record the trigger time against that notification specifically rather than
	// against every notification the runners happen to expect.
	if err := triggered.record(notificationsLib.TemplateTemplateDeleted); err != nil {
		return err
	}

	// Every expected notification must have been produced by one of the actions
	// above. Without this check, adding an ID to the expected set without adding the
	// action that causes it leaves every runner waiting for a notification that can
	// never arrive, until the budget expires.
	return triggered.verifyAll()
}

// triggerRecorder records when the action producing each expected notification
// completed, and reports an expected notification that no action produced.
type triggerRecorder struct {
	expected map[uuid.UUID]chan time.Time
	done     map[uuid.UUID]struct{}
}

func newTriggerRecorder(expected map[uuid.UUID]chan time.Time) *triggerRecorder {
	return &triggerRecorder{
		expected: expected,
		done:     make(map[uuid.UUID]struct{}, len(expected)),
	}
}

// record stores the current time as the trigger time for notificationID. The
// channel is buffered and closed after the send, so the read in
// computeNotificationLatencies finds the value whenever it runs.
func (t *triggerRecorder) record(notificationID uuid.UUID) error {
	ch, ok := t.expected[notificationID]
	if !ok {
		return xerrors.Errorf("triggered notification %q that no runner is waiting for", notificationID)
	}
	if _, ok := t.done[notificationID]; ok {
		return xerrors.Errorf("notification %q triggered more than once", notificationID)
	}
	ch <- time.Now()
	close(ch)
	t.done[notificationID] = struct{}{}
	return nil
}

// verifyAll reports any expected notification that no action triggered.
func (t *triggerRecorder) verifyAll() error {
	var errs error
	for id := range t.expected {
		if _, ok := t.done[id]; !ok {
			errs = errors.Join(errs, xerrors.Errorf(
				"no action triggers expected notification %q, so no runner can receive it", id))
		}
	}
	return errs
}

// scaletestRun owns every resource this run creates or changes on the deployment:
// the users the runners connect as, and the template whose deletion triggers the
// notification under test.
//
// Setup and cleanup are each a single entry point covering both resources, so they
// cannot drift apart in timeout, concurrency, progress reporting, or interrupt
// handling. Cleaning the two up separately previously gave each its own full
// --cleanup-timeout, so the worst case was twice the budget the operator asked for.
type scaletestRun struct {
	logger  slog.Logger
	stderr  io.Writer
	metrics *notifications.Metrics

	// client performs template operations as the calling admin. setupClient has a
	// bounded connection pool for the per-user work.
	client      *codersdk.Client
	setupClient *codersdk.Client

	orgID       uuid.UUID
	concurrency int
	noCleanup   bool

	reuseUsers    bool
	callerID      uuid.UUID
	userCount     int
	adminCount    int
	tokenName     string
	tokenLifetime time.Duration
	templateName  string

	// users is filled by setup and read by cleanup, so a failure part-way through
	// setup still tears down what it managed to create.
	users []preparedUser
}

// setup clears any stale trigger template and makes the users the runners connect
// as, either creating them or reusing existing scaletest users. It is bounded by
// the caller's context and runs at most concurrency requests at a time.
func (r *scaletestRun) setup(ctx context.Context) error {
	// A template stranded by an earlier killed run would accumulate. Per-run names
	// keep one from breaking this run, so this only stops the pile-up and failing to
	// sweep is not fatal. Skipped under --no-cleanup, which promises to delete
	// nothing.
	if !r.noCleanup {
		if err := r.sweepStaleTemplates(ctx); err != nil {
			r.logger.Warn(ctx, "failed to sweep stale trigger templates", slog.Error(err))
		}
	}

	if r.reuseUsers {
		return r.prepareExistingUsers(ctx)
	}
	return r.createUsers(ctx)
}

// cleanup undoes everything setup and the trigger created, best-effort, and
// returns the joined errors. Users come first: an elevated role and a live token
// on an account matter more than a leftover template.
func (r *scaletestRun) cleanup(ctx context.Context) error {
	errs := r.cleanupUsers(ctx)

	// Delete by name rather than by an ID recorded when the create returned. A
	// create can commit on the server while the client sees an error, and the name
	// is chosen before the request is made, so the lookup finds the template either
	// way. It is also already gone on the happy path, where the trigger deleted it
	// to fire the notification.
	if err := r.deleteTemplateByName(ctx, r.templateName); err != nil {
		errs = errors.Join(errs, xerrors.Errorf("clean up trigger template: %w", err))
	}
	return errs
}

func (r *scaletestRun) createUsers(ctx context.Context) error {
	r.users = make([]preparedUser, r.userCount)
	for i := range r.users {
		r.users[i] = preparedUser{
			origin:  originCreated,
			isAdmin: i < r.adminCount,
			id:      strconv.Itoa(i),
		}
	}

	// A separate client because RunReturningUser turns on body logging for whatever
	// client it is handed, which would follow the shared setup client into cleanup.
	createClient, err := loadtestutil.DupClientConfiguringTransport(r.client, BypassHeader, boundPool(r.concurrency))
	if err != nil {
		return xerrors.Errorf("create user-creation client: %w", err)
	}

	_, _ = fmt.Fprintf(r.stderr, "Creating %d users (%d template admins) with %d concurrent workers...\n",
		r.userCount, r.adminCount, r.concurrency)
	return forEachUser(ctx, newProgress(r.stderr, "Created"), r.users, r.concurrency, func(ctx context.Context, pu *preparedUser) error {
		if err := createAndLoginUser(ctx, createClient, r.orgID, pu); err != nil {
			r.metrics.AddError("create_user")
			return xerrors.Errorf("create user %q: %w", pu.id, err)
		}
		return nil
	})
}

func (r *scaletestRun) prepareExistingUsers(ctx context.Context) error {
	_, _ = fmt.Fprintf(r.stderr, "Selecting %d existing users (%d template admins)...\n", r.userCount, r.adminCount)
	selected, err := selectExistingUsers(ctx, r.setupClient, r.callerID, r.userCount)
	if err != nil {
		return xerrors.Errorf("select existing users: %w", err)
	}

	r.users = make([]preparedUser, len(selected))
	for i, u := range selected {
		r.users[i] = preparedUser{
			origin:  originReused,
			user:    u,
			isAdmin: i < r.adminCount,
		}
	}

	_, _ = fmt.Fprintf(r.stderr, "Preparing %d users with %d concurrent workers...\n", len(r.users), r.concurrency)
	return forEachUser(ctx, newProgress(r.stderr, "Prepared"), r.users, r.concurrency, func(ctx context.Context, pu *preparedUser) error {
		if err := prepareUser(ctx, r.setupClient, r.tokenName, r.tokenLifetime, pu); err != nil {
			r.metrics.AddError("prepare_user")
			return xerrors.Errorf("prepare user %q: %w", pu.user.ID, err)
		}
		return nil
	})
}

// cleanupUsers restores reused users or deletes created ones, whichever this run
// set up.
func (r *scaletestRun) cleanupUsers(ctx context.Context) error {
	if len(r.users) == 0 {
		return nil
	}
	if r.reuseUsers {
		return forEachUserBestEffort(ctx, newProgress(r.stderr, "Restored"), r.users, r.concurrency, func(ctx context.Context, pu *preparedUser) error {
			return restoreUser(ctx, r.metrics, r.setupClient, pu)
		})
	}
	return forEachUserBestEffort(ctx, newProgress(r.stderr, "Deleted"), r.users, r.concurrency, func(ctx context.Context, pu *preparedUser) error {
		return deleteUser(ctx, r.metrics, r.setupClient, pu)
	})
}

// deleteTemplateByName deletes the named template if it still exists.
func (r *scaletestRun) deleteTemplateByName(ctx context.Context, name string) error {
	tpl, err := r.client.TemplateByName(ctx, r.orgID, name)
	if err != nil {
		if sdkErr, ok := errors.AsType[*codersdk.Error](err); ok && sdkErr.StatusCode() == http.StatusNotFound {
			// Never created, or already deleted by the trigger.
			return nil
		}
		return xerrors.Errorf("look up template %q: %w", name, err)
	}
	if err := r.client.DeleteTemplate(ctx, tpl.ID); err != nil {
		return xerrors.Errorf("delete template %q: %w", name, err)
	}
	return nil
}

// sweepStaleTemplates deletes trigger templates left behind by earlier runs that
// were killed between creating and deleting one. Per-run names keep a stranded
// template from breaking this run, so this only stops them accumulating.
//
// Every trigger template except this run's own is fair game. A concurrent run's
// in-flight template is indistinguishable from a stranded one, which is one of the
// reasons only one instance of this command may run against a deployment at a
// time.
func (r *scaletestRun) sweepStaleTemplates(ctx context.Context) error {
	// Filter server-side rather than listing every template in the deployment. The
	// filter is a substring match, so the prefix check below still decides.
	templates, err := r.client.Templates(ctx, codersdk.TemplateFilter{
		OrganizationID: r.orgID,
		FuzzyName:      notificationsPrefix,
	})
	if err != nil {
		return xerrors.Errorf("list templates: %w", err)
	}
	var errs error
	for _, tpl := range templates {
		if tpl.Name == r.templateName || !strings.HasPrefix(tpl.Name, notificationsPrefix) {
			continue
		}
		r.logger.Info(ctx, "deleting stale trigger template left by an earlier run",
			slog.F("template_id", tpl.ID), slog.F("template_name", tpl.Name))
		if err := r.client.DeleteTemplate(ctx, tpl.ID); err != nil {
			errs = errors.Join(errs, xerrors.Errorf("delete stale template %q: %w", tpl.Name, err))
		}
	}
	return errs
}

// boundPool returns a transport configuration that caps the connection pool at
// limit and keeps those connections warm, so a phase making limit concurrent
// requests reuses connections instead of re-dialing per request.
func boundPool(limit int) func(*http.Transport) {
	return func(t *http.Transport) {
		t.MaxIdleConns = limit
		t.MaxIdleConnsPerHost = limit
		t.MaxConnsPerHost = limit
		t.IdleConnTimeout = scaletestIdleConnTimeout
	}
}

// progress periodically reports how far a long phase has got. Setup and cleanup
// can each run for minutes at scale, and an operator who reads silence as a hang
// may kill the process, which is the one exit the deferred cleanup cannot cover.
//
// The zero value reports nothing, which is what tests want.
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

// forEachUser runs fn once per user, at most limit at a time, reporting progress
// through p. The first error cancels the shared context so in-flight and pending
// calls stop early, and that error is returned. limit must be positive, which the
// CLI validates.
func forEachUser(ctx context.Context, p progress, users []preparedUser, limit int, fn func(context.Context, *preparedUser) error) error {
	var completed atomic.Int64
	stopProgress := p.watch(ctx, len(users), &completed)
	defer stopProgress()

	eg, egCtx := errgroup.WithContext(ctx)
	eg.SetLimit(limit)
	for i := range users {
		pu := &users[i]
		eg.Go(func() error {
			if err := egCtx.Err(); err != nil {
				return err
			}
			if err := fn(egCtx, pu); err != nil {
				return err
			}
			completed.Add(1)
			return nil
		})
	}
	return eg.Wait()
}

// forEachUserBestEffort runs fn once per user, at most limit at a time, reporting
// progress through p. Every call runs to completion even when one fails, and all
// returned errors are joined. Cleanup uses this so a single failure cannot abandon
// the remaining users.
func forEachUserBestEffort(ctx context.Context, p progress, users []preparedUser, limit int, fn func(context.Context, *preparedUser) error) error {
	var completed atomic.Int64
	stopProgress := p.watch(ctx, len(users), &completed)
	defer stopProgress()

	var (
		mu   sync.Mutex
		errs error
	)
	var eg errgroup.Group
	eg.SetLimit(limit)
	for i := range users {
		pu := &users[i]
		eg.Go(func() error {
			if err := fn(ctx, pu); err != nil {
				mu.Lock()
				errs = errors.Join(errs, err)
				mu.Unlock()
				return nil
			}
			completed.Add(1)
			return nil
		})
	}
	_ = eg.Wait()
	return errs
}

// preparedUser is a user made ready to connect for the run, authenticated with
// a session token, plus the metadata each setup path needs to clean it up.
//
// It is produced by one of two mutually exclusive paths:
//   - create: a new user is created and logged in; cleanup deletes it by ID.
//   - reuse: an existing scaletest user is promoted (if admin) and issued a
//     minted token; cleanup revokes the token and removes only that role.
//
// userOrigin records where a prepared user came from, which decides what cleanup
// is allowed to do to it. Deleting a reused account or demoting a created one are
// both wrong, and the sinks enforce that rather than trusting their callers.
type userOrigin int

const (
	// originCreated means this run created the user and must delete it.
	originCreated userOrigin = iota
	// originReused means the user predates this run and must survive it.
	originReused
)

type preparedUser struct {
	origin       userOrigin
	user         codersdk.User
	sessionToken string
	// isAdmin marks a user designated as a template admin for the run.
	isAdmin bool

	// Reuse path only.
	tokenID string

	// Create path only. id identifies the user within the run; the create runner
	// generates the username and email from it.
	id string
}

// selectExistingUsers returns userCount existing scaletest users, failing fast
// unless at least that many are present.
//
// Only users whose username carries the scaletest prefix are eligible, as created
// by this command or by any other scaletest command that leaves its users behind.
// That keeps the run from promoting real accounts, and it means the residue of a
// hard kill stays reclaimable with "coder exp scaletest cleanup", which enumerates
// a superset of them.
//
// Also excluded: excludeID (the caller running the test), service accounts, and
// users that already hold the owner or template-admin role, so promoting a subset
// adds exactly the intended new recipients and restores cleanly. Note that
// deleting a template notifies every template admin and owner in the deployment,
// so pre-existing admins receive the notification too and are not measured.
//
// The query filters to active users because a suspended or dormant user cannot
// receive notifications, and to password login because scaletest users are
// created with passwords.
//
// The users endpoint returns results ordered by username, so the returned slice
// is stable across runs without any additional sorting.
func selectExistingUsers(ctx context.Context, client *codersdk.Client, excludeID uuid.UUID, userCount int) ([]codersdk.User, error) {
	// Page rather than asking for every user at once: on the large deployments this
	// command targets that is a single very large query, and the loop can stop as
	// soon as enough eligible users are found.
	const pageSize = 1000
	pool := make([]codersdk.User, 0, userCount)
	for offset := 0; len(pool) < userCount; offset += pageSize {
		resp, err := client.Users(ctx, codersdk.UsersRequest{
			Status:    codersdk.UserStatusActive,
			LoginType: []codersdk.LoginType{codersdk.LoginTypePassword},
			// Narrow server-side so a deployment with many users does not page all of
			// them back. This is a substring match across username, email, and name, so
			// the exact prefix check below still decides eligibility.
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
			// Require the username prefix rather than loadtestutil.IsScaleTestUser,
			// which also matches on the scaletest email domain. This is deliberately the
			// narrower test: it is a subset of what "coder exp scaletest cleanup"
			// reclaims, so anything promoted here stays reclaimable, while an account
			// that merely has a scaletest-looking address is never touched.
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

// prepareUser promotes the user to template admin (when designated) and mints a
// token authenticating as that user, filling in the mutable fields of pu.
func prepareUser(ctx context.Context, client *codersdk.Client, tokenName string, tokenLifetime time.Duration, pu *preparedUser) error {
	if pu.isAdmin {
		// Add template-admin to the roles the user already holds. Cleanup removes
		// only this role, so roles granted by anyone else during the run survive.
		current, err := currentRoleNames(ctx, client, pu.user.ID)
		if err != nil {
			return err
		}
		// Selection excludes users that already hold the role, so finding it here
		// means it was granted between selection and now. Skip the redundant write;
		// cleanup removes the role either way, which is the state selection expected.
		if !slices.Contains(current, codersdk.RoleTemplateAdmin) {
			if _, err := client.UpdateUserRoles(ctx, pu.user.ID.String(), codersdk.UpdateRoles{
				Roles: append(current, codersdk.RoleTemplateAdmin),
			}); err != nil {
				return xerrors.Errorf("promote to template admin: %w", err)
			}
		}
	}

	// Name the token after this run and give it a bounded lifetime. Without both, a
	// token orphaned by a hard kill is indistinguishable from the user's own and
	// lives for the deployment default, 7 days out of the box.
	//
	// Requesting a lifetime at all subjects the request to the deployment's
	// max-token-lifetime, which a request with no lifetime skips entirely. The cap
	// for non-owners defaults to 100 years, so this normally passes, but a
	// deployment that lowers it below the run budget will reject the request. That
	// is worth reporting rather than silently falling back to the 7-day default.
	// The token is left at the default scope. The runner only reads one inbox
	// endpoint, so inbox_notification:read would be the right scope, but that is not
	// in coderd's external scope catalog (rbac.IsExternalScope) and the tokens API
	// rejects it. Narrowing this needs that catalog to change first.
	tokenRes, err := client.CreateToken(ctx, pu.user.ID.String(), codersdk.CreateTokenRequest{
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
	pu.sessionToken = tokenRes.Key
	pu.tokenID = tokenID

	return nil
}

// restoreUser revokes the minted token and removes the template-admin role this
// test added, returning the joined errors of any failures. It never deletes users.
//
// Whether to remove the role is decided by reading the user's roles at cleanup
// time, not by recording what this run believes it did. A promotion can commit on
// the server and still return an error to the client, from a reset connection or
// a deadline that fires after the write, so a client-side record misses exactly
// the case that leaves an elevated role on an account.
//
// Only users this run designated as admins are considered, so a role granted to a
// user it never intended to promote is left alone. Among those, selection excludes
// users that already hold template-admin, so one carrying it at cleanup acquired it
// during this run. A grant made by someone else inside that window is
// indistinguishable from ours and is removed too, which is acceptable because reuse
// only ever selects disposable scaletest accounts.
func restoreUser(ctx context.Context, metrics *notifications.Metrics, client *codersdk.Client, u *preparedUser) error {
	// The mirror of the guard in deleteUser: a created user is deleted, never
	// restored, so restoring one means the caller mixed up the two paths.
	if u.origin != originReused {
		return xerrors.Errorf("refusing to restore user %q that this run created", u.user.ID)
	}
	if u.user.ID == uuid.Nil {
		return nil
	}
	var errs error
	if u.tokenID != "" {
		if err := client.DeleteAPIKey(ctx, u.user.ID.String(), u.tokenID); err != nil {
			metrics.AddError("revoke_token")
			errs = errors.Join(errs, xerrors.Errorf("revoke token for user %q: %w", u.user.ID, err))
		}
	}
	if !u.isAdmin {
		return errs
	}
	current, err := currentRoleNames(ctx, client, u.user.ID)
	if err != nil {
		metrics.AddError("demote_user")
		return errors.Join(errs, err)
	}
	// Nothing to undo, so no write and no audit entry for a user setup never
	// reached.
	if !slices.Contains(current, codersdk.RoleTemplateAdmin) {
		return errs
	}
	remaining := slices.DeleteFunc(current, func(role string) bool {
		return role == codersdk.RoleTemplateAdmin
	})
	if _, err := client.UpdateUserRoles(ctx, u.user.ID.String(), codersdk.UpdateRoles{Roles: remaining}); err != nil {
		metrics.AddError("demote_user")
		errs = errors.Join(errs, xerrors.Errorf("demote user %q: %w", u.user.ID, err))
	}
	return errs
}

// createAndLoginUser creates the user in orgID and logs in to obtain a session
// token, then promotes it to template admin when designated, filling in the
// mutable fields of pu. pu.user is set as soon as the user exists so a later
// failure still lets cleanup delete it.
//
// client must not be the shared setup client: RunReturningUser calls SetLogger and
// SetLogBodies on whatever it is given, which would make the setup client copy
// every request and response body for the rest of setup and all of cleanup.
func createAndLoginUser(ctx context.Context, client *codersdk.Client, orgID uuid.UUID, pu *preparedUser) error {
	// Reuse createusers.Runner so the create+login sequence lives in one place. It
	// generates the username and email from the id when Config leaves them empty.
	runner := createusers.NewRunner(client, createusers.Config{OrganizationID: orgID})
	created, err := runner.RunReturningUser(ctx, pu.id, io.Discard)
	// Capture the user even on failure: if creation succeeded but a later step
	// failed, cleanup still needs the ID to delete it.
	pu.user = runner.User()
	if err != nil {
		// The create may have completed server-side while the client gave up, which
		// leaves no ID for cleanup to delete. Look the user up by the name the runner
		// generated so it is not orphaned. Best-effort on a context that is likely
		// already done, so failure here only loses what was already lost.
		if pu.user.ID == uuid.Nil {
			if found, ferr := findUserByScaletestID(ctx, client, pu.id); ferr == nil {
				pu.user = found
			}
		}
		return xerrors.Errorf("create and login user: %w", err)
	}
	pu.sessionToken = created.SessionToken

	// The create runner does not assign roles.
	if pu.isAdmin {
		if _, err := client.UpdateUserRoles(ctx, pu.user.ID.String(), codersdk.UpdateRoles{Roles: []string{codersdk.RoleTemplateAdmin}}); err != nil {
			return xerrors.Errorf("assign template admin role: %w", err)
		}
	}

	return nil
}

// findUserByScaletestID looks for a user this run created, identified by the id
// suffix the create runner puts in the generated username. Used to recover the ID
// of a user whose creation completed after the client stopped waiting.
//
// The suffix is not unique across runs, so this can match a user left behind by an
// earlier run with the same index. That is acceptable only because a single
// operator runs this command against a deployment at a time, and because both
// candidates are disposable scaletest users that "coder exp scaletest cleanup"
// reclaims either way.
func findUserByScaletestID(ctx context.Context, client *codersdk.Client, id string) (codersdk.User, error) {
	resp, err := client.Users(ctx, codersdk.UsersRequest{
		SearchQuery: loadtestutil.ScaleTestPrefix,
		Pagination:  codersdk.Pagination{Limit: 1000},
	})
	if err != nil {
		return codersdk.User{}, xerrors.Errorf("list users: %w", err)
	}
	suffix := "-" + id
	for _, u := range resp.Users {
		if strings.HasPrefix(u.Username, loadtestutil.ScaleTestPrefix+"-") && strings.HasSuffix(u.Username, suffix) {
			return u, nil
		}
	}
	return codersdk.User{}, xerrors.Errorf("no user found for scaletest id %q", id)
}

// deleteUser deletes the user by ID, best-effort. Callers must pass only users
// this test created; it must never see reuse-path users, whose accounts belong to
// the deployment.
func deleteUser(ctx context.Context, metrics *notifications.Metrics, client *codersdk.Client, u *preparedUser) error {
	// Refuse here rather than trusting the call site. Deleting a reused account is
	// unrecoverable, so the check belongs where the damage would be done.
	if u.origin != originCreated {
		return xerrors.Errorf("refusing to delete user %q that this run did not create", u.user.ID)
	}
	if u.user.ID == uuid.Nil {
		return nil
	}
	if err := client.DeleteUser(ctx, u.user.ID); err != nil {
		metrics.AddError("delete_user")
		return xerrors.Errorf("delete user %q: %w", u.user.ID, err)
	}
	return nil
}

func userHasRole(user codersdk.User, role string) bool {
	return slices.ContainsFunc(user.Roles, func(r codersdk.SlimRole) bool { return r.Name == role })
}

func userRoleNames(user codersdk.User) []string {
	names := make([]string, 0, len(user.Roles))
	for _, r := range user.Roles {
		names = append(names, r.Name)
	}
	return names
}

// currentRoleNames fetches the user's site role names as they are right now,
// rather than relying on a value captured earlier in the run.
func currentRoleNames(ctx context.Context, client *codersdk.Client, userID uuid.UUID) ([]string, error) {
	user, err := client.User(ctx, userID.String())
	if err != nil {
		return nil, xerrors.Errorf("fetch roles for user %q: %w", userID, err)
	}
	return userRoleNames(user), nil
}
