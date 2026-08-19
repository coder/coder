package usage

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/pproflabel"
	"github.com/coder/coder/v2/coderd/usage/usagetypes"
	"github.com/coder/coder/v2/cryptorand"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/quartz"
)

const (
	tallymanURL         = "https://tallyman-prod.coder.com"
	tallymanIngestURLV1 = tallymanURL + "/api/v1/events/ingest"

	tallymanPublishInitialMinimumDelay = 5 * time.Minute
	// Chosen to be a prime number and not a multiple of 5 like many other
	// recurring tasks.
	tallymanPublishInterval  = 17 * time.Minute
	tallymanPublishTimeout   = 30 * time.Second
	tallymanPublishBatchSize = 100
	// usagePublishDBTimeout bounds each database phase of a publish
	// iteration so a stalled database cannot block the single publish loop
	// indefinitely.
	usagePublishDBTimeout = 30 * time.Second
	// usagePublishingWindow matches the 30-day selection window in
	// SelectUsageEventsForPublishing: events older than this are never
	// published, so their publish-failure rows are pruned.
	usagePublishingWindow = 30 * 24 * time.Hour
	// usagePublishRejectionRetention matches the failure threshold in
	// license.UsagePublishingFailureThreshold: rejections older than this
	// are no longer recent, so their rows are pruned.
	usagePublishRejectionRetention = 24 * time.Hour
)

var errUsagePublishingDisabled = xerrors.New("usage publishing is not enabled by any license")

// Publisher publishes usage events ***somewhere***.
type Publisher interface {
	// Close closes the publisher and waits for it to finish.
	io.Closer
	// Start starts the publisher. It must only be called once.
	Start() error
}

type tallymanPublisher struct {
	ctx         context.Context
	ctxCancel   context.CancelFunc
	log         slog.Logger
	db          database.Store
	licenseKeys map[string]ed25519.PublicKey
	done        chan struct{}

	// Configured with options:
	ingestURL      string
	httpClient     *http.Client
	clock          quartz.Clock
	initialDelay   time.Duration
	publishTimeout time.Duration
}

var _ Publisher = &tallymanPublisher{}

// NewTallymanPublisher creates a Publisher that publishes usage events to
// Coder's Tallyman service.
func NewTallymanPublisher(ctx context.Context, log slog.Logger, db database.Store, keys map[string]ed25519.PublicKey, opts ...TallymanPublisherOption) Publisher {
	ctx, cancel := context.WithCancel(ctx)
	ctx = dbauthz.AsUsagePublisher(ctx) //nolint:gocritic // we intentionally want to be able to process usage events

	publisher := &tallymanPublisher{
		ctx:         ctx,
		ctxCancel:   cancel,
		log:         log,
		db:          db,
		licenseKeys: keys,
		done:        make(chan struct{}),

		ingestURL:      tallymanIngestURLV1,
		httpClient:     http.DefaultClient,
		clock:          quartz.NewReal(),
		publishTimeout: tallymanPublishTimeout,
	}
	for _, opt := range opts {
		opt(publisher)
	}
	return publisher
}

type TallymanPublisherOption func(*tallymanPublisher)

// PublisherWithHTTPClient sets the HTTP client to use for publishing usage events.
func PublisherWithHTTPClient(httpClient *http.Client) TallymanPublisherOption {
	return func(p *tallymanPublisher) {
		if httpClient == nil {
			httpClient = http.DefaultClient
		}
		p.httpClient = httpClient
	}
}

// PublisherWithClock sets the clock to use for publishing usage events.
func PublisherWithClock(clock quartz.Clock) TallymanPublisherOption {
	return func(p *tallymanPublisher) {
		p.clock = clock
	}
}

// PublisherWithIngestURL sets the ingest URL to use for publishing usage
// events.
func PublisherWithIngestURL(ingestURL string) TallymanPublisherOption {
	return func(p *tallymanPublisher) {
		p.ingestURL = ingestURL
	}
}

// PublisherWithInitialDelay sets the initial delay for the publisher.
func PublisherWithInitialDelay(initialDelay time.Duration) TallymanPublisherOption {
	return func(p *tallymanPublisher) {
		p.initialDelay = initialDelay
	}
}

