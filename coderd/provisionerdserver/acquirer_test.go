package provisionerdserver_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"golang.org/x/exp/slices"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/provisionerdserver"
	"github.com/coder/coder/v2/testutil"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// TestAcquirer_Store tests that a database.Store is accepted as a provisionerdserver.AcquirerStore
func TestAcquirer_Store(t *testing.T) {
	t.Parallel()
	db := dbfake.New()
	ps := pubsub.NewInMemory()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	_ = provisionerdserver.NewAcquirer(ctx, logger.Named("acquirer"), db, ps)
}

func TestAcquirer_Single(t *testing.T) {
	t.Parallel()
	fs := newFakeOrderedStore()
	ps := pubsub.NewInMemory()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	uut := provisionerdserver.NewAcquirer(ctx, logger.Named("acquirer"), fs, ps)

	workerID := uuid.New()
	pt := []database.ProvisionerType{database.ProvisionerTypeEcho}
	tags := provisionerdserver.Tags{
		"foo": "bar",
	}
	acquiree := newTestAcquiree(t, workerID, pt, tags)
	jobID := uuid.New()
	err := fs.sendCtx(ctx, database.ProvisionerJob{ID: jobID}, nil)
	require.NoError(t, err)
	acquiree.startAcquire(ctx, uut)
	job := acquiree.success(ctx)
	require.Equal(t, jobID, job.ID)
	require.Len(t, fs.params, 1)
	require.Equal(t, workerID, fs.params[0].WorkerID.UUID)
}

// TestAcquirer_MultipleSameDomain tests multiple acquirees with the same provisioners and tags
func TestAcquirer_MultipleSameDomain(t *testing.T) {
	t.Parallel()
	fs := newFakeOrderedStore()
	ps := pubsub.NewInMemory()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	uut := provisionerdserver.NewAcquirer(ctx, logger.Named("acquirer"), fs, ps)

	acquirees := make([]*testAcquiree, 0, 10)
	jobIDs := make(map[uuid.UUID]bool)
	workerIDs := make(map[uuid.UUID]bool)
	pt := []database.ProvisionerType{database.ProvisionerTypeEcho}
	tags := provisionerdserver.Tags{
		"foo": "bar",
	}
	for i := 0; i < 10; i++ {
		wID := uuid.New()
		workerIDs[wID] = true
		a := newTestAcquiree(t, wID, pt, tags)
		acquirees = append(acquirees, a)
		a.startAcquire(ctx, uut)
	}
	for i := 0; i < 10; i++ {
		jobID := uuid.New()
		jobIDs[jobID] = true
		err := fs.sendCtx(ctx, database.ProvisionerJob{ID: jobID}, nil)
		require.NoError(t, err)
	}
	gotJobIDs := make(map[uuid.UUID]bool)
	for i := 0; i < 10; i++ {
		j := acquirees[i].success(ctx)
		gotJobIDs[j.ID] = true
	}
	require.Equal(t, jobIDs, gotJobIDs)
	require.Len(t, fs.overlaps, 0)
	gotWorkerCalls := make(map[uuid.UUID]bool)
	for _, params := range fs.params {
		gotWorkerCalls[params.WorkerID.UUID] = true
	}
	require.Equal(t, workerIDs, gotWorkerCalls)
}

// TestAcquirer_WaitsOnNoJobs tests that after a call that returns no jobs, Acquirer waits for a new
// job posting before retrying
func TestAcquirer_WaitsOnNoJobs(t *testing.T) {
	t.Parallel()
	fs := newFakeOrderedStore()
	ps := pubsub.NewInMemory()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	uut := provisionerdserver.NewAcquirer(ctx, logger.Named("acquirer"), fs, ps)

	workerID := uuid.New()
	pt := []database.ProvisionerType{database.ProvisionerTypeEcho}
	tags := provisionerdserver.Tags{
		"foo": "bar",
	}
	acquiree := newTestAcquiree(t, workerID, pt, tags)
	jobID := uuid.New()
	err := fs.sendCtx(ctx, database.ProvisionerJob{}, sql.ErrNoRows)
	require.NoError(t, err)
	err = fs.sendCtx(ctx, database.ProvisionerJob{ID: jobID}, nil)
	require.NoError(t, err)
	acquiree.startAcquire(ctx, uut)
	require.Eventually(t, func() bool {
		fs.mu.Lock()
		defer fs.mu.Unlock()
		return len(fs.params) == 1
	}, testutil.WaitShort, testutil.IntervalFast)
	acquiree.requireBlocked()

	// First send in some with incompatible tags & types
	postJob(t, ps, database.ProvisionerTypeEcho, provisionerdserver.Tags{
		"cool":   "tapes",
		"strong": "bad",
	})
	postJob(t, ps, database.ProvisionerTypeEcho, provisionerdserver.Tags{
		"foo": "fighters",
	})
	postJob(t, ps, database.ProvisionerTypeTerraform, provisionerdserver.Tags{
		"foo": "bar",
	})
	acquiree.requireBlocked()

	// compatible tags
	postJob(t, ps, database.ProvisionerTypeEcho, provisionerdserver.Tags{})
	job := acquiree.success(ctx)
	require.Equal(t, jobID, job.ID)
}

