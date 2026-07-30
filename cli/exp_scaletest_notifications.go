//go:build !slim

package cli

import (
	"bytes"
	"context"
	"errors"
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
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	notificationsLib "github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/cryptorand"
	"github.com/coder/coder/v2/scaletest/harness"
	"github.com/coder/coder/v2/scaletest/loadtestutil"
	"github.com/coder/coder/v2/scaletest/notifications"
	"github.com/coder/serpent"
)

// setupShards bounds the concurrency of the user setup and cleanup phases so
// they hold only ~setupShards connections regardless of user count.
const setupShards = 10

func (r *RootCmd) scaletestNotifications() *serpent.Command {
	var (
		userCount               int64
		templateAdminPercentage float64
		notificationTimeout     time.Duration
		smtpRequestTimeout      time.Duration
		dialTimeout             time.Duration
		noCleanup               bool
		createUsers             bool
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

			setupClient, err := loadtestutil.DupClientCopyingHeaders(client, BypassHeader)
			if err != nil {
				return xerrors.Errorf("create setup client: %w", err)
			}
			// The setup phase runs setupShards goroutines, each making sequential
			// requests, so at most setupShards connections are ever in flight. Size the
			// pool to match so connections are kept warm and reused instead of
			// re-dialed per request.
			if ht, ok := setupClient.HTTPClient.Transport.(*codersdk.HeaderTransport); ok {
				if t, ok := ht.Transport.(*http.Transport); ok {
					t.MaxIdleConns = setupShards
					t.MaxIdleConnsPerHost = setupShards
					t.MaxConnsPerHost = setupShards
					t.IdleConnTimeout = 60 * time.Second
				}
			}
			orgID := me.OrganizationIDs[0]

			// Set up the users the runners will connect as. Either reuse existing
			// deployment users (promote + mint tokens, restored afterwards) or create
			// dedicated users (deleted afterwards). cleanupUsers tears down whatever this
			// path set up and is used for both the abort and normal cleanup paths.
			var preparedUsers []preparedUser
			var cleanupUsers func(context.Context) error
			if createUsers {
				for i := 0; i < int(userCount); i++ {
					username, email, err := loadtestutil.GenerateUserIdentifier(strconv.Itoa(i))
					if err != nil {
						return xerrors.Errorf("generate user identifier: %w", err)
					}
					preparedUsers = append(preparedUsers, preparedUser{
						IsAdmin:  i < int(templateAdminCount),
						username: username,
						email:    email,
					})
				}
				cleanupUsers = func(ctx context.Context) error {
					return bestEffortSharded(ctx, preparedUsers, setupShards, func(ctx context.Context, shard []preparedUser) error {
						return deleteUsers(ctx, metrics, setupClient, shard)
					})
				}
				_, _ = fmt.Fprintf(inv.Stderr, "Creating %d users (%d template admins) across %d shards...\n", userCount, templateAdminCount, setupShards)
				err = runSharded(ctx, preparedUsers, setupShards, func(ctx context.Context, shard []preparedUser) error {
					return createAndLoginUsers(ctx, metrics, setupClient, orgID, shard)
				})
			} else {
				_, _ = fmt.Fprintf(inv.Stderr, "Selecting %d existing users (%d template admins)...\n", userCount, templateAdminCount)
				selectedUsers, serr := selectExistingUsers(ctx, setupClient, int(userCount))
				if serr != nil {
					return xerrors.Errorf("select existing users: %w", serr)
				}
				preparedUsers = make([]preparedUser, len(selectedUsers))
				for i, u := range selectedUsers {
					preparedUsers[i] = preparedUser{
						User:          u,
						IsAdmin:       i < int(templateAdminCount),
						originalRoles: userRoleNames(u),
					}
				}
				cleanupUsers = func(ctx context.Context) error {
					return bestEffortSharded(ctx, preparedUsers, setupShards, func(ctx context.Context, shard []preparedUser) error {
						return restoreUsers(ctx, metrics, setupClient, shard)
					})
				}
				_, _ = fmt.Fprintf(inv.Stderr, "Preparing %d users across %d shards...\n", len(preparedUsers), setupShards)
				err = runSharded(ctx, preparedUsers, setupShards, func(ctx context.Context, shard []preparedUser) error {
					return prepareUsers(ctx, metrics, setupClient, shard)
				})
			}

			// Setup is all-or-nothing: on any failure, tear down whatever we already
			// created or promoted, then abort the run.
			if err != nil {
				if !noCleanup {
					_, _ = fmt.Fprintln(inv.Stderr, "\nCleaning up users after setup failure...")
					cleanupCtx, cleanupCancel := cleanupStrategy.toContext(ctx)
					if cerr := cleanupUsers(cleanupCtx); cerr != nil {
						logger.Error(ctx, "failed to clean up users after setup failure", slog.Error(cerr))
					}
					cleanupCancel()
				}
				return xerrors.Errorf("set up users: %w", err)
			}

			_, _ = fmt.Fprintf(inv.Stderr, "Set up %d users (%d template admins)\n", len(preparedUsers), templateAdminCount)

			dialBarrier := &sync.WaitGroup{}
			templateAdminWatchBarrier := &sync.WaitGroup{}
			dialBarrier.Add(len(preparedUsers))
			templateAdminWatchBarrier.Add(int(templateAdminCount))

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

			for i, pu := range preparedUsers {
				id := strconv.Itoa(i)
				name := fmt.Sprintf("notifications-%s", id)

				config := notifications.Config{
					PreCreatedUser:        pu.User,
					SessionToken:          pu.SessionToken,
					NotificationTimeout:   notificationTimeout,
					DialTimeout:           dialTimeout,
					DialBarrier:           dialBarrier,
					ReceivingWatchBarrier: templateAdminWatchBarrier,
					Metrics:               metrics,
				}
				if pu.IsAdmin {
					config.ExpectedNotificationsIDs = expectedNotificationIDs
					config.SMTPApiURL = smtpAPIURL
					config.SMTPRequestTimeout = smtpRequestTimeout
					config.SMTPHttpClient = smtpHTTPClient
				}
				if err := config.Validate(); err != nil {
					return xerrors.Errorf("validate config: %w", err)
				}

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

			if err := computeNotificationLatencies(ctx, logger, metrics, triggerTimes, res); err != nil {
				return xerrors.Errorf("compute notification latencies: %w", err)
			}

			for _, o := range outputs {
				err = o.write(res, inv.Stdout)
				if err != nil {
					return xerrors.Errorf("write output %q to %q: %w", o.format, o.path, err)
				}
			}

			if !noCleanup {
				_, _ = fmt.Fprintln(inv.Stderr, "\nCleaning up users...")
				cleanupCtx, cleanupCancel := cleanupStrategy.toContext(ctx)
				defer cleanupCancel()
				if cerr := cleanupUsers(cleanupCtx); cerr != nil {
					logger.Error(ctx, "failed to clean up users", slog.Error(cerr))
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
			Flag:        "no-cleanup",
			Env:         "CODER_SCALETEST_NO_CLEANUP",
			Description: "Do not clean up resources after the test completes.",
			Value:       serpent.BoolOf(&noCleanup),
		},
		{
			Flag:        "create-users",
			Env:         "CODER_SCALETEST_NOTIFICATION_CREATE_USERS",
			Description: "Create new users for the test instead of reusing existing ones. Created users are deleted on cleanup.",
			Value:       serpent.BoolOf(&createUsers),
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

// runSharded splits users into shards contiguous chunks and runs fn on each
// chunk concurrently. Chunks are sub-slices of users, so fn may mutate its
// elements in place. If any fn returns an error, the shared context is canceled
// so sibling chunks can stop early, and the first error is returned.
func runSharded(ctx context.Context, users []preparedUser, shards int, fn func(context.Context, []preparedUser) error) error {
	if shards < 1 {
		shards = 1
	}
	chunk := (len(users) + shards - 1) / shards
	if chunk < 1 {
		return nil
	}
	eg, egCtx := errgroup.WithContext(ctx)
	for start := 0; start < len(users); start += chunk {
		end := min(start+chunk, len(users))
		shard := users[start:end]
		eg.Go(func() error {
			return fn(egCtx, shard)
		})
	}
	return eg.Wait()
}

// bestEffortSharded shards users like runSharded but runs every chunk to
// completion, never canceling siblings when one fails, and joins all returned
// errors. Cleanup uses this so a single failure does not abandon the remaining
// users.
func bestEffortSharded(ctx context.Context, users []preparedUser, shards int, fn func(context.Context, []preparedUser) error) error {
	var (
		mu   sync.Mutex
		errs error
	)
	_ = runSharded(ctx, users, shards, func(ctx context.Context, shard []preparedUser) error {
		if err := fn(ctx, shard); err != nil {
			mu.Lock()
			errs = errors.Join(errs, err)
			mu.Unlock()
		}
		return nil
	})
	return errs
}

// preparedUser is a user made ready to connect for the run, authenticated with
// a session token, plus the metadata each setup path needs to clean it up.
//
// It is produced by one of two mutually exclusive paths:
//   - reuse: an existing user is promoted (if admin) and issued a minted token;
//     cleanup revokes the token and demotes via originalRoles.
//   - create: a brand new user is created (as admin if designated) and logged
//     in; cleanup deletes the user by ID. username/email are pre-generated so
//     creation can run in the sharded fill.
type preparedUser struct {
	User         codersdk.User
	SessionToken string
	// IsAdmin marks a user designated as a template admin for the run. In the
	// reuse path, since we never select users that are already admins, IsAdmin
	// also means we promoted the user and must demote them during cleanup.
	IsAdmin bool

	// Reuse path only.
	tokenID       string
	originalRoles []string

	// Create path only.
	username string
	email    string
}

// selectExistingUsers returns userCount eligible existing users, failing fast
// unless at least that many are present. Eligible users exclude the caller,
// service accounts, and users that already hold the owner or template-admin
// role, so promoting a subset yields exactly the intended notification audience
// and restores cleanly.
//
// The users endpoint returns results ordered by username, so the returned slice
// is stable across runs without any additional sorting.
func selectExistingUsers(ctx context.Context, client *codersdk.Client, userCount int) ([]codersdk.User, error) {
	me, err := client.User(ctx, codersdk.Me)
	if err != nil {
		return nil, xerrors.Errorf("fetch current user: %w", err)
	}

	// Limit 0 returns all matching users in a single page.
	resp, err := client.Users(ctx, codersdk.UsersRequest{
		Status:     codersdk.UserStatusActive,
		LoginType:  []codersdk.LoginType{codersdk.LoginTypePassword},
		Pagination: codersdk.Pagination{Limit: 0},
	})
	if err != nil {
		return nil, xerrors.Errorf("list users: %w", err)
	}

	pool := make([]codersdk.User, 0, len(resp.Users))
	for _, u := range resp.Users {
		if u.ID == me.ID || u.IsServiceAccount {
			continue
		}
		if userHasRole(u, codersdk.RoleOwner) || userHasRole(u, codersdk.RoleTemplateAdmin) {
			continue
		}
		pool = append(pool, u)
	}

	if len(pool) < userCount {
		return nil, xerrors.Errorf("notifications scaletest requires %d eligible existing users but only %d are present", userCount, len(pool))
	}

	return pool[:userCount], nil
}

// prepareUsers promotes and mints a token for each user in the given slice,
// sequentially, stopping and returning on the first failure. Preparation is
// all-or-nothing (matching the original create+login runner, where any failure
// failed the whole load test), so the caller aborts the run and restores any
// users already touched. It is intended to be called concurrently over disjoint
// slices via runSharded, so it also stops early once ctx is canceled by a
// sibling failure.
func prepareUsers(ctx context.Context, metrics *notifications.Metrics, client *codersdk.Client, users []preparedUser) error {
	for i := range users {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := prepareUser(ctx, client, &users[i]); err != nil {
			metrics.AddError("prepare_user")
			return xerrors.Errorf("prepare user %q: %w", users[i].User.ID, err)
		}
	}
	return nil
}

// prepareUser promotes the user to template admin (when designated) and mints a
// token authenticating as that user, filling in the mutable fields of pu.
func prepareUser(ctx context.Context, client *codersdk.Client, pu *preparedUser) error {
	if pu.IsAdmin {
		promotedRoles := append(append([]string{}, pu.originalRoles...), codersdk.RoleTemplateAdmin)
		if _, err := client.UpdateUserRoles(ctx, pu.User.ID.String(), codersdk.UpdateRoles{Roles: promotedRoles}); err != nil {
			return xerrors.Errorf("promote to template admin: %w", err)
		}
	}

	// A token with no explicit lifetime uses the server default, which outlives
	// the test. Owners are excluded from the pool, so the max-token-lifetime cap
	// does not apply.
	tokenRes, err := client.CreateToken(ctx, pu.User.ID.String(), codersdk.CreateTokenRequest{})
	if err != nil {
		return xerrors.Errorf("create token: %w", err)
	}
	pu.SessionToken = tokenRes.Key
	pu.tokenID = tokenIDFromKey(tokenRes.Key)

	return nil
}

// restoreUsers revokes the minted token and demotes each admin user back to its
// original roles. It processes every user best-effort, never deletes users, and
// returns the joined errors of any failures.
func restoreUsers(ctx context.Context, metrics *notifications.Metrics, client *codersdk.Client, users []preparedUser) error {
	var errs error
	for _, u := range users {
		if u.User.ID == uuid.Nil {
			continue
		}
		if u.tokenID != "" {
			if err := client.DeleteAPIKey(ctx, u.User.ID.String(), u.tokenID); err != nil {
				metrics.AddError("revoke_token")
				errs = errors.Join(errs, xerrors.Errorf("revoke token for user %q: %w", u.User.ID, err))
			}
		}
		// We only ever promote users that were not already admins, so any admin
		// here was promoted by us and must be demoted.
		if u.IsAdmin {
			if _, err := client.UpdateUserRoles(ctx, u.User.ID.String(), codersdk.UpdateRoles{Roles: u.originalRoles}); err != nil {
				metrics.AddError("demote_user")
				errs = errors.Join(errs, xerrors.Errorf("demote user %q: %w", u.User.ID, err))
			}
		}
	}
	return errs
}

// createAndLoginUsers creates and logs in each user in the given slice,
// sequentially, stopping and returning on the first failure. Like prepareUsers
// it is all-or-nothing and intended to run over disjoint slices via runSharded,
// so the caller aborts and deletes any users already created on failure.
func createAndLoginUsers(ctx context.Context, metrics *notifications.Metrics, client *codersdk.Client, orgID uuid.UUID, users []preparedUser) error {
	for i := range users {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := createAndLoginUser(ctx, client, orgID, &users[i]); err != nil {
			metrics.AddError("create_user")
			return xerrors.Errorf("create user %q: %w", users[i].username, err)
		}
	}
	return nil
}

// createAndLoginUser creates the user in orgID (as a template admin when
// designated) and logs in to obtain a session token, filling in the mutable
// fields of pu. pu.User is set as soon as the user exists so a later failure
// still lets cleanup delete it.
func createAndLoginUser(ctx context.Context, client *codersdk.Client, orgID uuid.UUID, pu *preparedUser) error {
	password, err := cryptorand.String(16)
	if err != nil {
		return xerrors.Errorf("generate password: %w", err)
	}

	user, err := client.CreateUserWithOrgs(ctx, codersdk.CreateUserRequestWithOrgs{
		OrganizationIDs: []uuid.UUID{orgID},
		Username:        pu.username,
		Email:           pu.email,
		Password:        password,
	})
	if err != nil {
		return xerrors.Errorf("create user: %w", err)
	}
	pu.User = user

	if pu.IsAdmin {
		if _, err := client.UpdateUserRoles(ctx, user.ID.String(), codersdk.UpdateRoles{Roles: []string{codersdk.RoleTemplateAdmin}}); err != nil {
			return xerrors.Errorf("assign template admin role: %w", err)
		}
	}

	// LoginWithPassword is a public endpoint that ignores the client's own
	// session token, so reusing the shared client's connection pool is safe.
	loginRes, err := client.LoginWithPassword(ctx, codersdk.LoginWithPasswordRequest{
		Email:    pu.email,
		Password: password,
	})
	if err != nil {
		return xerrors.Errorf("login as new user: %w", err)
	}
	pu.SessionToken = loginRes.SessionToken

	return nil
}

// deleteUsers deletes each created user by ID, best-effort. It only ever deletes
// users this test created (their IDs come from CreateUserWithOrgs), so it never
// touches the deployment's existing users, and returns the joined errors of any
// failures.
func deleteUsers(ctx context.Context, metrics *notifications.Metrics, client *codersdk.Client, users []preparedUser) error {
	var errs error
	for _, u := range users {
		if u.User.ID == uuid.Nil {
			continue
		}
		if err := client.DeleteUser(ctx, u.User.ID); err != nil {
			metrics.AddError("delete_user")
			errs = errors.Join(errs, xerrors.Errorf("delete user %q: %w", u.User.ID, err))
		}
	}
	return errs
}

func userHasRole(user codersdk.User, role string) bool {
	for _, r := range user.Roles {
		if r.Name == role {
			return true
		}
	}
	return false
}

func userRoleNames(user codersdk.User) []string {
	names := make([]string, 0, len(user.Roles))
	for _, r := range user.Roles {
		names = append(names, r.Name)
	}
	return names
}

// tokenIDFromKey extracts the key ID from a session token of the form
// "<keyID>-<secret>".
func tokenIDFromKey(key string) string {
	for i := 0; i < len(key); i++ {
		if key[i] == '-' {
			return key[:i]
		}
	}
	return ""
}
