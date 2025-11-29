package usage_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/usage/usagetypes"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/usage"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m, testutil.GoleakOptions...)
}

// TestIntegration tests the inserter and publisher by running them with a real
// database.
func TestIntegration(t *testing.T) {
	t.Parallel()
	const eventCount = 3

	ctx := testutil.Context(t, testutil.WaitLong)
	log := slogtest.Make(t, nil)
	db, _ := dbtestutil.NewDB(t)

	clock := quartz.NewMock(t)
	deploymentID, licenseJWT := configureDeployment(ctx, t, db)
	now := time.Now()

	var (
		calls   int
		handler func(req usagetypes.TallymanV1IngestRequest) any
	)
	baseURL := fakeServer(t, tallymanHandler(t, deploymentID.String(), licenseJWT, func(req usagetypes.TallymanV1IngestRequest) any {
		calls++
		t.Logf("tallyman backend received call %d", calls)

		if handler == nil {
			t.Errorf("handler is nil")
			return usagetypes.TallymanV1IngestResponse{}
		}
		return handler(req)
	}))

	inserter := usage.NewDBInserter(
		usage.InserterWithClock(clock),
	)
	// Insert an old event that should never be published.
	clock.Set(now.Add(-31 * 24 * time.Hour))
	err := inserter.InsertDiscreteUsageEvent(ctx, db, usagetypes.DCManagedAgentsV1{
		Count: 31,
	})
	require.NoError(t, err)

	// Insert the events we expect to be published.
	clock.Set(now.Add(1 * time.Second))
	for i := 0; i < eventCount; i++ {
		clock.Advance(time.Second)
		err := inserter.InsertDiscreteUsageEvent(ctx, db, usagetypes.DCManagedAgentsV1{
			Count: uint64(i + 1), // nolint:gosec // these numbers are tiny and will not overflow
		})
		require.NoErrorf(t, err, "collecting event %d", i)
	}

	// Wrap the publisher's DB in a dbauthz to ensure that the publisher has
	// enough permissions.
	authzDB := dbauthz.New(db, rbac.NewAuthorizer(prometheus.NewRegistry()), log, coderdtest.AccessControlStorePointer())
	publisher := usage.NewTallymanPublisher(ctx, log, authzDB, coderdenttest.Keys,
		usage.PublisherWithClock(clock),
		usage.PublisherWithTallymanBaseURL(baseURL),
	)
	defer publisher.Close()

	// Start the publisher with a trap.
	tickerTrap := clock.Trap().NewTicker()
	defer tickerTrap.Close()
	startErr := make(chan error)
	go func() {
		err := publisher.Start()
		testutil.AssertSend(ctx, t, startErr, err)
	}()
	tickerCall := tickerTrap.MustWait(ctx)
	tickerCall.MustRelease(ctx)
	// The initial duration will always be some time between 5m and 17m.
	require.GreaterOrEqual(t, tickerCall.Duration, 5*time.Minute)
	require.LessOrEqual(t, tickerCall.Duration, 17*time.Minute)
	require.NoError(t, testutil.RequireReceive(ctx, t, startErr))

	// Set up a trap for the ticker.Reset call.
	tickerResetTrap := clock.Trap().TickerReset()
	defer tickerResetTrap.Close()

	// Configure the handler for the first publish. This handler will accept the
	// first event, temporarily reject the second, and permanently reject the
	// third.
	var temporarilyRejectedEventID string
	handler = func(req usagetypes.TallymanV1IngestRequest) any {
		// On the first call, accept the first event, temporarily reject the
		// second, and permanently reject the third.
		acceptedEvents := make([]usagetypes.TallymanV1IngestAcceptedEvent, 1)
		rejectedEvents := make([]usagetypes.TallymanV1IngestRejectedEvent, 2)
		if assert.Len(t, req.Events, eventCount) {
			assert.JSONEqf(t, jsoninate(t, usagetypes.DCManagedAgentsV1{
				Count: 1,
			}), string(req.Events[0].EventData), "event data did not match for event %d", 0)
			acceptedEvents[0].ID = req.Events[0].ID

			temporarilyRejectedEventID = req.Events[1].ID
			assert.JSONEqf(t, jsoninate(t, usagetypes.DCManagedAgentsV1{
				Count: 2,
			}), string(req.Events[1].EventData), "event data did not match for event %d", 1)
			rejectedEvents[0].ID = req.Events[1].ID
			rejectedEvents[0].Message = "temporarily rejected"
			rejectedEvents[0].Permanent = false

			assert.JSONEqf(t, jsoninate(t, usagetypes.DCManagedAgentsV1{
				Count: 3,
			}), string(req.Events[2].EventData), "event data did not match for event %d", 2)
			rejectedEvents[1].ID = req.Events[2].ID
			rejectedEvents[1].Message = "permanently rejected"
			rejectedEvents[1].Permanent = true
		}
		return usagetypes.TallymanV1IngestResponse{
			AcceptedEvents: acceptedEvents,
			RejectedEvents: rejectedEvents,
		}
	}

	// Advance the clock to the initial tick, which should trigger the first
	// publish, then wait for the reset call. The duration will always be 17m
	// for resets (only the initial tick is variable).
	clock.Advance(tickerCall.Duration)
	tickerResetCall := tickerResetTrap.MustWait(ctx)
	require.Equal(t, 17*time.Minute, tickerResetCall.Duration)
	tickerResetCall.MustRelease(ctx)

	// The publisher should have published the events once.
	require.Equal(t, 1, calls)

	// Set the handler for the next publish call. This call should only include
	// the temporarily rejected event from earlier. This time we'll accept it.
	handler = func(req usagetypes.TallymanV1IngestRequest) any {
		assert.Len(t, req.Events, 1)
		acceptedEvents := make([]usagetypes.TallymanV1IngestAcceptedEvent, len(req.Events))
		for i, event := range req.Events {
			assert.Equal(t, temporarilyRejectedEventID, event.ID)
			acceptedEvents[i].ID = event.ID
		}
		return usagetypes.TallymanV1IngestResponse{
			AcceptedEvents: acceptedEvents,
			RejectedEvents: []usagetypes.TallymanV1IngestRejectedEvent{},
		}
	}

	// Advance the clock to the next tick and wait for the reset call.
	clock.Advance(tickerResetCall.Duration)
	tickerResetCall = tickerResetTrap.MustWait(ctx)
	tickerResetCall.MustRelease(ctx)

	// The publisher should have published the events again.
	require.Equal(t, 2, calls)

	// There should be no more publish calls after this, so set the handler to
	// nil.
	handler = nil

	// Advance the clock to the next tick.
	clock.Advance(tickerResetCall.Duration)
	tickerResetTrap.MustWait(ctx).MustRelease(ctx)

	// No publish should have taken place since there are no more events to
	// publish.
	require.Equal(t, 2, calls)

	require.NoError(t, publisher.Close())
}

