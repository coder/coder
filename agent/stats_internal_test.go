package agent

import (
	"context"
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
	"google.golang.org/protobuf/types/known/durationpb"
	"tailscale.com/types/ipproto"
	"tailscale.com/types/netlogtype"

	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/testutil"
)

func TestStatsReporter(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := testutil.Logger(t)
	fSource := newFakeNetworkStatsSource(ctx, t)
	fCollector := newFakeCollector(t)
	fDest := newFakeStatsDest()
	uut := newStatsReporter(logger, fSource, fCollector)

	loopErr := make(chan error, 1)
	loopCtx, loopCancel := context.WithCancel(ctx)
	go func() {
		err := uut.reportLoop(loopCtx, fDest)
		loopErr <- err
	}()

	// initial request to get duration
	req := testutil.TryReceive(ctx, t, fDest.reqs)
	require.NotNil(t, req)
	require.Nil(t, req.Stats)
	interval := time.Second * 34
	testutil.RequireSend(ctx, t, fDest.resps, &proto.UpdateStatsResponse{ReportInterval: durationpb.New(interval)})

	// call to source to set the callback and interval
	gotInterval := testutil.TryReceive(ctx, t, fSource.period)
	require.Equal(t, interval, gotInterval)

	// callback returning netstats
	netStats := map[netlogtype.Connection]netlogtype.Counts{
		{
			Proto: ipproto.TCP,
			Src:   netip.MustParseAddrPort("192.168.1.33:4887"),
			Dst:   netip.MustParseAddrPort("192.168.2.99:9999"),
		}: {
			TxPackets: 22,
			TxBytes:   23,
			RxPackets: 24,
			RxBytes:   25,
		},
	}
	fSource.callback(time.Now(), time.Now(), netStats, nil)

	// collector called to complete the stats
	gotNetStats := testutil.TryReceive(ctx, t, fCollector.calls)
	require.Equal(t, netStats, gotNetStats)

	// while we are collecting the stats, send in two new netStats to simulate
	// what happens if we don't keep up.  The stats should be accumulated.
	netStats0 := map[netlogtype.Connection]netlogtype.Counts{
		{
			Proto: ipproto.TCP,
			Src:   netip.MustParseAddrPort("192.168.1.33:4887"),
			Dst:   netip.MustParseAddrPort("192.168.2.99:9999"),
		}: {
			TxPackets: 10,
			TxBytes:   10,
			RxPackets: 10,
			RxBytes:   10,
		},
	}
	fSource.callback(time.Now(), time.Now(), netStats0, nil)
	netStats1 := map[netlogtype.Connection]netlogtype.Counts{
		{
			Proto: ipproto.TCP,
			Src:   netip.MustParseAddrPort("192.168.1.33:4887"),
			Dst:   netip.MustParseAddrPort("192.168.2.99:9999"),
		}: {
			TxPackets: 11,
			TxBytes:   11,
			RxPackets: 11,
			RxBytes:   11,
		},
	}
	fSource.callback(time.Now(), time.Now(), netStats1, nil)

	// complete first collection
	stats := &proto.Stats{SessionCountJetbrains: 55}
	testutil.RequireSend(ctx, t, fCollector.stats, stats)

	// destination called to report the first stats
	update := testutil.TryReceive(ctx, t, fDest.reqs)
	require.NotNil(t, update)
	require.Equal(t, stats, update.Stats)
	testutil.RequireSend(ctx, t, fDest.resps, &proto.UpdateStatsResponse{ReportInterval: durationpb.New(interval)})

	// second update -- netStat0 and netStats1 are accumulated and reported
	wantNetStats := map[netlogtype.Connection]netlogtype.Counts{
		{
			Proto: ipproto.TCP,
			Src:   netip.MustParseAddrPort("192.168.1.33:4887"),
			Dst:   netip.MustParseAddrPort("192.168.2.99:9999"),
		}: {
			TxPackets: 21,
			TxBytes:   21,
			RxPackets: 21,
			RxBytes:   21,
		},
	}
	gotNetStats = testutil.TryReceive(ctx, t, fCollector.calls)
	require.Equal(t, wantNetStats, gotNetStats)
	stats = &proto.Stats{SessionCountJetbrains: 66}
	testutil.RequireSend(ctx, t, fCollector.stats, stats)
	update = testutil.TryReceive(ctx, t, fDest.reqs)
	require.NotNil(t, update)
	require.Equal(t, stats, update.Stats)
	interval2 := 27 * time.Second
	testutil.RequireSend(ctx, t, fDest.resps, &proto.UpdateStatsResponse{ReportInterval: durationpb.New(interval2)})

	// set the new interval
	gotInterval = testutil.TryReceive(ctx, t, fSource.period)
	require.Equal(t, interval2, gotInterval)

	loopCancel()
	err := testutil.TryReceive(ctx, t, loopErr)
	require.NoError(t, err)
}