// PublisherWithPublishTimeout sets the timeout for each tallyman publish
// request.
func PublisherWithPublishTimeout(timeout time.Duration) TallymanPublisherOption {
	return func(p *tallymanPublisher) {
		p.publishTimeout = timeout
	}
}

// Start implements Publisher.
func (p *tallymanPublisher) Start() error {
	ctx := p.ctx
	deploymentID, err := p.db.GetDeploymentID(ctx)
	if err != nil {
		return xerrors.Errorf("get deployment ID: %w", err)
	}
	deploymentUUID, err := uuid.Parse(deploymentID)
	if err != nil {
		return xerrors.Errorf("parse deployment ID %q: %w", deploymentID, err)
	}

	if p.initialDelay <= 0 {
		// Pick a random time between tallymanPublishInitialMinimumDelay and
		// tallymanPublishInterval.
		maxPlusDelay := tallymanPublishInterval - tallymanPublishInitialMinimumDelay
		plusDelay, err := cryptorand.Int63n(int64(maxPlusDelay))
		if err != nil {
			return xerrors.Errorf("could not generate random start delay: %w", err)
		}
		p.initialDelay = tallymanPublishInitialMinimumDelay + time.Duration(plusDelay)
	}

	if len(p.licenseKeys) == 0 {
		return xerrors.New("no license keys provided")
	}

	pproflabel.Go(ctx, pproflabel.Service(pproflabel.ServiceTallymanPublisher), func(ctx context.Context) {
		p.publishLoop(ctx, deploymentUUID)
	})
	return nil
}

func (p *tallymanPublisher) publishLoop(ctx context.Context, deploymentID uuid.UUID) {
	defer close(p.done)

	// Start the ticker with the initial delay. We will reset it to the interval
	// after the first tick.
	ticker := p.clock.NewTicker(p.initialDelay)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		err := p.publish(ctx, deploymentID)
		if err != nil {
			p.log.Warn(ctx, "publish usage events to tallyman", slog.Error(err))
		}
		ticker.Reset(tallymanPublishInterval)
	}
}

// publish publishes usage events to Tallyman in a loop until there is an error
// (or any rejection) or there are no more events to publish.
func (p *tallymanPublisher) publish(ctx context.Context, deploymentID uuid.UUID) error {
	for {
		accepted, err := p.publishOnce(ctx, deploymentID)
		if err != nil {
			return xerrors.Errorf("publish usage events to tallyman: %w", err)
		}
		if accepted < tallymanPublishBatchSize {
			// We published less than the batch size, so we're done.
			return nil
		}
	}
}

