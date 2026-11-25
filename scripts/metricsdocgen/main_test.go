package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadAndMergeMetrics(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	generatedPath := filepath.Join(dir, "generated_metrics")
	staticPath := filepath.Join(dir, "metrics")

	if err := os.WriteFile(generatedPath, []byte(`# HELP requests_total Generated help.
# TYPE requests_total counter
requests_total{generated=""} 0
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(staticPath, []byte(`# HELP requests_total Static help.
# TYPE requests_total gauge
requests_total{generated="",runtime=""} 0
# HELP go_goroutines Goroutines that currently exist.
# TYPE go_goroutines gauge
go_goroutines 0
`), 0o600); err != nil {
		t.Fatal(err)
	}

	metrics, err := readAndMergeMetrics(generatedPath, staticPath)
	if err != nil {
		t.Fatalf("readAndMergeMetrics: %v", err)
	}
	if len(metrics) != 2 {
		t.Fatalf("got %d metrics, want 2", len(metrics))
	}

	metric := metrics[1]
	if got, want := metric.GetHelp(), "Generated help."; got != want {
		t.Errorf("HELP = %q, want %q", got, want)
	}
	if got, want := metric.GetType().String(), "COUNTER"; got != want {
		t.Errorf("TYPE = %q, want %q", got, want)
	}
	labels := map[string]bool{}
	for _, label := range metric.Metric[0].Label {
		labels[label.GetName()] = true
	}
	for _, label := range []string{"generated", "runtime"} {
		if !labels[label] {
			t.Errorf("labels = %v, missing %q", labels, label)
		}
	}

	staticOnly := metrics[0]
	if got, want := staticOnly.GetName(), "go_goroutines"; got != want {
		t.Errorf("static-only metric name = %q, want %q", got, want)
	}
	if got, want := staticOnly.GetHelp(), "Goroutines that currently exist."; got != want {
		t.Errorf("static-only HELP = %q, want %q", got, want)
	}
}