func TestPublisherNoEligibleLicenses(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	log := slogtest.Make(t, nil)
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	clock := quartz.NewMock(t)

	// Configure the deployment manually.
	deploymentID := uuid.New()
	db.EXPECT().GetDeploymentID(gomock.Any()).Return(deploymentID.String(), nil).Times(1)

	var calls int
	baseURL := fakeServer(t, tallymanHandler(t, deploymentID.String(), "", func(req usagetypes.TallymanV1IngestRequest) any {
		calls++
		return usagetypes.TallymanV1IngestResponse{
			AcceptedEvents: []usagetypes.TallymanV1IngestAcceptedEvent{},
			RejectedEvents: []usagetypes.TallymanV1IngestRejectedEvent{},
		}
	}))

	publisher := usage.NewTallymanPublisher(ctx, log, db, coderdenttest.Keys,
		usage.PublisherWithClock(clock),
		usage.PublisherWithTallymanBaseURL(baseURL),
	)
	defer publisher.Close()

	// Start the publisher with a trap.
	tickerTrap := clock.Trap().NewTicker()
	defer tickerTrap.Close()
	startErr := make(chan error)
	go func() {
		err := publisher.Start()
		testutil.RequireSend(ctx, t, startErr, err)
	}()
	tickerCall := tickerTrap.MustWait(ctx)
	tickerCall.MustRelease(ctx)
	require.NoError(t, testutil.RequireReceive(ctx, t, startErr))

	// Mock zero licenses.
	db.EXPECT().GetUnexpiredLicenses(gomock.Any()).Return([]database.License{}, nil).Times(1)

	// Tick and wait for the reset call.
	tickerResetTrap := clock.Trap().TickerReset()
	defer tickerResetTrap.Close()
	clock.Advance(tickerCall.Duration)
	tickerResetCall := tickerResetTrap.MustWait(ctx)
	tickerResetCall.MustRelease(ctx)

	// The publisher should not have published the events.
	require.Equal(t, 0, calls)

	// Mock a single license with usage publishing disabled.
	licenseJWT := coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
		PublishUsageData: false,
	})
	db.EXPECT().GetUnexpiredLicenses(gomock.Any()).Return([]database.License{
		{
			ID:         1,
			JWT:        licenseJWT,
			UploadedAt: dbtime.Now(),
			Exp:        dbtime.Now().Add(48 * time.Hour), // fake
			UUID:       uuid.New(),
		},
	}, nil).Times(1)

	// Tick and wait for the reset call.
	clock.Advance(tickerResetCall.Duration)
	tickerResetTrap.MustWait(ctx).MustRelease(ctx)

	// The publisher should still not have published the events.
	require.Equal(t, 0, calls)
}