// publishOnce publishes up to tallymanPublishBatchSize usage events to
// tallyman. It returns the number of successfully published events.
func (p *tallymanPublisher) publishOnce(ctx context.Context, deploymentID uuid.UUID) (int, error) {
	// The pre-publish database reads run under their own bounded context so
	// a stalled database cannot block the publish loop indefinitely.
	selectCtx, selectCtxCancel := context.WithTimeout(ctx, usagePublishDBTimeout)
	defer selectCtxCancel()

	// Prune failure rows whose events aged out of the publish window on
	// every cycle, before the license check, so the failures relation stays
	// pruned even while publishing is disabled or idle. Without this a
	// disabled interval longer than the window would accumulate stale rows
	// that keep raising EUP01, and the detection probe's inserted_at index
	// walk would have to skip every one of them on the first refresh after
	// re-enablement. Rejections need no idle prune: detection bounds them
	// by a range on their index's leading column, so stale rows only
	// occupy space until the next publishing cycle.
	if err := p.db.PruneUsageEventsPublishFailures(selectCtx, dbtime.Time(p.clock.Now().Add(-usagePublishingWindow))); err != nil {
		p.log.Warn(ctx, "prune usage events publish failures", slog.Error(err))
	}

	licenseJwt, err := p.getBestLicenseJWT(selectCtx)
	if xerrors.Is(err, errUsagePublishingDisabled) {
		return 0, nil
	} else if err != nil {
		return 0, xerrors.Errorf("find usage publishing license: %w", err)
	}

	events, err := p.db.SelectUsageEventsForPublishing(selectCtx, dbtime.Time(p.clock.Now()))
	if err != nil {
		return 0, xerrors.Errorf("select usage events for publishing: %w", err)
	}
	if len(events) == 0 {
		// No events to publish; the prune above already ran.
		return 0, nil
	}

	var (
		eventIDs    = make(map[string]struct{})
		tallymanReq = usagetypes.TallymanV1IngestRequest{
			Events: make([]usagetypes.TallymanV1IngestEvent, 0, len(events)),
		}
	)
	for _, event := range events {
		eventIDs[event.ID] = struct{}{}
		eventType := usagetypes.UsageEventType(event.EventType)
		if !eventType.Valid() {
			// This should never happen due to the check constraint in the
			// database.
			return 0, xerrors.Errorf("event %q has an invalid event type %q", event.ID, event.EventType)
		}
		tallymanReq.Events = append(tallymanReq.Events, usagetypes.TallymanV1IngestEvent{
			ID:        event.ID,
			EventType: eventType,
			EventData: event.EventData,
			CreatedAt: event.CreatedAt,
		})
	}
	if len(eventIDs) != len(events) {
		// This should never happen due to the unique constraint in the
		// database.
		return 0, xerrors.Errorf("duplicate event IDs found in events for publishing")
	}

	// Only the tallyman request runs under the publish timeout. The
	// post-publish database update and failing-events marker outcome below
	// get a fresh bounded context instead: a request that consumes the
	// entire timeout (e.g. tallyman hanging until the deadline) must still
	// persist its failure, or the in-flight attempt markers on the selected
	// rows would hide the failure from detection until they expire, only
	// for the next retry to refresh them and hide it again.
	publishCtx, publishCtxCancel := context.WithTimeout(ctx, p.publishTimeout)
	resp, err := p.sendPublishRequest(publishCtx, deploymentID, licenseJwt, tallymanReq)
	publishCtxCancel()
	allFailed := err != nil
	if err != nil {
		p.log.Warn(ctx, "failed to send publish request to tallyman", slog.F("count", len(events)), slog.Error(err))
		// Fake a response with all events temporarily rejected.
		resp = usagetypes.TallymanV1IngestResponse{
			AcceptedEvents: []usagetypes.TallymanV1IngestAcceptedEvent{},
			RejectedEvents: make([]usagetypes.TallymanV1IngestRejectedEvent, len(events)),
		}
		for i, event := range events {
			resp.RejectedEvents[i] = usagetypes.TallymanV1IngestRejectedEvent{
				ID:        event.ID,
				Message:   fmt.Sprintf("failed to publish to tallyman: %v", err),
				Permanent: false,
			}
		}
	} else {
		p.log.Debug(ctx, "published usage events to tallyman", slog.F("accepted", len(resp.AcceptedEvents)), slog.F("rejected", len(resp.RejectedEvents)))
	}

	if len(resp.AcceptedEvents)+len(resp.RejectedEvents) != len(events) {
		p.log.Warn(ctx, "tallyman returned a different number of events than we sent", slog.F("sent", len(events)), slog.F("accepted", len(resp.AcceptedEvents)), slog.F("rejected", len(resp.RejectedEvents)))
	}

	acceptedEvents := make(map[string]*usagetypes.TallymanV1IngestAcceptedEvent)
	rejectedEvents := make(map[string]*usagetypes.TallymanV1IngestRejectedEvent)
	for _, event := range resp.AcceptedEvents {
		acceptedEvents[event.ID] = &event
	}
	for _, event := range resp.RejectedEvents {
		rejectedEvents[event.ID] = &event
	}

	dbUpdate := database.UpdateUsageEventsPostPublishParams{
		Now:             dbtime.Time(p.clock.Now()),
		IDs:             make([]string, len(events)),
		FailureMessages: make([]string, len(events)),
		SetPublishedAts: make([]bool, len(events)),
	}
	for i, event := range events {
		dbUpdate.IDs[i] = event.ID
		if _, ok := acceptedEvents[event.ID]; ok {
			dbUpdate.FailureMessages[i] = ""
			dbUpdate.SetPublishedAts[i] = true
			continue
		}
		if rejectedEvent, ok := rejectedEvents[event.ID]; ok {
			message := rejectedEvent.Message
			if message == "" {
				// The stored failure_message discriminates rejections from
				// successes (UpdateUsageEventsPostPublish turns an empty
				// message into NULL), so an empty rejection message must
				// not be stored verbatim or the rejection would be
				// indistinguishable from a successful publish.
				message = "tallyman rejected the event without a message"
			}
			dbUpdate.FailureMessages[i] = message
			dbUpdate.SetPublishedAts[i] = rejectedEvent.Permanent
			continue
		}
		// It's not good if this path gets hit, but we'll handle it as if it
		// was a temporary rejection.
		dbUpdate.FailureMessages[i] = "tallyman did not include the event in the response"
		dbUpdate.SetPublishedAts[i] = false
	}

	// Collate rejected events into a single map of ID to failure message for
	// logging. We only want to log once.
	// If all events failed, we'll log the overall error above.
	if !allFailed {
		rejectionReasonsForLog := make(map[string]string)
		for i, id := range dbUpdate.IDs {
			failureMessage := dbUpdate.FailureMessages[i]
			if failureMessage == "" {
				continue
			}
			setPublishedAt := dbUpdate.SetPublishedAts[i]
			if setPublishedAt {
				failureMessage = "permanently rejected: " + failureMessage
			}
			rejectionReasonsForLog[id] = failureMessage
		}
		if len(rejectionReasonsForLog) > 0 {
			p.log.Warn(ctx, "tallyman rejected usage events", slog.F("count", len(rejectionReasonsForLog)), slog.F("event_failure_reasons", rejectionReasonsForLog))
		}
	}

	// The post-publish update gets its own bounded context, separate from
	// the request's (see above) so a request that consumed its entire
	// deadline still persists its outcome, and separate from the loop's so
	// a stalled database cannot block it indefinitely. Schema triggers on
	// usage_events do the publish-failure bookkeeping atomically within
	// this single statement, so every writer participates: publishing an
	// event (including permanent rejections, which are terminal and also
	// recorded as rejections) deletes its failure row, and a concluded
	// attempt that leaves an event unpublished records one. Publish
	// failure detection reads the failures and rejections relations
	// instead of scanning usage_events, and because rows are keyed by
	// event ID, a batch only ever resolves its own events; failures
	// another replica's rows justify stay recorded until those exact rows
	// publish.
	updateCtx, updateCtxCancel := context.WithTimeout(ctx, usagePublishDBTimeout)
	defer updateCtxCancel()
	now := p.clock.Now()
	windowStart := dbtime.Time(now.Add(-usagePublishingWindow))
	err = p.db.UpdateUsageEventsPostPublish(updateCtx, dbUpdate)
	if err != nil {
		// The update failing leaves every batch row unpublished with a
		// fresh in-flight attempt marker that hides it from the stuck
		// probe, so record the whole batch as failing on a fresh bounded
		// context (the update may have consumed this one's deadline). The
		// insert verifies against usage_events, so rows another cycle has
		// since published are skipped. Best-effort: a database problem
		// specific to the update must not also silence failure detection,
		// and the next cycle retries.
		failCtx, failCtxCancel := context.WithTimeout(ctx, usagePublishDBTimeout)
		defer failCtxCancel()
		if failErr := p.db.UpsertUsageEventsPublishFailures(failCtx, database.UpsertUsageEventsPublishFailuresParams{
			IDs:         dbUpdate.IDs,
			WindowStart: windowStart,
		}); failErr != nil {
			p.log.Warn(ctx, "record usage events publish failures", slog.Error(failErr))
		}
		return 0, xerrors.Errorf("update usage events post publish: %w", err)
	}

	// Best-effort housekeeping, deliberately outside the batch outcome: a
	// prune timing out or failing must not undo or fail the committed
	// update (that would locally unpublish tallyman-accepted events and
	// resend them). The prune is LIMIT-bounded and retried every cycle, so
	// a missed one only defers cleanup. The failures relation is pruned at
	// the top of every cycle instead, so it stays pruned even while
	// publishing is disabled.
	if err := p.db.PruneUsageEventsPublishRejections(updateCtx, dbtime.Time(now.Add(-usagePublishRejectionRetention))); err != nil {
		p.log.Warn(ctx, "prune usage events publish rejections", slog.Error(err))
	}

	var returnErr error
	if len(resp.RejectedEvents) > 0 {
		returnErr = xerrors.New("some events were rejected by tallyman")
	}
	return len(resp.AcceptedEvents), returnErr
}