// TestStatsReporter_NonFatalRPCError verifies that a transient RPC
// error from UpdateStats does not cause reportLoop to exit. The
// loop should log the error and continue reporting on the next
// callback. Regression test for #22864.
func TestStatsReporter_NonFatalRPCError(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := testutil.Logger(t)
	fSource := newFakeNetworkStatsSource(ctx, t)
	fCollector := newFakeCollector(t)
	fDest := newFakeErrStatsDest()
	uut := newStatsReporter(logger, fSource, fCollector)

	loopErr := make(chan error, 1)
	loopCtx, loopCancel := context.WithCancel(ctx)
	go func() {
		loopErr <- uut.reportLoop(loopCtx, fDest)
	}()

	// Initial request to get the reporting interval.
	req := testutil.TryReceive(ctx, t, fDest.reqs)
	require.NotNil(t, req)
	require.Nil(t, req.Stats)
	interval := 34 * time.Second
	testutil.RequireSend(ctx, t, fDest.resps,
		&proto.UpdateStatsResponse{
			ReportInterval: durationpb.New(interval),
		})

	// Source configured with callback and interval.
	gotInterval := testutil.TryReceive(ctx, t, fSource.period)
	require.Equal(t, interval, gotInterval)

	// First callback delivers network stats.
	netStats := map[netlogtype.Connection]netlogtype.Counts{
		{
			Proto: ipproto.TCP,
			Src:   netip.MustParseAddrPort("10.0.0.1:1234"),
			Dst:   netip.MustParseAddrPort("10.0.0.2:5678"),
		}: {TxPackets: 1, TxBytes: 2, RxPackets: 3, RxBytes: 4},
	}
	fSource.callback(time.Now(), time.Now(), netStats, nil)

	// Collector is called; supply stats.
	gotNetStats := testutil.TryReceive(ctx, t, fCollector.calls)
	require.Equal(t, netStats, gotNetStats)
	stats := &proto.Stats{SessionCountJetbrains: 10}
	testutil.RequireSend(ctx, t, fCollector.stats, stats)

	// Destination receives the request, but we return an error.
	update := testutil.TryReceive(ctx, t, fDest.reqs)
	require.NotNil(t, update)
	require.Equal(t, stats, update.Stats)
	testutil.RequireSend(ctx, t, fDest.errs,
		xerrors.New("transient RPC failure"))

	// The loop must NOT have exited. Send another callback and
	// verify it is reported successfully.
	netStats2 := map[netlogtype.Connection]netlogtype.Counts{
		{
			Proto: ipproto.TCP,
			Src:   netip.MustParseAddrPort("10.0.0.1:1234"),
			Dst:   netip.MustParseAddrPort("10.0.0.2:5678"),
		}: {TxPackets: 5, TxBytes: 6, RxPackets: 7, RxBytes: 8},
	}
	fSource.callback(time.Now(), time.Now(), netStats2, nil)

	gotNetStats2 := testutil.TryReceive(ctx, t, fCollector.calls)
	require.Equal(t, netStats2, gotNetStats2)
	stats2 := &proto.Stats{SessionCountJetbrains: 20}
	testutil.RequireSend(ctx, t, fCollector.stats, stats2)

	update2 := testutil.TryReceive(ctx, t, fDest.reqs)
	require.NotNil(t, update2)
	require.Equal(t, stats2, update2.Stats)
	testutil.RequireSend(ctx, t, fDest.resps,
		&proto.UpdateStatsResponse{
			ReportInterval: durationpb.New(interval),
		})

	// Clean shutdown — loop exits without error.
	loopCancel()
	err := testutil.TryReceive(ctx, t, loopErr)
	require.NoError(t, err)
}

