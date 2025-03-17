package provisionerdserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/database/provisionerjobs"
	"github.com/coder/coder/v2/coderd/database/pubsub"
)

const (
	dbMaxBackoff = 10 * time.Second
	// backPollDuration is the period for the backup polling described in Acquirer comment
	backupPollDuration = 30 * time.Second
)

// Acquirer is shared among multiple routines that need to call
// database.Store.AcquireProvisionerJob. The callers that acquire jobs are called "acquirees".  The
// goal is to minimize polling the database (i.e. lower our average query rate) and simplify the
// acquiree's logic by handling retrying the database if a job is not available at the time of the
// call.
//
// When multiple acquirees share a set of provisioner types and tags, we define them as part of the
// same "domain".  Only one acquiree from each domain may query the database at a time.  If the
// database returns no jobs for that acquiree, the entire domain waits until the Acquirer is
// notified over the pubsub of a new job acceptable to the domain.
//
// As a backup to pubsub notifications, each domain is allowed to query periodically once every 30s.
// This ensures jobs are not stuck permanently if the service that created them fails to publish
// (e.g. a crash).
type Acquirer struct {
	ctx    context.Context
	logger slog.Logger
	store  AcquirerStore
	ps     pubsub.Pubsub

	mu sync.Mutex
	q  map[dKey]domain

	// testing only
	backupPollDuration time.Duration
}

type AcquirerOption func(*Acquirer)

func TestingBackupPollDuration(dur time.Duration) AcquirerOption {
	return func(a *Acquirer) {
		a.backupPollDuration = dur
	}
}

// AcquirerStore is the subset of database.Store that the Acquirer needs
type AcquirerStore interface {
	AcquireProvisionerJob(context.Context, database.AcquireProvisionerJobParams) (database.ProvisionerJob, error)
}

func NewAcquirer(ctx context.Context, logger slog.Logger, store AcquirerStore, ps pubsub.Pubsub,
	opts ...AcquirerOption,
) *Acquirer {
	a := &Acquirer{
		ctx:                ctx,
		logger:             logger,
		store:              store,
		ps:                 ps,
		q:                  make(map[dKey]domain),
		backupPollDuration: backupPollDuration,
	}
	for _, opt := range opts {
		opt(a)
	}
	a.subscribe()
	return a
}

// AcquireJob acquires a job with one of the given provisioner types and compatible
// tags from the database.  The call blocks until a job is acquired, the context is
// done, or the database returns an error _other_ than that no jobs are available.
// If no jobs are available, this method handles retrying as appropriate.
func (a *Acquirer) AcquireJob(
	ctx context.Context, organization uuid.UUID, worker uuid.UUID, pt []database.ProvisionerType, tags Tags,
) (
	retJob database.ProvisionerJob, retErr error,
) {
	logger := a.logger.With(
		slog.F("organization_id", organization),
		slog.F("worker_id", worker),
		slog.F("provisioner_types", pt),
		slog.F("tags", tags))
	logger.Debug(ctx, "acquiring job")
	dk := domainKey(organization, pt, tags)
	dbTags, err := tags.ToJSON()
	if err != nil {
		return database.ProvisionerJob{}, err
	}
	// buffer of 1 so that cancel doesn't deadlock while writing to the channel
	clearance := make(chan struct{}, 1)
	for {
		a.want(organization, pt, tags, clearance)
		select {
		case <-ctx.Done():
			err := ctx.Err()
			logger.Debug(ctx, "acquiring job canceled", slog.Error(err))
			internalError := a.cancel(dk, clearance)
			if internalError != nil {
				// internalError takes precedence
				return database.ProvisionerJob{}, internalError
			}
			return database.ProvisionerJob{}, err
		case <-clearance:
			logger.Debug(ctx, "got clearance to call database")
			job, err := a.store.AcquireProvisionerJob(ctx, database.AcquireProvisionerJobParams{
				OrganizationID: organization,
				StartedAt: sql.NullTime{
					Time:  dbtime.Now(),
					Valid: true,
				},
				WorkerID: uuid.NullUUID{
					UUID:  worker,
					Valid: true,
				},
				Types:           pt,
				ProvisionerTags: dbTags,
			})
			if errors.Is(err, sql.ErrNoRows) {
				logger.Debug(ctx, "no job available")
				continue
			}
			// we are not going to retry, so signal we are done
			internalError := a.done(dk, clearance)
			if internalError != nil {
				// internal error takes precedence
				return database.ProvisionerJob{}, internalError
			}
			if err != nil {
				logger.Warn(ctx, "error attempting to acquire job", slog.Error(err))
				return database.ProvisionerJob{}, xerrors.Errorf("failed to acquire job: %w", err)
			}
			logger.Debug(ctx, "successfully acquired job")
			return job, nil
		}
	}
}