// getBestLicenseJWT returns the best license JWT to use for the request. The
// criteria is as follows:
// - The license must be valid and active (after nbf, before exp)
// - The license must have usage publishing enabled
// The most recently issued (iat) license is chosen.
//
// If no licenses are found or none have usage publishing enabled,
// errUsagePublishingDisabled is returned.
func (p *tallymanPublisher) getBestLicenseJWT(ctx context.Context) (string, error) {
	licenses, err := p.db.GetUnexpiredLicenses(ctx)
	if err != nil {
		return "", xerrors.Errorf("get unexpired licenses: %w", err)
	}
	if len(licenses) == 0 {
		return "", errUsagePublishingDisabled
	}

	type licenseJWTWithClaims struct {
		Claims *license.Claims
		Raw    string
	}

	var bestLicense licenseJWTWithClaims
	for _, dbLicense := range licenses {
		claims, err := license.ParseClaims(dbLicense.JWT, p.licenseKeys)
		if err != nil {
			p.log.Warn(ctx, "failed to parse license claims", slog.F("license_id", dbLicense.ID), slog.Error(err))
			continue
		}
		if claims.AccountType != license.AccountTypeSalesforce {
			// Non-Salesforce accounts cannot be tracked as they do not have a
			// trusted Salesforce opportunity ID encoded in the license.
			continue
		}
		if !claims.PublishUsageData {
			// Publishing is disabled.
			continue
		}

		// Otherwise, if it's issued more recently, it's the best license.
		// IssuedAt is verified to be non-nil in license.ParseClaims.
		if bestLicense.Claims == nil || claims.IssuedAt.Time.After(bestLicense.Claims.IssuedAt.Time) {
			bestLicense = licenseJWTWithClaims{
				Claims: claims,
				Raw:    dbLicense.JWT,
			}
		}
	}

	if bestLicense.Raw == "" {
		return "", errUsagePublishingDisabled
	}

	return bestLicense.Raw, nil
}

