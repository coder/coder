package lab_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/coder/coder/examples/mcp-apps/pprof-lab/lab"
)

// testTimeout bounds every poll in this file.
const testTimeout = 10 * time.Second

// waitFor polls cond until it returns true or testTimeout passes.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), testTimeout)
	defer cancel()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for !cond() {
		select {
		case <-ctx.Done():
			t.Fatalf("timed out waiting for %s", what)
		case <-ticker.C:
		}
	}
}

// stateInt reads an integer-valued entry from a State map.
func stateInt(t *testing.T, state map[string]any, key string) int64 {
	t.Helper()
	switch v := state[key].(type) {
	case int:
		return int64(v)
	case int64:
		return v
	default:
		t.Fatalf("state[%q] = %v (%T), want integer", key, state[key], state[key])
		return 0
	}
}

func isClosed(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

func TestScenarioLifecycle(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		knobs    map[string]any
		progress string
	}{
		{name: "goroutine-leak", knobs: map[string]any{"count": 8}, progress: "parked"},
		{name: "heap-growth", knobs: map[string]any{"mib_per_second": 64, "cap_mib": 1}, progress: "records"},
		{name: "alloc-churn", knobs: map[string]any{"workers": 1}, progress: "allocations"},
		{name: "cpu-burn", knobs: map[string]any{"workers": 1}, progress: "iterations"},
		{name: "mutex-contention", knobs: map[string]any{"goroutines": 2}, progress: "acquisitions"},
		{name: "block-wait", knobs: map[string]any{"goroutines": 2}, progress: "wakeups"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			reg := lab.NewRegistry()
			s, ok := reg.Get(tc.name)
			if !ok {
				t.Fatalf("scenario %q not registered", tc.name)
			}
			if s.Description() == "" || len(s.Knobs()) == 0 {
				t.Fatal("scenario must have a description and knobs")
			}
			if s.Running() {
				t.Fatal("scenario running before Start")
			}
			t.Cleanup(s.Stop)

			if err := s.Start(t.Context(), tc.knobs); err != nil {
				t.Fatalf("Start: %v", err)
			}
			done := lab.Done(s)
			if done == nil {
				t.Fatal("Done returned nil after Start")
			}
			waitFor(t, tc.progress, func() bool {
				return stateInt(t, s.State(), tc.progress) > 0
			})
			if !s.Running() {
				t.Fatal("scenario not running after Start")
			}

			s.Stop()
			if !isClosed(done) {
				t.Fatal("run goroutines still alive after Stop")
			}
			if s.Running() {
				t.Fatal("scenario running after Stop")
			}
			if lab.Done(s) != nil {
				t.Fatal("Done should be nil after Stop")
			}
		})
	}
}