type fakeNetworkStatsSource struct {
	sync.Mutex
	ctx      context.Context
	t        testing.TB
	callback func(start, end time.Time, virtual, physical map[netlogtype.Connection]netlogtype.Counts)
	period   chan time.Duration
}

func (f *fakeNetworkStatsSource) SetConnStatsCallback(maxPeriod time.Duration, _ int, dump func(start time.Time, end time.Time, virtual map[netlogtype.Connection]netlogtype.Counts, physical map[netlogtype.Connection]netlogtype.Counts)) {
	f.Lock()
	defer f.Unlock()
	f.callback = dump
	select {
	case <-f.ctx.Done():
		f.t.Error("timeout")
	case f.period <- maxPeriod:
		// OK
	}
}

func newFakeNetworkStatsSource(ctx context.Context, t testing.TB) *fakeNetworkStatsSource {
	f := &fakeNetworkStatsSource{
		ctx:    ctx,
		t:      t,
		period: make(chan time.Duration),
	}
	return f
}

type fakeCollector struct {
	t     testing.TB
	calls chan map[netlogtype.Connection]netlogtype.Counts
	stats chan *proto.Stats
}

func (f *fakeCollector) Collect(ctx context.Context, networkStats map[netlogtype.Connection]netlogtype.Counts) *proto.Stats {
	select {
	case <-ctx.Done():
		f.t.Error("timeout on collect")
		return nil
	case f.calls <- networkStats:
		// ok
	}
	select {
	case <-ctx.Done():
		f.t.Error("timeout on collect")
		return nil
	case s := <-f.stats:
		return s
	}
}

func newFakeCollector(t testing.TB) *fakeCollector {
	return &fakeCollector{
		t:     t,
		calls: make(chan map[netlogtype.Connection]netlogtype.Counts),
		stats: make(chan *proto.Stats),
	}
}

type fakeStatsDest struct {
	reqs  chan *proto.UpdateStatsRequest
	resps chan *proto.UpdateStatsResponse
}

func (f *fakeStatsDest) UpdateStats(ctx context.Context, req *proto.UpdateStatsRequest) (*proto.UpdateStatsResponse, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case f.reqs <- req:
		// OK
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case resp := <-f.resps:
		return resp, nil
	}
}

func newFakeStatsDest() *fakeStatsDest {
	return &fakeStatsDest{
		reqs:  make(chan *proto.UpdateStatsRequest),
		resps: make(chan *proto.UpdateStatsResponse),
	}
}

// fakeErrStatsDest is like fakeStatsDest but can return errors on
// demand via the errs channel. When a value is available on errs,
// it is returned instead of waiting for resps.
type fakeErrStatsDest struct {
	reqs  chan *proto.UpdateStatsRequest
	resps chan *proto.UpdateStatsResponse
	errs  chan error
}

func (f *fakeErrStatsDest) UpdateStats(
	ctx context.Context, req *proto.UpdateStatsRequest,
) (*proto.UpdateStatsResponse, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case f.reqs <- req:
		// OK
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case err := <-f.errs:
		return nil, err
	case resp := <-f.resps:
		return resp, nil
	}
}

func newFakeErrStatsDest() *fakeErrStatsDest {
	return &fakeErrStatsDest{
		reqs:  make(chan *proto.UpdateStatsRequest),
		resps: make(chan *proto.UpdateStatsResponse),
		errs:  make(chan error),
	}
}