// TestPublisherClaimExpiry tests the claim query to ensure that events are not
// claimed if they've recently been claimed by another publisher.
func TestPublisherClaimExpiry(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	log := slogtest.Make(t, nil)
	db, _ := dbtestutil.NewDB(t)
	clock := quartz.NewMock(t)
	deploymentID, licenseJWT := configureDeployment(ctx, t, db)
	now := time.Now()

	var calls int
	baseURL := fakeServer(t, tallymanHandler(t, deploymentID.String(), licenseJWT, func(req usagetypes.TallymanV1IngestRequest) any {
		calls++
		return tallymanAcceptAllHandler(req)
	}))

	inserter := usage.NewDBInserter(
		usage.InserterWithClock(clock),
	)

	publisher := usage.NewTallymanPublisher(ctx, log, db, coderdenttest.Keys,
		usage.PublisherWithClock(clock),
		usage.PublisherWithTallymanBaseURL(baseURL),
		usage.PublisherWithInitialDelay(17*time.Minute),
	)
	defer publisher.Close()

	// Create an event that was claimed 1h-18m ago. The ticker has a forced
	// delay of 17m in this test.
	clock.Set(now)
	err := inserter.InsertDiscreteUsageEvent(ctx, db, usagetypes.DCManagedAgentsV1{
		Count: 1,
	})
	require.NoError(t, err)
	// Claim the event in the past. Claiming it this way via the database
	// directly means it won't be marked as published or unclaimed.
	events, err := db.SelectUsageEventsForPublishing(ctx, now.Add(-42*time.Minute))
	require.NoError(t, err)
	require.Len(t, events, 1)

	// Start the publisher with a trap.
	tickerTrap := clock.Trap().NewTicker()
	defer tickerTrap.Close()
	startErr := make(chan error)
	go func() {
		err := publisher.Start()
		testutil.RequireSend(ctx, t, startErr, err)
	}()
	tickerCall := tickerTrap.MustWait(ctx)
	require.Equal(t, 17*time.Minute, tickerCall.Duration)
	tickerCall.MustRelease(ctx)
	require.NoError(t, testutil.RequireReceive(ctx, t, startErr))

	// Set up a trap for the ticker.Reset call.
	tickerResetTrap := clock.Trap().TickerReset()
	defer tickerResetTrap.Close()

	// Advance the clock to the initial tick, which should trigger the first
	// publish, then wait for the reset call. The duration will always be 17m
	// for resets (only the initial tick is variable).
	clock.Advance(tickerCall.Duration)
	tickerResetCall := tickerResetTrap.MustWait(ctx)
	require.Equal(t, 17*time.Minute, tickerResetCall.Duration)
	tickerResetCall.MustRelease(ctx)

	// No events should have been published since none are eligible.
	require.Equal(t, 0, calls)

	// Advance the clock to the next tick and wait for the reset call.
	clock.Advance(tickerResetCall.Duration)
	tickerResetCall = tickerResetTrap.MustWait(ctx)
	tickerResetCall.MustRelease(ctx)

	// The publisher should have published the event, as it's now eligible.
	require.Equal(t, 1, calls)
}