// TestAcquirer_RetriesPending tests that if we get a job posting while a db call is in progress
// we retry to acquire a job immediately, even if the first call returned no jobs.  We want this
// behavior since the query that found no jobs could have resolved before the job was posted, but
// the query result could reach us later than the posting over the pubsub.
func TestAcquirer_RetriesPending(t *testing.T) {
	t.Parallel()
	fs := newFakeOrderedStore()
	ps := pubsub.NewInMemory()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	uut := provisionerdserver.NewAcquirer(ctx, logger.Named("acquirer"), fs, ps)

	workerID := uuid.New()
	pt := []database.ProvisionerType{database.ProvisionerTypeEcho}
	tags := provisionerdserver.Tags{
		"foo": "bar",
	}
	acquiree := newTestAcquiree(t, workerID, pt, tags)
	jobID := uuid.New()

	acquiree.startAcquire(ctx, uut)
	require.Eventually(t, func() bool {
		fs.mu.Lock()
		defer fs.mu.Unlock()
		return len(fs.params) == 1
	}, testutil.WaitShort, testutil.IntervalFast)

	// First call to DB is in progress.  Send in posting
	postJob(t, ps, database.ProvisionerTypeEcho, provisionerdserver.Tags{})
	// there is a race between the posting being processed and the DB call
	// returning.  In either case we should retry, but we're trying to hit the
	// case where the posting is processed first, so sleep a little bit to give
	// it a chance.
	time.Sleep(testutil.IntervalMedium)

	// Now, when first DB call returns ErrNoRows we retry.
	err := fs.sendCtx(ctx, database.ProvisionerJob{}, sql.ErrNoRows)
	require.NoError(t, err)
	err = fs.sendCtx(ctx, database.ProvisionerJob{ID: jobID}, nil)
	require.NoError(t, err)

	job := acquiree.success(ctx)
	require.Equal(t, jobID, job.ID)
}

// TestAcquirer_DifferentDomains tests that acquirees with different tags don't block each other
func TestAcquirer_DifferentDomains(t *testing.T) {
	t.Parallel()
	fs := newFakeTaggedStore(t)
	ps := pubsub.NewInMemory()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)

	pt := []database.ProvisionerType{database.ProvisionerTypeEcho}
	worker0 := uuid.New()
	tags0 := provisionerdserver.Tags{
		"worker": "0",
	}
	acquiree0 := newTestAcquiree(t, worker0, pt, tags0)
	worker1 := uuid.New()
	tags1 := provisionerdserver.Tags{
		"worker": "1",
	}
	acquiree1 := newTestAcquiree(t, worker1, pt, tags1)
	jobID := uuid.New()
	fs.jobs = []database.ProvisionerJob{
		{ID: jobID, Provisioner: database.ProvisionerTypeEcho, Tags: database.StringMap{"worker": "1"}},
	}

	uut := provisionerdserver.NewAcquirer(ctx, logger.Named("acquirer"), fs, ps)

	ctx0, cancel0 := context.WithCancel(ctx)
	defer cancel0()
	acquiree0.startAcquire(ctx0, uut)
	select {
	case params := <-fs.params:
		require.Equal(t, worker0, params.WorkerID.UUID)
	case <-ctx.Done():
		t.Fatal("timed out waiting for call to database from worker0")
	}
	acquiree0.requireBlocked()

	// worker1 should not be blocked by worker0, as they are different tags
	acquiree1.startAcquire(ctx, uut)
	job := acquiree1.success(ctx)
	require.Equal(t, jobID, job.ID)

	cancel0()
	acquiree0.requireCanceled(ctx)
}

