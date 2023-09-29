package dashboard_test

import (
	"context"
	"math/rand"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/scaletest/dashboard"
	"github.com/coder/coder/v2/testutil"
)

func Test_Run(t *testing.T) {
	t.Parallel()
	if testutil.RaceEnabled() {
		t.Skip("skipping timing-sensitive test because of race detector")
	}
	if runtime.GOOS == "windows" {
		t.Skip("skipping test on Windows")
	}

	successAction := func(_ context.Context) error {
		<-time.After(testutil.IntervalFast)
		return nil
	}

	failAction := func(_ context.Context) error {
		<-time.After(testutil.IntervalMedium)
		return assert.AnError
	}

	//nolint: gosec // just for testing
	rg := rand.New(rand.NewSource(0)) // deterministic for testing

	client := coderdtest.New(t, nil)
	_ = coderdtest.CreateFirstUser(t, client)

	log := slogtest.Make(t, &slogtest.Options{
		IgnoreErrors: true,
	})
	m := &testMetrics{}
	cfg := dashboard.Config{
		Interval: 500 * time.Millisecond,
		Jitter:   100 * time.Millisecond,
		Logger:   log,
		Headless: true,
		ActionFunc: func(_ context.Context, rnd func(int) int) (dashboard.Label, dashboard.Action, error) {
			if rnd(2) == 0 {
				return "fails", failAction, nil
			}
			return "succeeds", successAction, nil
		},
		RandIntn: rg.Intn,
	}
	r := dashboard.NewRunner(client, m, cfg)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	t.Cleanup(cancel)
	done := make(chan error)
	go func() {
		defer close(done)
		done <- r.Run(ctx, "", nil)
	}()
	err, ok := <-done
	assert.True(t, ok)
	require.NoError(t, err)

	for _, dur := range m.ObservedDurations["succeeds"] {
		assert.NotZero(t, dur)
	}
	for _, dur := range m.ObservedDurations["fails"] {
		assert.NotZero(t, dur)
	}
	assert.Zero(t, m.Errors["succeeds"])
	assert.NotZero(t, m.Errors["fails"])
}

type testMetrics struct {
	sync.RWMutex
	ObservedDurations map[string][]float64
	Errors            map[string]int
	Statuses          map[string]map[string]int
}

func (m *testMetrics) ObserveDuration(action string, d time.Duration) {
	m.Lock()
	defer m.Unlock()
	if m.ObservedDurations == nil {
		m.ObservedDurations = make(map[string][]float64)
	}
	m.ObservedDurations[action] = append(m.ObservedDurations[action], d.Seconds())
}

func (m *testMetrics) IncErrors(action string) {
	m.Lock()
	defer m.Unlock()
	if m.Errors == nil {
		m.Errors = make(map[string]int)
	}
	m.Errors[action]++
}

func (m *testMetrics) IncStatuses(action string, code string) {
	m.Lock()
	defer m.Unlock()
	if m.Statuses == nil {
		m.Statuses = make(map[string]map[string]int)
	}
	if m.Statuses[action] == nil {
		m.Statuses[action] = make(map[string]int)
	}
	m.Statuses[action][code]++
}