// want signals that an acquiree wants clearance to query for a job with the given dKey.
func (a *Acquirer) want(organization uuid.UUID, pt []database.ProvisionerType, tags Tags, clearance chan<- struct{}) {
	dk := domainKey(organization, pt, tags)
	a.mu.Lock()
	defer a.mu.Unlock()
	cleared := false
	d, ok := a.q[dk]
	if !ok {
		ctx, cancel := context.WithCancel(a.ctx)
		d = domain{
			ctx:            ctx,
			cancel:         cancel,
			a:              a,
			key:            dk,
			pt:             pt,
			tags:           tags,
			organizationID: organization,
			acquirees:      make(map[chan<- struct{}]*acquiree),
		}
		a.q[dk] = d
		go d.poll(a.backupPollDuration)
		// this is a new request for this dKey, so is cleared.
		cleared = true
	}
	w, ok := d.acquirees[clearance]
	if !ok {
		w = &acquiree{clearance: clearance}
		d.acquirees[clearance] = w
	}
	// pending means that we got a job posting for this dKey while we were
	// querying, so we should clear this acquiree to retry another time.
	if w.pending {
		cleared = true
		w.pending = false
	}
	w.inProgress = cleared
	if cleared {
		// this won't block because clearance is buffered.
		clearance <- struct{}{}
	}
}

// cancel signals that an acquiree no longer wants clearance to query.  Any error returned is a serious internal error
// indicating that integrity of the internal state is corrupted by a code bug.
func (a *Acquirer) cancel(dk dKey, clearance chan<- struct{}) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	d, ok := a.q[dk]
	if !ok {
		// this is a code error, as something removed the domain early, or cancel
		// was called twice.
		err := xerrors.New("cancel for domain that doesn't exist")
		a.logger.Critical(a.ctx, "internal error", slog.Error(err))
		return err
	}
	w, ok := d.acquirees[clearance]
	if !ok {
		// this is a code error, as something removed the acquiree early, or cancel
		// was called twice.
		err := xerrors.New("cancel for an acquiree that doesn't exist")
		a.logger.Critical(a.ctx, "internal error", slog.Error(err))
		return err
	}
	delete(d.acquirees, clearance)
	if w.inProgress && len(d.acquirees) > 0 {
		// this one canceled before querying, so give another acquiree a chance
		// instead
		for _, other := range d.acquirees {
			if other.inProgress {
				err := xerrors.New("more than one acquiree in progress for same key")
				a.logger.Critical(a.ctx, "internal error", slog.Error(err))
				return err
			}
			other.inProgress = true
			other.clearance <- struct{}{}
			break // just one
		}
	}
	if len(d.acquirees) == 0 {
		d.cancel()
		delete(a.q, dk)
	}
	return nil
}

// done signals that the acquiree has completed acquiring a job (usually successfully, but we also get this call if
// there is a database error other than ErrNoRows).  Any error returned is a serious internal error indicating that
// integrity of the internal state is corrupted by a code bug.
func (a *Acquirer) done(dk dKey, clearance chan struct{}) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	d, ok := a.q[dk]
	if !ok {
		// this is a code error, as something removed the domain early, or done
		// was called twice.
		err := xerrors.New("done for a domain that doesn't exist")
		a.logger.Critical(a.ctx, "internal error", slog.Error(err))
		return err
	}
	w, ok := d.acquirees[clearance]
	if !ok {
		// this is a code error, as something removed the dKey early, or done
		// was called twice.
		err := xerrors.New("done for an acquiree that doesn't exist")
		a.logger.Critical(a.ctx, "internal error", slog.Error(err))
		return err
	}
	if !w.inProgress {
		err := xerrors.New("done acquiree was not in progress")
		a.logger.Critical(a.ctx, "internal error", slog.Error(err))
		return err
	}
	delete(d.acquirees, clearance)
	if len(d.acquirees) == 0 {
		d.cancel()
		delete(a.q, dk)
		return nil
	}
	// in the mainline, this means that the acquiree successfully got a job.
	// if any others are waiting, clear one of them to try to get a job next so
	// that we process the jobs until there are no more acquirees or the database
	// is empty of jobs meeting our criteria
	for _, other := range d.acquirees {
		if other.inProgress {
			err := xerrors.New("more than one acquiree in progress for same key")
			a.logger.Critical(a.ctx, "internal error", slog.Error(err))
			return err
		}
		other.inProgress = true
		other.clearance <- struct{}{}
		break // just one
	}
	return nil
}

func (a *Acquirer) subscribe() {
	subscribed := make(chan struct{})
	go func() {
		defer close(subscribed)
		eb := backoff.NewExponentialBackOff()
		eb.MaxElapsedTime = 0 // retry indefinitely
		eb.MaxInterval = dbMaxBackoff
		bkoff := backoff.WithContext(eb, a.ctx)
		var cancel context.CancelFunc
		err := backoff.Retry(func() error {
			cancelFn, err := a.ps.SubscribeWithErr(provisionerjobs.EventJobPosted, a.jobPosted)
			if err != nil {
				a.logger.Warn(a.ctx, "failed to subscribe to job postings", slog.Error(err))
				return err
			}
			cancel = cancelFn
			return nil
		}, bkoff)
		if err != nil {
			if a.ctx.Err() == nil {
				a.logger.Error(a.ctx, "code bug: retry failed before context canceled", slog.Error(err))
			}
			return
		}
		defer cancel()
		bkoff.Reset()
		a.logger.Debug(a.ctx, "subscribed to job postings")

		// unblock the outer function from returning
		subscribed <- struct{}{}

		// hold subscriptions open until context is canceled
		<-a.ctx.Done()
	}()
	<-subscribed
}