func TestAcquirer_BackupPoll(t *testing.T) {
	t.Parallel()
	fs := newFakeOrderedStore()
	ps := pubsub.NewInMemory()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	uut := provisionerdserver.NewAcquirer(
		ctx, logger.Named("acquirer"), fs, ps,
		provisionerdserver.TestingBackupPollDuration(testutil.IntervalMedium),
	)

	workerID := uuid.New()
	pt := []database.ProvisionerType{database.ProvisionerTypeEcho}
	tags := provisionerdserver.Tags{
		"foo": "bar",
	}
	acquiree := newTestAcquiree(t, workerID, pt, tags)
	jobID := uuid.New()
	err := fs.sendCtx(ctx, database.ProvisionerJob{}, sql.ErrNoRows)
	require.NoError(t, err)
	err = fs.sendCtx(ctx, database.ProvisionerJob{ID: jobID}, nil)
	require.NoError(t, err)
	acquiree.startAcquire(ctx, uut)
	job := acquiree.success(ctx)
	require.Equal(t, jobID, job.ID)
}

// TestAcquirer_UnblockOnCancel tests that a canceled call doesn't block a call
// from the same domain.
func TestAcquirer_UnblockOnCancel(t *testing.T) {
	t.Parallel()
	fs := newFakeOrderedStore()
	ps := pubsub.NewInMemory()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)

	pt := []database.ProvisionerType{database.ProvisionerTypeEcho}
	worker0 := uuid.New()
	tags := provisionerdserver.Tags{
		"foo": "bar",
	}
	acquiree0 := newTestAcquiree(t, worker0, pt, tags)
	worker1 := uuid.New()
	acquiree1 := newTestAcquiree(t, worker1, pt, tags)
	jobID := uuid.New()

	uut := provisionerdserver.NewAcquirer(ctx, logger.Named("acquirer"), fs, ps)

	// queue up 2 responses --- we may not need both, since acquiree0 will
	// usually cancel before calling, but cancel is async, so it might call.
	for i := 0; i < 2; i++ {
		err := fs.sendCtx(ctx, database.ProvisionerJob{ID: jobID}, nil)
		require.NoError(t, err)
	}

	ctx0, cancel0 := context.WithCancel(ctx)
	cancel0()
	acquiree0.startAcquire(ctx0, uut)
	acquiree1.startAcquire(ctx, uut)
	job := acquiree1.success(ctx)
	require.Equal(t, jobID, job.ID)
}

func postJob(t *testing.T, ps pubsub.Pubsub, pt database.ProvisionerType, tags provisionerdserver.Tags) {
	t.Helper()
	msg, err := json.Marshal(provisionerdserver.JobPosting{
		ProvisionerType: pt,
		Tags:            tags,
	})
	require.NoError(t, err)
	err = ps.Publish(provisionerdserver.EventJobPosted, msg)
	require.NoError(t, err)
}

// fakeOrderedStore is a fake store that lets tests send AcquireProvisionerJob
// results in order over a channel, and tests for overlapped calls.
type fakeOrderedStore struct {
	jobs   chan database.ProvisionerJob
	errors chan error

	mu     sync.Mutex
	params []database.AcquireProvisionerJobParams

	// inflight and overlaps track whether any calls from workers overlap with
	// one another
	inflight map[uuid.UUID]bool
	overlaps [][]uuid.UUID
}

func newFakeOrderedStore() *fakeOrderedStore {
	return &fakeOrderedStore{
		// buffer the channels so that we can queue up lots of responses to
		// occur nearly simultaneously
		jobs:     make(chan database.ProvisionerJob, 100),
		errors:   make(chan error, 100),
		inflight: make(map[uuid.UUID]bool),
	}
}