func TestStartRestartsWithNewKnobs(t *testing.T) {
	t.Parallel()
	s, _ := lab.NewRegistry().Get("goroutine-leak")
	t.Cleanup(s.Stop)

	if err := s.Start(t.Context(), map[string]any{"count": 3}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	first := lab.Done(s)
	waitFor(t, "3 parked", func() bool { return stateInt(t, s.State(), "parked") == 3 })

	if err := s.Start(t.Context(), map[string]any{"count": 5}); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	if !isClosed(first) {
		t.Fatal("first run still alive after restart")
	}
	waitFor(t, "5 parked", func() bool { return stateInt(t, s.State(), "parked") == 5 })
	if got := stateInt(t, s.State(), "goroutines"); got != 5 {
		t.Fatalf("goroutines = %d, want 5", got)
	}

	s.Stop()
	if got := stateInt(t, s.State(), "parked"); got != 0 {
		t.Fatalf("parked = %d after Stop, want 0", got)
	}
}

func TestHeapGrowthHonoursCap(t *testing.T) {
	t.Parallel()
	s, _ := lab.NewRegistry().Get("heap-growth")
	t.Cleanup(s.Stop)

	err := s.Start(t.Context(), map[string]any{"mib_per_second": 64, "cap_mib": 2})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	done := lab.Done(s)
	waitFor(t, "capped", func() bool {
		capped, _ := s.State()["capped"].(bool)
		return capped
	})
	// The grower exits at the cap while the records stay retained.
	waitFor(t, "grower exit", func() bool { return isClosed(done) })
	state := s.State()
	if got := stateInt(t, state, "retained_mib"); got != 2 {
		t.Fatalf("retained_mib = %d, want 2", got)
	}
	if got := stateInt(t, state, "records"); got != 2048 {
		t.Fatalf("records = %d, want 2048", got)
	}
	if !s.Running() {
		t.Fatal("heap-growth should report running while records are retained")
	}

	s.Stop()
	state = s.State()
	if got := stateInt(t, state, "records"); got != 0 {
		t.Fatalf("records = %d after Stop, want 0", got)
	}
	if capped, _ := state["capped"].(bool); capped {
		t.Fatal("capped should reset on Stop")
	}
	if stateInt(t, state, "cap_mib") != 0 || stateInt(t, state, "mib_per_second") != 0 {
		t.Fatalf("knobs should reset on Stop, got %v", state)
	}
	if s.Running() {
		t.Fatal("heap-growth running after Stop")
	}
}

func TestClampKnob(t *testing.T) {
	t.Parallel()
	spec := lab.Knob{Name: "count", Default: 5, Min: 1, Max: 10}

	cases := []struct {
		name    string
		knobs   map[string]any
		want    int
		wantErr bool
	}{
		{name: "missing uses default", knobs: map[string]any{}, want: 5},
		{name: "nil uses default", knobs: map[string]any{"count": nil}, want: 5},
		{name: "int in range", knobs: map[string]any{"count": 7}, want: 7},
		{name: "json float in range", knobs: map[string]any{"count": float64(10)}, want: 10},
		{name: "json number in range", knobs: map[string]any{"count": json.Number("1")}, want: 1},
		{name: "below min", knobs: map[string]any{"count": 0}, wantErr: true},
		{name: "above max", knobs: map[string]any{"count": 11}, wantErr: true},
		{name: "fractional", knobs: map[string]any{"count": 2.5}, wantErr: true},
		{name: "string", knobs: map[string]any{"count": "3"}, wantErr: true},
		{name: "bool", knobs: map[string]any{"count": true}, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := lab.ClampKnob(tc.knobs, spec)
			if tc.wantErr {
				var kerr *lab.KnobError
				if !errors.As(err, &kerr) {
					t.Fatalf("err = %v, want *lab.KnobError", err)
				}
				if kerr.Knob != "count" {
					t.Fatalf("KnobError.Knob = %q, want count", kerr.Knob)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %d, want %d", got, tc.want)
			}
		})
	}
}

func TestParseKnobsRejectsUnknown(t *testing.T) {
	t.Parallel()
	specs := []lab.Knob{{Name: "count", Default: 5, Min: 1, Max: 10}}
	_, err := lab.ParseKnobs(map[string]any{"count": 2, "bogus": 1}, specs)
	var kerr *lab.KnobError
	if !errors.As(err, &kerr) || kerr.Knob != "bogus" {
		t.Fatalf("err = %v, want KnobError for bogus", err)
	}
}

func TestScenarioKnobBounds(t *testing.T) {
	t.Parallel()
	for _, s := range lab.NewRegistry().All() {
		t.Run(s.Name(), func(t *testing.T) {
			t.Parallel()
			for _, k := range s.Knobs() {
				if k.Min > k.Default || k.Default > k.Max {
					t.Fatalf("knob %s: default %d outside [%d, %d]", k.Name, k.Default, k.Min, k.Max)
				}
				fresh, _ := lab.NewRegistry().Get(s.Name())
				t.Cleanup(fresh.Stop)
				err := fresh.Start(t.Context(), map[string]any{k.Name: k.Max + 1})
				var kerr *lab.KnobError
				if !errors.As(err, &kerr) {
					t.Fatalf("knob %s: Start(max+1) err = %v, want KnobError", k.Name, err)
				}
				if fresh.Running() {
					t.Fatalf("knob %s: scenario running after rejected Start", k.Name)
				}
			}
		})
	}
}

func TestRegistry(t *testing.T) {
	t.Parallel()
	reg := lab.NewRegistry()
	want := []string{"goroutine-leak", "heap-growth", "alloc-churn", "cpu-burn", "mutex-contention", "block-wait"}
	all := reg.All()
	if len(all) != len(want) {
		t.Fatalf("len(All()) = %d, want %d", len(all), len(want))
	}
	for i, s := range all {
		if s.Name() != want[i] {
			t.Fatalf("All()[%d] = %q, want %q", i, s.Name(), want[i])
		}
	}
	if _, ok := reg.Get("nope"); ok {
		t.Fatal("Get(nope) should fail")
	}

	leak, _ := reg.Get("goroutine-leak")
	t.Cleanup(reg.StopAll)
	if err := leak.Start(t.Context(), map[string]any{"count": 2}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	done := lab.Done(leak)
	reg.StopAll()
	if !isClosed(done) || leak.Running() {
		t.Fatal("StopAll did not stop goroutine-leak")
	}
}