func (a *Acquirer) jobPosted(ctx context.Context, message []byte, err error) {
	if errors.Is(err, pubsub.ErrDroppedMessages) {
		a.logger.Warn(a.ctx, "pubsub may have dropped job postings")
		a.clearOrPendAll()
		return
	}
	if err != nil {
		a.logger.Warn(a.ctx, "unhandled pubsub error", slog.Error(err))
		return
	}
	posting := provisionerjobs.JobPosting{}
	err = json.Unmarshal(message, &posting)
	if err != nil {
		a.logger.Error(a.ctx, "unable to parse job posting",
			slog.F("message", string(message)),
			slog.Error(err),
		)
		return
	}
	a.logger.Debug(ctx, "got job posting", slog.F("posting", posting))

	a.mu.Lock()
	defer a.mu.Unlock()
	for _, d := range a.q {
		if d.contains(posting) {
			a.clearOrPendLocked(d)
			// we only need to wake up a single domain since there is only one
			// new job available
			return
		}
	}
}

func (a *Acquirer) clearOrPendAll() {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, d := range a.q {
		a.clearOrPendLocked(d)
	}
}

func (a *Acquirer) clearOrPend(d domain) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(d.acquirees) == 0 {
		// this can happen if the domain is removed right around the time the
		// backup poll (which calls this function) triggers.  Nothing to do
		// since there are no acquirees.
		return
	}
	a.clearOrPendLocked(d)
}

func (*Acquirer) clearOrPendLocked(d domain) {
	// MUST BE CALLED HOLDING THE a.mu LOCK
	var nominee *acquiree
	for _, w := range d.acquirees {
		if nominee == nil {
			nominee = w
		}
		// acquiree in progress always takes precedence, since we don't want to
		// wake up more than one acquiree per dKey at a time.
		if w.inProgress {
			nominee = w
			break
		}
	}
	if nominee.inProgress {
		nominee.pending = true
		return
	}
	nominee.inProgress = true
	nominee.clearance <- struct{}{}
}

type dKey string

// domainKey generates a canonical map key for the given provisioner types and
// tags.  It uses the null byte (0x00) as a delimiter because it is an
// unprintable control character and won't show up in any "reasonable" set of
// string tags, even in non-Latin scripts.  It is important that Tags are
// validated not to contain this control character prior to use.
func domainKey(orgID uuid.UUID, pt []database.ProvisionerType, tags Tags) dKey {
	sb := strings.Builder{}
	_, _ = sb.WriteString(orgID.String())
	_ = sb.WriteByte(0x00)

	// make a copy of pt before sorting, so that we don't mutate the original
	// slice or underlying array.
	pts := make([]database.ProvisionerType, len(pt))
	copy(pts, pt)
	slices.Sort(pts)
	for _, t := range pts {
		_, _ = sb.WriteString(string(t))
		_ = sb.WriteByte(0x00)
	}
	_ = sb.WriteByte(0x00)
	var keys []string
	for k := range tags {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		_, _ = sb.WriteString(k)
		_ = sb.WriteByte(0x00)
		_, _ = sb.WriteString(tags[k])
		_ = sb.WriteByte(0x00)
	}
	return dKey(sb.String())
}

// acquiree represents a specific client of Acquirer that wants to acquire a job
type acquiree struct {
	clearance chan<- struct{}
	// inProgress is true when the acquiree was granted clearance and a query
	// is possibly in progress.
	inProgress bool
	// pending is true if we get a job posting while a query is in progress, so
	// that we know to try again, even if we didn't get a job on the query.
	pending bool
}

// domain represents a set of acquirees with the same provisioner types and
// tags.  Acquirees in the same domain are restricted such that only one queries
// the database at a time.
type domain struct {
	ctx            context.Context
	cancel         context.CancelFunc
	a              *Acquirer
	key            dKey
	pt             []database.ProvisionerType
	tags           Tags
	organizationID uuid.UUID
	acquirees      map[chan<- struct{}]*acquiree
}

func (d domain) contains(p provisionerjobs.JobPosting) bool {
	// If the organization ID is 'uuid.Nil', this is a legacy job posting.
	// Ignore this check in the legacy case.
	if p.OrganizationID != uuid.Nil && p.OrganizationID != d.organizationID {
		return false
	}
	if !slices.Contains(d.pt, p.ProvisionerType) {
		return false
	}
	for k, v := range p.Tags {
		dv, ok := d.tags[k]
		if !ok {
			return false
		}
		if v != dv {
			return false
		}
	}
	return true
}

func (d domain) poll(dur time.Duration) {
	tkr := time.NewTicker(dur)
	defer tkr.Stop()
	for {
		select {
		case <-d.ctx.Done():
			return
		case <-tkr.C:
			d.a.clearOrPend(d)
		}
	}
}