func (s *fakeOrderedStore) AcquireProvisionerJob(
	_ context.Context, params database.AcquireProvisionerJobParams,
) (
	database.ProvisionerJob, error,
) {
	s.mu.Lock()
	s.params = append(s.params, params)
	for workerID := range s.inflight {
		s.overlaps = append(s.overlaps, []uuid.UUID{workerID, params.WorkerID.UUID})
	}
	s.inflight[params.WorkerID.UUID] = true
	s.mu.Unlock()

	job := <-s.jobs
	err := <-s.errors

	s.mu.Lock()
	delete(s.inflight, params.WorkerID.UUID)
	s.mu.Unlock()

	return job, err
}

func (s *fakeOrderedStore) sendCtx(ctx context.Context, job database.ProvisionerJob, err error) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case s.jobs <- job:
		// OK
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case s.errors <- err:
		// OK
	}
	return nil
}

// fakeTaggedStore is a test store that allows tests to specify which jobs are
// available, and returns them to callers with the appropriate provisioner type
// and tags. It doesn't care about the order.
type fakeTaggedStore struct {
	t      *testing.T
	mu     sync.Mutex
	jobs   []database.ProvisionerJob
	params chan database.AcquireProvisionerJobParams
}

func newFakeTaggedStore(t *testing.T) *fakeTaggedStore {
	return &fakeTaggedStore{
		t:      t,
		params: make(chan database.AcquireProvisionerJobParams, 100),
	}
}

func (s *fakeTaggedStore) AcquireProvisionerJob(
	_ context.Context, params database.AcquireProvisionerJobParams,
) (
	database.ProvisionerJob, error,
) {
	defer func() { s.params <- params }()
	var tags provisionerdserver.Tags
	err := json.Unmarshal(params.Tags, &tags)
	if !assert.NoError(s.t, err) {
		return database.ProvisionerJob{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
jobLoop:
	for i, job := range s.jobs {
		if !slices.Contains(params.Types, job.Provisioner) {
			continue
		}
		for k, v := range job.Tags {
			pv, ok := tags[k]
			if !ok {
				continue jobLoop
			}
			if v != pv {
				continue jobLoop
			}
		}
		// found a job!
		s.jobs = append(s.jobs[:i], s.jobs[i+1:]...)
		return job, nil
	}
	return database.ProvisionerJob{}, sql.ErrNoRows
}

// testAcquiree is a helper type that handles asynchronously calling AcquireJob
// and asserting whether or not it returns, blocks, or is canceled.
type testAcquiree struct {
	t        *testing.T
	workerID uuid.UUID
	pt       []database.ProvisionerType
	tags     provisionerdserver.Tags
	ec       chan error
	jc       chan database.ProvisionerJob
}

func newTestAcquiree(t *testing.T, workerID uuid.UUID, pt []database.ProvisionerType, tags provisionerdserver.Tags) *testAcquiree {
	return &testAcquiree{
		t:        t,
		workerID: workerID,
		pt:       pt,
		tags:     tags,
		ec:       make(chan error, 1),
		jc:       make(chan database.ProvisionerJob, 1),
	}
}

func (a *testAcquiree) startAcquire(ctx context.Context, uut *provisionerdserver.Acquirer) {
	go func() {
		j, e := uut.AcquireJob(ctx, a.workerID, a.pt, a.tags)
		a.ec <- e
		a.jc <- j
	}()
}

func (a *testAcquiree) success(ctx context.Context) database.ProvisionerJob {
	select {
	case <-ctx.Done():
		a.t.Fatal("timeout waiting for AcquireJob error")
	case err := <-a.ec:
		require.NoError(a.t, err)
	}
	select {
	case <-ctx.Done():
		a.t.Fatal("timeout waiting for AcquireJob job")
	case job := <-a.jc:
		return job
	}
	// unhittable
	return database.ProvisionerJob{}
}

func (a *testAcquiree) requireBlocked() {
	select {
	case <-a.ec:
		a.t.Fatal("AcquireJob should block")
	default:
		// OK
	}
}

func (a *testAcquiree) requireCanceled(ctx context.Context) {
	select {
	case err := <-a.ec:
		require.ErrorIs(a.t, err, context.Canceled)
	case <-ctx.Done():
		a.t.Fatal("timed out waiting for AcquireJob")
	}
	select {
	case job := <-a.jc:
		require.Equal(a.t, uuid.Nil, job.ID)
	case <-ctx.Done():
		a.t.Fatal("timed out waiting for AcquireJob")
	}
}