// TestPublisherMissingEvents tests that the publisher notices events that are
// not returned by the Tallyman server and marks them as temporarily rejected.
func TestPublisherMissingEvents(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	log := slogtest.Make(t, nil)
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	deploymentID, licenseJWT := configureMockDeployment(t, db)
	clock := quartz.NewMock(t)
	now := time.Now()
	clock.Set(now)

	var calls int
	baseURL := fakeServer(t, tallymanHandler(t, deploymentID.String(), licenseJWT, func(req usagetypes.TallymanV1IngestRequest) any {
		calls++
		return usagetypes.TallymanV1IngestResponse{
			AcceptedEvents: []usagetypes.TallymanV1IngestAcceptedEvent{},
			RejectedEvents: []usagetypes.TallymanV1IngestRejectedEvent{},
		}
	}))

	publisher := usage.NewTallymanPublisher(ctx, log, db, coderdenttest.Keys,
		usage.PublisherWithClock(clock),
		usage.PublisherWithTallymanBaseURL(baseURL),
	)

	// Expect the publisher to call SelectUsageEventsForPublishing, followed by
	// UpdateUsageEventsPostPublish.
	events := []database.UsageEvent{
		{
			ID:        uuid.New().String(),
			EventType: string(usagetypes.UsageEventTypeDCManagedAgentsV1),
			EventData: []byte(jsoninate(t, usagetypes.DCManagedAgentsV1{
				Count: 1,
			})),
			CreatedAt:        now,
			PublishedAt:      sql.NullTime{},
			PublishStartedAt: sql.NullTime{},
			FailureMessage:   sql.NullString{},
		},
	}
	db.EXPECT().SelectUsageEventsForPublishing(gomock.Any(), gomock.Any()).Return(events, nil).Times(1)
	db.EXPECT().UpdateUsageEventsPostPublish(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, params database.UpdateUsageEventsPostPublishParams) error {
			assert.Equal(t, []string{events[0].ID}, params.IDs)
			assert.Equal(t, []string{"tallyman did not include the event in the response"}, params.FailureMessages)
			assert.Equal(t, []bool{false}, params.SetPublishedAts)
			return nil
		},
	).Times(1)

	// Start the publisher with a trap.
	tickerTrap := clock.Trap().NewTicker()
	defer tickerTrap.Close()
	startErr := make(chan error)
	go func() {
		err := publisher.Start()
		testutil.RequireSend(ctx, t, startErr, err)
	}()
	tickerCall := tickerTrap.MustWait(ctx)
	tickerCall.MustRelease(ctx)
	require.NoError(t, testutil.RequireReceive(ctx, t, startErr))

	// Tick and wait for the reset call.
	tickerResetTrap := clock.Trap().TickerReset()
	defer tickerResetTrap.Close()
	clock.Advance(tickerCall.Duration)
	tickerResetTrap.MustWait(ctx).MustRelease(ctx)

	// The publisher should have published the events once.
	require.Equal(t, 1, calls)

	require.NoError(t, publisher.Close())
}

