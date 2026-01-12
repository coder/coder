package dbtestutil

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/prometheus/client_golang/prometheus"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmetrics"
)

// wrapWithQueryTracking wraps the store with dbmetrics and sets up query
// tracking with optional reset capability.
func wrapWithQueryTracking(t testing.TB, db database.Store, logger slog.Logger, resetCh chan struct{}) (database.Store, func()) {
	reg := prometheus.NewRegistry()
	db = dbmetrics.NewQueryMetrics(db, logger, reg)

	// Use a mutex to safely access baselineCounts from both the reset
	// goroutine and the cleanup function.
	var mu sync.Mutex
	var baselineCounts map[string]uint64
	done := make(chan struct{})

	// If resetCh provided, listen for resets in a goroutine.
	if resetCh != nil {
		go func() {
			defer close(done)
			for range resetCh {
				mu.Lock()
				baselineCounts = getQueryCounts(reg)
				mu.Unlock()
			}
		}()
	} else {
		close(done)
	}

	cleanup := func() {
		// Close the channel to stop the goroutine.
		if resetCh != nil {
			close(resetCh)
		}
		<-done

		finalCounts := getQueryCounts(reg)

		mu.Lock()
		reportCounts := diffCounts(baselineCounts, finalCounts)
		mu.Unlock()

		if len(reportCounts) == 0 {
			return
		}

		writeQueryReport(t, reportCounts)
	}

	return db, cleanup
}

// getQueryCounts extracts per-query call counts from the registry.
func getQueryCounts(reg prometheus.Gatherer) map[string]uint64 {
	metrics, err := reg.Gather()
	if err != nil {
		return nil
	}

	counts := make(map[string]uint64)
	for _, m := range metrics {
		if m.GetName() != "coderd_db_query_latencies_seconds" {
			continue
		}
		for _, metric := range m.GetMetric() {
			for _, label := range metric.GetLabel() {
				if label.GetName() == "query" {
					counts[label.GetValue()] = metric.GetHistogram().GetSampleCount()
				}
			}
		}
	}
	return counts
}

// diffCounts returns counts from 'after' minus counts from 'before'.
// If before is nil, returns after unchanged.
func diffCounts(before, after map[string]uint64) map[string]uint64 {
	if before == nil {
		return after
	}
	result := make(map[string]uint64)
	for query, count := range after {
		diff := count - before[query]
		if diff > 0 {
			result[query] = diff
		}
	}
	return result
}

// writeQueryReport writes a TSV report of query counts.
// If DBTRACKER_REPORT_DIR is set, writes to that directory; otherwise writes
// to the test's package directory.
func writeQueryReport(t testing.TB, counts map[string]uint64) {
	testName := t.Name()
	sanitizedName := regexp.MustCompile("[^a-zA-Z0-9-_]+").ReplaceAllString(testName, "_")

	// Determine output directory.
	outDir := os.Getenv("DBTRACKER_REPORT_DIR")
	if outDir == "" {
		var err error
		outDir, err = filepath.Abs(".")
		if err != nil {
			t.Logf("query tracking: failed to get working directory: %v", err)
			return
		}
	}

	// Sort by count descending.
	type row struct {
		Query string
		Count uint64
	}
	rows := make([]row, 0, len(counts))
	for query, count := range counts {
		rows = append(rows, row{query, count})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Count != rows[j].Count {
			return rows[i].Count > rows[j].Count
		}
		return rows[i].Query < rows[j].Query
	})

	var buf strings.Builder
	_, _ = buf.WriteString("Count\tQuery\tTestName\n")
	for _, r := range rows {
		_, _ = fmt.Fprintf(&buf, "%d\t%s\t%s\n", r.Count, r.Query, testName)
	}

	filename := filepath.Join(outDir, sanitizedName+".querytracking.tsv")

	if err := os.WriteFile(filename, []byte(buf.String()), 0o600); err != nil {
		t.Logf("query tracking: failed to write report: %v", err)
		return
	}

	t.Logf("query tracking: wrote %d queries to %s", len(rows), filename)
	t.Logf("query tracking: merge all reports with: (head -1 %s/*.tsv | head -1; tail -q -n +2 %s/*.tsv) | sort -t$'\\t' -k1 -rn", outDir, outDir)
}