func (p *tallymanPublisher) sendPublishRequest(ctx context.Context, deploymentID uuid.UUID, licenseJwt string, req usagetypes.TallymanV1IngestRequest) (usagetypes.TallymanV1IngestResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return usagetypes.TallymanV1IngestResponse{}, err
	}

	r, err := http.NewRequestWithContext(ctx, http.MethodPost, p.ingestURL, bytes.NewReader(body))
	if err != nil {
		return usagetypes.TallymanV1IngestResponse{}, err
	}
	r.Header.Set("User-Agent", "coderd/"+buildinfo.Version())
	r.Header.Set(usagetypes.TallymanCoderLicenseKeyHeader, licenseJwt)
	r.Header.Set(usagetypes.TallymanCoderDeploymentIDHeader, deploymentID.String())

	resp, err := p.httpClient.Do(r)
	if err != nil {
		return usagetypes.TallymanV1IngestResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errBody usagetypes.TallymanV1Response
		if err := json.NewDecoder(resp.Body).Decode(&errBody); err != nil {
			errBody = usagetypes.TallymanV1Response{
				Message: fmt.Sprintf("could not decode error response body: %v", err),
			}
		}
		return usagetypes.TallymanV1IngestResponse{}, xerrors.Errorf("unexpected status code %v, error: %s", resp.StatusCode, errBody.Message)
	}

	var respBody usagetypes.TallymanV1IngestResponse
	if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		return usagetypes.TallymanV1IngestResponse{}, xerrors.Errorf("decode response body: %w", err)
	}

	return respBody, nil
}

// Close implements Publisher.
func (p *tallymanPublisher) Close() error {
	p.ctxCancel()
	<-p.done
	return nil
}