func TestPublisherLicenseSelection(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	log := slogtest.Make(t, nil)
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	clock := quartz.NewMock(t)
	now := time.Now()

	// Configure the deployment manually.
	deploymentID := uuid.New()
	db.EXPECT().GetDeploymentID(gomock.Any()).Return(deploymentID.String(), nil).Times(1)

	// Insert multiple licenses:
	// 1. PublishUsageData false, type=salesforce, iat 30m ago              (ineligible, publish not enabled)
	// 2. PublishUsageData true,  type=trial,      iat 1h ago               (ineligible, not salesforce)
	// 3. PublishUsageData true,  type=salesforce, iat 30m ago, exp 10m ago (ineligible, expired)
	// 4. PublishUsageData true,  type=salesforce, iat 1h ago               (eligible)
	// 5. PublishUsageData true,  type=salesforce, iat 30m ago              (eligible, and newer!)
	badLicense1 := coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
		PublishUsageData: false,
		IssuedAt:         now.Add(-30 * time.Minute),
	})
	badLicense2 := coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
		PublishUsageData: true,
		IssuedAt:         now.Add(-1 * time.Hour),
		AccountType:      "trial",
	})
	badLicense3 := coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
		PublishUsageData: true,
		IssuedAt:         now.Add(-30 * time.Minute),
		ExpiresAt:        now.Add(-10 * time.Minute),
	})
	badLicense4 := coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
		PublishUsageData: true,
		IssuedAt:         now.Add(-1 * time.Hour),
	})
	expectedLicense := coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
		PublishUsageData: true,
		IssuedAt:         now.Add(-30 * time.Minute),
	})
	// GetUnexpiredLicenses is not supposed to return expired licenses, but for
	// the purposes of this test we're going to do it anyway.
	db.EXPECT().GetUnexpiredLicenses(gomock.Any()).Return([]database.License{
		{
			ID:         1,
			JWT:        badLicense1,
			Exp:        now.Add(48 * time.Hour), // fake times, the JWT should be checked
			UUID:       uuid.New(),
			UploadedAt: now,
		},
		{
			ID:         2,
			JWT:        badLicense2,
			Exp:        now.Add(48 * time.Hour),
			UUID:       uuid.New(),
			UploadedAt: now,
		},
		{
			ID:         3,
			JWT:        badLicense3,
			Exp:        now.Add(48 * time.Hour),
			UUID:       uuid.New(),
			UploadedAt: now,
		},
		{
			ID:         4,
			JWT:        badLicense4,
			Exp:        now.Add(48 * time.Hour),
			UUID:       uuid.New(),
			UploadedAt: now,
		},
		{
			ID:         5,
			JWT:        expectedLicense,
			Exp:        now.Add(48 * time.Hour),
			UUID:       uuid.New(),
			UploadedAt: now,
		},
	}, nil)

	called := false
	baseURL := fakeServer(t, tallymanHandler(t, deploymentID.String(), expectedLicense, func(req usagetypes.TallymanV1IngestRequest) any {
		called = true
		return tallymanAcceptAllHandler(req)
	}))

	publisher := usage.NewTallymanPublisher(ctx, log, db, coderdenttest.Keys,
		usage.PublisherWithClock(clock),
		usage.PublisherWithTallymanBaseURL(baseURL),
	)
	defer publisher.Close()

	// Start the publisher with a trap.
	tickerTrap := clock.Trap().NewTicker()
	defer tickerTrap.Close()
	startErr := make(chan error)
	go func() {
		err := publisher.Start()
		testutil.RequireSend(ctx, t, startErr, err)
	}()
	tickerCall := tickerTrap.MustWait(ctx)
	tickerCall.MustRelease(ctx)
	require.NoError(t, testutil.RequireReceive(ctx, t, startErr))

	// Mock events to be published.
	events := []database.UsageEvent{
		{
			ID:        uuid.New().String(),
			EventType: string(usagetypes.UsageEventTypeDCManagedAgentsV1),
			EventData: []byte(jsoninate(t, usagetypes.DCManagedAgentsV1{
				Count: 1,
			})),
		},
	}
	db.EXPECT().SelectUsageEventsForPublishing(gomock.Any(), gomock.Any()).Return(events, nil).Times(1)
	db.EXPECT().UpdateUsageEventsPostPublish(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, params database.UpdateUsageEventsPostPublishParams) error {
			assert.Equal(t, []string{events[0].ID}, params.IDs)
			assert.Equal(t, []string{""}, params.FailureMessages)
			assert.Equal(t, []bool{true}, params.SetPublishedAts)
			return nil
		},
	).Times(1)

	// Tick and wait for the reset call.
	tickerResetTrap := clock.Trap().TickerReset()
	defer tickerResetTrap.Close()
	clock.Advance(tickerCall.Duration)
	tickerResetTrap.MustWait(ctx).MustRelease(ctx)

	// The publisher should have published the events once.
	require.True(t, called)
}

