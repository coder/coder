package workspaceapps_test
import (
	"errors"
	"context"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/workspaceapps"
	"github.com/coder/coder/v2/testutil"
)
type fakeReporter struct {
	mu   sync.Mutex
	s    []workspaceapps.StatsReport
	err  error
	errN int
}
func (r *fakeReporter) stats() []workspaceapps.StatsReport {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.s
}
func (r *fakeReporter) errors() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.errN
}
func (r *fakeReporter) setError(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.err = err
}
func (r *fakeReporter) ReportAppStats(_ context.Context, stats []workspaceapps.StatsReport) error {
	r.mu.Lock()
	if r.err != nil {
		r.errN++
		r.mu.Unlock()
		return r.err
	}
	r.s = append(r.s, stats...)
	r.mu.Unlock()
	return nil
}
func TestStatsCollector(t *testing.T) {
	t.Parallel()
	rollupUUID := uuid.New()
	rollupUUID2 := uuid.New()
	someUUID := uuid.New()
	rollupWindow := time.Minute
	start := dbtime.Now().Truncate(time.Minute).UTC()
	end := start.Add(10 * time.Second)
	tests := []struct {
		name           string
		flushIncrement time.Duration
		flushCount     int
		stats          []workspaceapps.StatsReport
		want           []workspaceapps.StatsReport
	}{
		{
			name:           "Single stat rolled up and reported once",
			flushIncrement: 2*rollupWindow + time.Second,
			flushCount:     10, // Only reported once.
			stats: []workspaceapps.StatsReport{
				{
					SessionID:        rollupUUID,
					SessionStartedAt: start,
					SessionEndedAt:   end,
					Requests:         1,
				},
			},
			want: []workspaceapps.StatsReport{
				{
					SessionID:        rollupUUID,
					SessionStartedAt: start,
					SessionEndedAt:   start.Add(rollupWindow),
					Requests:         1,
				},
			},
		},
		{
			name:           "Two unique stat rolled up",
			flushIncrement: 2*rollupWindow + time.Second,
			flushCount:     10, // Only reported once.
			stats: []workspaceapps.StatsReport{
				{
					AccessMethod:     workspaceapps.AccessMethodPath,
					SlugOrPort:       "code-server",
					SessionID:        rollupUUID,
					SessionStartedAt: start,
					SessionEndedAt:   end,
					Requests:         1,
				},
				{
					AccessMethod:     workspaceapps.AccessMethodTerminal,
					SessionID:        rollupUUID2,
					SessionStartedAt: start,
					SessionEndedAt:   end,
					Requests:         1,
				},
			},
			want: []workspaceapps.StatsReport{
				{
					AccessMethod:     workspaceapps.AccessMethodPath,
					SlugOrPort:       "code-server",
					SessionID:        rollupUUID,
					SessionStartedAt: start,
					SessionEndedAt:   start.Add(rollupWindow),
					Requests:         1,
				},
				{
					AccessMethod:     workspaceapps.AccessMethodTerminal,
					SessionID:        rollupUUID2,
					SessionStartedAt: start,
					SessionEndedAt:   start.Add(rollupWindow),
					Requests:         1,
				},
			},
		},
		{
			name:           "Multiple stats rolled up",
			flushIncrement: 2*rollupWindow + time.Second,
			flushCount:     2,
			stats: []workspaceapps.StatsReport{
				{
					SessionID:        rollupUUID,
					SessionStartedAt: start,
					SessionEndedAt:   end,
					Requests:         1,
				},
				{
					SessionID:        uuid.New(),
					SessionStartedAt: start,
					SessionEndedAt:   end,
					Requests:         1,
				},
			},
			want: []workspaceapps.StatsReport{
				{
					SessionID:        rollupUUID,
					SessionStartedAt: start,
					SessionEndedAt:   start.Add(rollupWindow),
					Requests:         2,
				},
			},
		},
		{
			name:           "Long sessions not rolled up but reported multiple times",
			flushIncrement: rollupWindow + time.Second,
			flushCount:     4,
			stats: []workspaceapps.StatsReport{
				{
					SessionID:        rollupUUID,
					SessionStartedAt: start,
					Requests:         1,
				},
			},
			want: []workspaceapps.StatsReport{
				{
					SessionID:        rollupUUID,
					SessionStartedAt: start,
					SessionEndedAt:   start.Add(rollupWindow + time.Second),
					Requests:         1,
				},
				{
					SessionID:        rollupUUID,
					SessionStartedAt: start,
					SessionEndedAt:   start.Add(2 * (rollupWindow + time.Second)),
					Requests:         1,
				},
				{
					SessionID:        rollupUUID,
					SessionStartedAt: start,
					SessionEndedAt:   start.Add(3 * (rollupWindow + time.Second)),
					Requests:         1,
				},
				{
					SessionID:        rollupUUID,
					SessionStartedAt: start,
					SessionEndedAt:   start.Add(4 * (rollupWindow + time.Second)),
					Requests:         1,
				},
			},
		},
		{
			name:           "Incomplete stats not reported until it exceeds rollup window",
			flushIncrement: rollupWindow / 4,
			flushCount:     6,
			stats: []workspaceapps.StatsReport{
				{
					SessionID:        someUUID,
					SessionStartedAt: start,
					Requests:         1,
				},
			},
			want: []workspaceapps.StatsReport{
				{
					SessionID:        someUUID,
					SessionStartedAt: start,
					SessionEndedAt:   start.Add(rollupWindow / 4 * 5),
					Requests:         1,
				},
				{
					SessionID:        someUUID,
					SessionStartedAt: start,
					SessionEndedAt:   start.Add(rollupWindow / 4 * 6),
					Requests:         1,
				},
			},
		},
		{
			name:           "Same stat reported without and with end time and rolled up",
			flushIncrement: rollupWindow + time.Second,
			flushCount:     1,
			stats: []workspaceapps.StatsReport{
				{
					SessionID:        someUUID,
					SessionStartedAt: start,
					Requests:         1,
				},
				{
					SessionID:        someUUID,
					SessionStartedAt: start,
					SessionEndedAt:   start.Add(10 * time.Second),
					Requests:         1,
				},
			},
			want: []workspaceapps.StatsReport{
				{
					SessionID:        someUUID,
					SessionStartedAt: start,
					SessionEndedAt:   start.Add(rollupWindow),
					Requests:         1,
				},
			},
		},
		{
			name:           "Same non-rolled up stat reported without and with end time",
			flushIncrement: rollupWindow * 2,
			flushCount:     1,
			stats: []workspaceapps.StatsReport{
				{
					SessionID:        someUUID,
					SessionStartedAt: start,
					Requests:         1,
				},
				{
					SessionID:        someUUID,
					SessionStartedAt: start,
					SessionEndedAt:   start.Add(rollupWindow * 2),
					Requests:         1,
				},
			},
			want: []workspaceapps.StatsReport{
				{
					SessionID:        someUUID,
					SessionStartedAt: start,
					SessionEndedAt:   start.Add(rollupWindow * 2),
					Requests:         1,
				},
			},
		},
	}
	// Run tests.
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			flush := make(chan chan<- struct{}, 1)
			var now atomic.Pointer[time.Time]
			now.Store(&start)
			reporter := &fakeReporter{}
			collector := workspaceapps.NewStatsCollector(workspaceapps.StatsCollectorOptions{
				Reporter:       reporter,
				ReportInterval: time.Hour,
				RollupWindow:   rollupWindow,
				Flush: flush,
				Now:   func() time.Time { return *now.Load() },
			})
			// Collect reports.
			for _, report := range tt.stats {
				collector.Collect(report)
			}
			// Advance time.
			flushTime := start.Add(tt.flushIncrement)
			for i := 0; i < tt.flushCount; i++ {
				now.Store(&flushTime)
				flushDone := make(chan struct{}, 1)
				flush <- flushDone
				<-flushDone
				flushTime = flushTime.Add(tt.flushIncrement)
			}
			var gotStats []workspaceapps.StatsReport
			require.Eventually(t, func() bool {
				gotStats = reporter.stats()
				return len(gotStats) == len(tt.want)
			}, testutil.WaitMedium, testutil.IntervalFast)
			// Order is not guaranteed.
			sortBySessionID := func(a, b workspaceapps.StatsReport) int {
				if a.SessionID == b.SessionID {
					return int(a.SessionEndedAt.Sub(b.SessionEndedAt))
				}
				if a.SessionID.String() < b.SessionID.String() {
					return -1
				}
				return 1
			}
			slices.SortFunc(tt.want, sortBySessionID)
			slices.SortFunc(gotStats, sortBySessionID)
			// Verify reported stats.
			for i, got := range gotStats {
				want := tt.want[i]
				assert.Equal(t, want.SessionID, got.SessionID, "session ID; i = %d", i)
				assert.Equal(t, want.SessionStartedAt, got.SessionStartedAt, "session started at; i = %d", i)
				assert.Equal(t, want.SessionEndedAt, got.SessionEndedAt, "session ended at; i = %d", i)
				assert.Equal(t, want.Requests, got.Requests, "requests; i = %d", i)
			}
		})
	}
}
func TestStatsCollector_backlog(t *testing.T) {
	t.Parallel()
	rollupWindow := time.Minute
	flush := make(chan chan<- struct{}, 1)
	start := dbtime.Now().Truncate(time.Minute).UTC()
	var now atomic.Pointer[time.Time]
	now.Store(&start)
	reporter := &fakeReporter{}
	collector := workspaceapps.NewStatsCollector(workspaceapps.StatsCollectorOptions{
		Reporter:       reporter,
		ReportInterval: time.Hour,
		RollupWindow:   rollupWindow,
		Flush: flush,
		Now:   func() time.Time { return *now.Load() },
	})
	reporter.setError(errors.New("some error"))
	// The first collected stat is "rolled up" and moved into the
	// backlog during the first flush. On the second flush nothing is
	// rolled up due to being unable to report the backlog.
	for i := 0; i < 2; i++ {
		collector.Collect(workspaceapps.StatsReport{
			SessionID:        uuid.New(),
			SessionStartedAt: start,
			SessionEndedAt:   start.Add(10 * time.Second),
			Requests:         1,
		})
		start = start.Add(time.Minute)
		now.Store(&start)
		flushDone := make(chan struct{}, 1)
		flush <- flushDone
		<-flushDone
	}
	// Flush was performed 2 times, 2 reports should have failed.
	wantErrors := 2
	assert.Equal(t, wantErrors, reporter.errors())
	assert.Empty(t, reporter.stats())
	reporter.setError(nil)
	// Flush again, this time the backlog should be reported in addition
	// to the second collected stat being rolled up and reported.
	flushDone := make(chan struct{}, 1)
	flush <- flushDone
	<-flushDone
	assert.Equal(t, wantErrors, reporter.errors())
	assert.Len(t, reporter.stats(), 2)
}
func TestStatsCollector_Close(t *testing.T) {
	t.Parallel()
	reporter := &fakeReporter{}
	collector := workspaceapps.NewStatsCollector(workspaceapps.StatsCollectorOptions{
		Reporter:       reporter,
		ReportInterval: time.Hour,
		RollupWindow:   time.Minute,
	})
	collector.Collect(workspaceapps.StatsReport{
		SessionID:        uuid.New(),
		SessionStartedAt: dbtime.Now(),
		SessionEndedAt:   dbtime.Now(),
		Requests:         1,
	})
	collector.Close()
	// Verify that stats are reported after close.
	assert.NotEmpty(t, reporter.stats())
}
