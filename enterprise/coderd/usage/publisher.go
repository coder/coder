package usage

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/pproflabel"
	"github.com/coder/coder/v2/coderd/usage/usagetypes"
	"github.com/coder/coder/v2/cryptorand"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/enterprise/coderd/usage/tallymansdk"
	"github.com/coder/quartz"
)

const (
	tallymanPublishInitialMinimumDelay = 5 * time.Minute
	// Chosen to be a prime number and not a multiple of 5 like many other
	// recurring tasks.
	tallymanPublishInterval  = 17 * time.Minute
	tallymanPublishTimeout   = 30 * time.Second
	tallymanPublishBatchSize = 100
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
	baseURL      *url.URL
	httpClient   *http.Client
	clock        quartz.Clock
	initialDelay time.Duration
}

var _ Publisher = &tallymanPublisher{}

// NewTallymanPublisher creates a Publisher that publishes usage events to
// Coder's Tallyman service.
func NewTallymanPublisher(ctx context.Context, log slog.Logger, db database.Store, keys map[string]ed25519.PublicKey, opts ...TallymanPublisherOption) Publisher {
	ctx, cancel := context.WithCancel(ctx)
	ctx = dbauthz.AsUsagePublisher(ctx) //nolint:gocritic // we intentionally want to be able to process usage events

	baseURL, _ := url.Parse(tallymansdk.DefaultURL)
	publisher := &tallymanPublisher{
		ctx:         ctx,
		ctxCancel:   cancel,
		log:         log,
		db:          db,
		licenseKeys: keys,
		done:        make(chan struct{}),

		baseURL:    baseURL,
		httpClient: http.DefaultClient,
		clock:      quartz.NewReal(),
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
// events. The base URL is extracted from the ingest URL.
func PublisherWithIngestURL(ingestURL string) TallymanPublisherOption {
	return func(p *tallymanPublisher) {
		parsed, err := url.Parse(ingestURL)
		if err != nil {
			// This shouldn't happen in practice, but if it does, keep the default.
			return
		}
		p.baseURL = &url.URL{
			Scheme: parsed.Scheme,
			Host:   parsed.Host,
		}
	}
}

// PublisherWithInitialDelay sets the initial delay for the publisher.
func PublisherWithInitialDelay(initialDelay time.Duration) TallymanPublisherOption {
	return func(p *tallymanPublisher) {
		p.initialDelay = initialDelay
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
		publishCtx, publishCtxCancel := context.WithTimeout(ctx, tallymanPublishTimeout)
		accepted, err := p.publishOnce(publishCtx, deploymentID)
		publishCtxCancel()
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
	licenseJwt, err := p.getBestLicenseJWT(ctx)
	if xerrors.Is(err, errUsagePublishingDisabled) {
		return 0, nil
	} else if err != nil {
		return 0, xerrors.Errorf("find usage publishing license: %w", err)
	}

	events, err := p.db.SelectUsageEventsForPublishing(ctx, dbtime.Time(p.clock.Now()))
	if err != nil {
		return 0, xerrors.Errorf("select usage events for publishing: %w", err)
	}
	if len(events) == 0 {
		// No events to publish.
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

	resp, err := p.sendPublishRequest(ctx, deploymentID, licenseJwt, tallymanReq)
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
			dbUpdate.FailureMessages[i] = rejectedEvent.Message
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

	err = p.db.UpdateUsageEventsPostPublish(ctx, dbUpdate)
	if err != nil {
		return 0, xerrors.Errorf("update usage events post publish: %w", err)
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
	// Create a new SDK client for this request.
	// We create it per-request since the license key may change.
	sdkClient := tallymansdk.New(
		p.baseURL,
		licenseJwt,
		deploymentID,
		tallymansdk.WithHTTPClient(p.httpClient),
	)

	return sdkClient.PublishUsageEvents(ctx, req)
}

// Close implements Publisher.
func (p *tallymanPublisher) Close() error {
	p.ctxCancel()
	<-p.done
	return nil
}