func TestPublisherTallymanError(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	log := slogtest.Make(t, nil)
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	clock := quartz.NewMock(t)
	now := time.Now()
	clock.Set(now)

	deploymentID, licenseJWT := configureMockDeployment(t, db)
	const errorMessage = "tallyman error"
	var calls int
	baseURL := fakeServer(t, tallymanHandler(t, deploymentID.String(), licenseJWT, func(req usagetypes.TallymanV1IngestRequest) any {
		calls++
		return usagetypes.TallymanV1Response{
			Message: errorMessage,
		}
	}))

	publisher := usage.NewTallymanPublisher(ctx, log, db, coderdenttest.Keys,
		usage.PublisherWithClock(clock),
		usage.PublisherWithTallymanBaseURL(baseURL),
	)
	defer publisher.Close()

	// Start the publisher with a trap.
	tickerTrap := clock.Trap().NewTicker()
	defer tickerTrap.Close()
	startErr := make(chan error)
	go func() {
		err := publisher.Start()
		testutil.RequireSend(ctx, t, startErr, err)
	}()
	tickerCall := tickerTrap.MustWait(ctx)
	tickerCall.MustRelease(ctx)
	require.NoError(t, testutil.RequireReceive(ctx, t, startErr))

	// Mock events to be published.
	events := []database.UsageEvent{
		{
			ID:        uuid.New().String(),
			EventType: string(usagetypes.UsageEventTypeDCManagedAgentsV1),
			EventData: []byte(jsoninate(t, usagetypes.DCManagedAgentsV1{
				Count: 1,
			})),
		},
	}
	db.EXPECT().SelectUsageEventsForPublishing(gomock.Any(), gomock.Any()).Return(events, nil).Times(1)
	db.EXPECT().UpdateUsageEventsPostPublish(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, params database.UpdateUsageEventsPostPublishParams) error {
			assert.Equal(t, []string{events[0].ID}, params.IDs)
			assert.Contains(t, params.FailureMessages[0], errorMessage)
			assert.Equal(t, []bool{false}, params.SetPublishedAts)
			return nil
		},
	).Times(1)

	// Tick and wait for the reset call.
	tickerResetTrap := clock.Trap().TickerReset()
	defer tickerResetTrap.Close()
	clock.Advance(tickerCall.Duration)
	tickerResetTrap.MustWait(ctx).MustRelease(ctx)

	// The publisher should have published the events once.
	require.Equal(t, 1, calls)
}

