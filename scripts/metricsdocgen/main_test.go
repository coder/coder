package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadAndMergeMetricsStaticLabelsOverrideGenerated(t *testing.T) {
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
requests_total{runtime=""} 0
`), 0o600); err != nil {
		t.Fatal(err)
	}

	originalGenerated, originalStatic := generatedMetricsFile, staticMetricsFile
	generatedMetricsFile, staticMetricsFile = generatedPath, staticPath
	t.Cleanup(func() {
		generatedMetricsFile, staticMetricsFile = originalGenerated, originalStatic
	})

	metrics, err := readAndMergeMetrics()
	if err != nil {
		t.Fatalf("readAndMergeMetrics: %v", err)
	}
	if len(metrics) != 1 {
		t.Fatalf("got %d metrics, want 1", len(metrics))
	}

	metric := metrics[0]
	if got, want := metric.GetHelp(), "Generated help."; got != want {
		t.Errorf("HELP = %q, want %q", got, want)
	}
	if got, want := metric.GetType().String(), "COUNTER"; got != want {
		t.Errorf("TYPE = %q, want %q", got, want)
	}
	if got, want := metric.Metric[0].Label[0].GetName(), "runtime"; got != want {
		t.Errorf("label = %q, want %q", got, want)
	}
}