func jsoninate(t *testing.T, v any) string {
	t.Helper()
	if e, ok := v.(usagetypes.Event); ok {
		v = e.Fields()
	}
	buf, err := json.Marshal(v)
	require.NoError(t, err)
	return string(buf)
}

func configureDeployment(ctx context.Context, t *testing.T, db database.Store) (uuid.UUID, string) {
	t.Helper()
	deploymentID := uuid.New()
	err := db.InsertDeploymentID(ctx, deploymentID.String())
	require.NoError(t, err)

	licenseRaw := coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
		PublishUsageData: true,
	})
	_, err = db.InsertLicense(ctx, database.InsertLicenseParams{
		UploadedAt: dbtime.Now(),
		JWT:        licenseRaw,
		Exp:        dbtime.Now().Add(48 * time.Hour),
		UUID:       uuid.New(),
	})
	require.NoError(t, err)

	return deploymentID, licenseRaw
}

func configureMockDeployment(t *testing.T, db *dbmock.MockStore) (uuid.UUID, string) {
	t.Helper()
	deploymentID := uuid.New()
	db.EXPECT().GetDeploymentID(gomock.Any()).Return(deploymentID.String(), nil).Times(1)

	licenseRaw := coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
		PublishUsageData: true,
	})
	db.EXPECT().GetUnexpiredLicenses(gomock.Any()).Return([]database.License{
		{
			ID:         1,
			UploadedAt: dbtime.Now(),
			JWT:        licenseRaw,
			Exp:        dbtime.Now().Add(48 * time.Hour),
			UUID:       uuid.New(),
		},
	}, nil)

	return deploymentID, licenseRaw
}

func fakeServer(t *testing.T, handler http.Handler) *url.URL {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	baseURL, err := url.Parse(server.URL)
	require.NoError(t, err)
	return baseURL
}

func tallymanHandler(t *testing.T, expectDeploymentID string, expectLicenseJWT string, handler func(req usagetypes.TallymanV1IngestRequest) any) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		t.Helper()
		licenseJWT := r.Header.Get(usagetypes.TallymanCoderLicenseKeyHeader)
		if expectLicenseJWT != "" && !assert.Equal(t, expectLicenseJWT, licenseJWT, "license JWT in request did not match") {
			rw.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(rw).Encode(usagetypes.TallymanV1Response{
				Message: "license JWT in request did not match",
			})
			return
		}

		deploymentID := r.Header.Get(usagetypes.TallymanCoderDeploymentIDHeader)
		if expectDeploymentID != "" && !assert.Equal(t, expectDeploymentID, deploymentID, "deployment ID in request did not match") {
			rw.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(rw).Encode(usagetypes.TallymanV1Response{
				Message: "deployment ID in request did not match",
			})
			return
		}

		var req usagetypes.TallymanV1IngestRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		if !assert.NoError(t, err, "could not decode request body") {
			rw.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(rw).Encode(usagetypes.TallymanV1Response{
				Message: "could not decode request body",
			})
			return
		}

		resp := handler(req)
		switch resp.(type) {
		case usagetypes.TallymanV1Response:
			rw.WriteHeader(http.StatusInternalServerError)
		default:
			rw.WriteHeader(http.StatusOK)
		}
		err = json.NewEncoder(rw).Encode(resp)
		if !assert.NoError(t, err, "could not encode response body") {
			rw.WriteHeader(http.StatusInternalServerError)
			return
		}
	})
}

func tallymanAcceptAllHandler(req usagetypes.TallymanV1IngestRequest) usagetypes.TallymanV1IngestResponse {
	acceptedEvents := make([]usagetypes.TallymanV1IngestAcceptedEvent, len(req.Events))
	for i, event := range req.Events {
		acceptedEvents[i].ID = event.ID
	}

	return usagetypes.TallymanV1IngestResponse{
		AcceptedEvents: acceptedEvents,
		RejectedEvents: []usagetypes.TallymanV1IngestRejectedEvent{},
	}
}
