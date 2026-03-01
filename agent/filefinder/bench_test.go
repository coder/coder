package filefinder

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
)

// Directory and file name pools for realistic tree generation.
var (
	dirNames = []string{
		"cmd", "internal", "pkg", "api", "auth", "database",
		"server", "client", "middleware", "handler", "config",
		"utils", "models", "service", "worker", "scheduler",
		"notification", "provisioner", "template", "workspace",
		"agent", "proxy", "crypto", "telemetry", "billing",
	}

	fileExts = []string{
		".go", ".ts", ".tsx", ".js", ".py", ".sql",
		".yaml", ".json", ".md", ".proto", ".sh",
	}

	fileStems = []string{
		"main", "handler", "middleware", "service", "model",
		"query", "config", "utils", "helpers", "types",
		"interface", "test", "mock", "factory", "builder",
		"adapter", "observer", "provider", "resolver", "schema",
		"migration", "fixture", "snapshot", "checkpoint",
	}
)

// generateFileTree creates n files under root in a realistic nested
// directory structure. It uses deterministic randomization (seeded
// from seed) so benchmarks are reproducible. Files are empty — only
// the paths matter for the filefinder index.
func generateFileTree(t testing.TB, root string, n int, seed int64) {
	t.Helper()

	rng := rand.New(rand.NewSource(seed)) //nolint:gosec // deterministic benchmarks

	// Pre-build a pool of directory paths at various depths.
	numDirs := n / 5
	if numDirs < 10 {
		numDirs = 10
	}

	dirs := make([]string, 0, numDirs)
	for i := 0; i < numDirs; i++ {
		depth := rng.Intn(6) + 1 // 1–6 levels deep
		parts := make([]string, depth)
		for d := 0; d < depth; d++ {
			parts[d] = dirNames[rng.Intn(len(dirNames))]
		}
		dirs = append(dirs, filepath.Join(parts...))
	}

	// Create all directories first in one pass.
	created := make(map[string]struct{})
	for _, d := range dirs {
		full := filepath.Join(root, d)
		if _, ok := created[full]; ok {
			continue
		}
		require.NoError(t, os.MkdirAll(full, 0o755))
		created[full] = struct{}{}
	}

	// Create n files distributed across the directories.
	for i := 0; i < n; i++ {
		dir := dirs[rng.Intn(len(dirs))]
		stem := fileStems[rng.Intn(len(fileStems))]
		ext := fileExts[rng.Intn(len(fileExts))]
		// Append a numeric suffix to avoid collisions.
		name := fmt.Sprintf("%s_%d%s", stem, i, ext)
		full := filepath.Join(root, dir, name)
		f, err := os.Create(full)
		require.NoError(t, err)
		f.Close()
	}
}

// buildIndex walks root and returns a populated Index, the same
// way Engine.AddRoot does but without starting a watcher.
func buildIndex(t testing.TB, root string) *Index {
	t.Helper()
	absRoot, err := filepath.Abs(root)
	require.NoError(t, err)

	idx := NewIndex()
	err = filepath.Walk(absRoot, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil //nolint:nilerr
		}
		base := filepath.Base(path)
		if _, skip := skipDirs[base]; skip && info.IsDir() {
			return filepath.SkipDir
		}
		if path == absRoot {
			return nil
		}
		relPath, relErr := filepath.Rel(absRoot, path)
		if relErr != nil {
			return nil //nolint:nilerr
		}
		relPath = filepath.ToSlash(relPath)
		var flags uint16
		if info.IsDir() {
			flags = uint16(FlagDir)
		}
		idx.Add(relPath, flags)
		return nil
	})
	require.NoError(t, err)
	return idx
}

// buildSearchableSnapshot creates an Index from root and returns
// a Snapshot usable by searchSnapshot.
func buildSearchableSnapshot(t testing.TB, root string) *Snapshot {
	t.Helper()
	return buildIndex(t, root).Snapshot()
}

// -------------------------------------------------------------------
// 1. BenchmarkBuildIndex — build the index from scratch at various
//    scales, measuring files/sec throughput.
// -------------------------------------------------------------------

func BenchmarkBuildIndex(b *testing.B) {
	scales := []struct {
		name string
		n    int
	}{
		{"1K", 1_000},
		{"10K", 10_000},
		{"100K", 100_000},
	}

	for _, sc := range scales {
		b.Run(sc.name, func(b *testing.B) {
			if sc.n >= 100_000 && testing.Short() {
				b.Skip("skipping large-scale benchmark")
			}

			dir := b.TempDir()
			generateFileTree(b, dir, sc.n, 42)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				idx := buildIndex(b, dir)
				if idx.Len() == 0 {
					b.Fatal("expected non-empty index")
				}
			}
			b.StopTimer()

			// Run once more to get a count for reporting.
			idx := buildIndex(b, dir)
			b.ReportMetric(float64(idx.Len())/b.Elapsed().Seconds(), "files/sec")
		})
	}
}

// -------------------------------------------------------------------
// 2. BenchmarkSearch_ByScale — search latency at various corpus
//    sizes and query types.
// -------------------------------------------------------------------

func BenchmarkSearch_ByScale(b *testing.B) {
	queries := []struct {
		name  string
		query string
	}{
		{"exact_basename", "handler.go"},
		{"short_query", "ha"},
		{"fuzzy_basename", "hndlr"},
		{"path_structured", "internal/handler"},
		{"multi_token", "api handler"},
	}

	scales := []struct {
		name string
		n    int
	}{
		{"1K", 1_000},
		{"10K", 10_000},
		{"100K", 100_000},
	}

	for _, sc := range scales {
		b.Run(sc.name, func(b *testing.B) {
			if sc.n >= 100_000 && testing.Short() {
				b.Skip("skipping large-scale benchmark")
			}

			dir := b.TempDir()
			generateFileTree(b, dir, sc.n, 42)
			idx := buildIndex(b, dir)

			// Build a snapshot that the search functions can
			// query against.
			snap := idx.Snapshot()
			plan := func(q string) *queryPlan { return newQueryPlan(q) }
			opts := DefaultSearchOptions()

			for _, q := range queries {
				b.Run(q.name, func(b *testing.B) {
					p := plan(q.query)
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						cands := searchSnapshot(p, snap, opts.MaxCandidates)
						if cands == nil {
							// Some queries may legitimately
							// return nil; that's fine.
							_ = cands
						}
					}
				})
			}
		})
	}
}

// -------------------------------------------------------------------
// 3. BenchmarkSearch_ConcurrentReads — verify lock-free reads
//    scale with goroutine count.
// -------------------------------------------------------------------

func BenchmarkSearch_ConcurrentReads(b *testing.B) {
	dir := b.TempDir()
	generateFileTree(b, dir, 10_000, 42)

	logger := slogtest.Make(b, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelError)
	ctx := context.Background()

	eng := NewEngine(logger)
	require.NoError(b, eng.AddRoot(ctx, dir))
	b.Cleanup(func() { _ = eng.Close() })

	opts := DefaultSearchOptions()
	goroutines := []int{1, 4, 16, 64}

	for _, g := range goroutines {
		b.Run(fmt.Sprintf("goroutines_%d", g), func(b *testing.B) {
			b.SetParallelism(g)
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					results, err := eng.Search(ctx, "handler", opts)
					if err != nil {
						b.Fatal(err)
					}
					_ = results
				}
			})
		})
	}
}

// -------------------------------------------------------------------
// 4. BenchmarkDeltaUpdate — applying delta updates (simulating
//    watcher events).
// -------------------------------------------------------------------

func BenchmarkDeltaUpdate(b *testing.B) {
	dir := b.TempDir()
	generateFileTree(b, dir, 10_000, 42)

	addCounts := []int{1, 10, 100}

	for _, count := range addCounts {
		b.Run(fmt.Sprintf("add_%d_files", count), func(b *testing.B) {
			// Pre-generate the paths we'll add.
			paths := make([]string, count)
			for i := range paths {
				paths[i] = fmt.Sprintf(
					"injected/dir_%d/newfile_%d.go", i%10, i,
				)
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				// Build a fresh index each iteration to
				// isolate the cost of adding.
				b.StopTimer()
				idx := buildIndex(b, dir)
				b.StartTimer()
				for _, p := range paths {
					idx.Add(p, 0)
				}
			}
			b.ReportMetric(float64(count), "files_added/op")
		})
	}

	// Benchmark search after delta additions to see if extra
	// delta docs degrade search latency.
	b.Run("search_after_100_additions", func(b *testing.B) {
		idx := buildIndex(b, dir)
		for i := 0; i < 100; i++ {
			idx.Add(fmt.Sprintf(
				"injected/extra/file_%d.go", i,
			), 0)
		}

		snap := idx.Snapshot()
		plan := newQueryPlan("handler")
		opts := DefaultSearchOptions()

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			cands := searchSnapshot(plan, snap, opts.MaxCandidates)
			_ = cands
		}
	})
}

// -------------------------------------------------------------------
// 5. TestMemoryProfile — report bytes per file in the delta index.
// -------------------------------------------------------------------

func TestMemoryProfile(t *testing.T) {
	scales := []struct {
		name string
		n    int
	}{
		{"10K", 10_000},
		{"100K", 100_000},
	}

	for _, sc := range scales {
		t.Run(sc.name, func(t *testing.T) {
			if sc.n >= 100_000 && testing.Short() {
				t.Skip("skipping large-scale memory profile")
			}

			dir := t.TempDir()
			generateFileTree(t, dir, sc.n, 42)

			// Use TotalAlloc (monotonically increasing) rather
			// than HeapAlloc to avoid underflow when GC frees
			// memory between measurements.
			runtime.GC()
			var before runtime.MemStats
			runtime.ReadMemStats(&before)

			idx := buildIndex(t, dir)

			var after runtime.MemStats
			runtime.ReadMemStats(&after)

			allocDelta := after.TotalAlloc - before.TotalAlloc
			bytesPerFile := float64(allocDelta) / float64(idx.Len())

			t.Logf("Scale:          %s", sc.name)
			t.Logf("Indexed docs:   %d", idx.Len())
			t.Logf("Total alloc:    %d bytes (%.1f MiB)",
				allocDelta, float64(allocDelta)/(1024*1024))
			t.Logf("Bytes per file: %.1f", bytesPerFile)

			// Also measure snapshot cost since snapshots are
			// published atomically. Snapshot() is O(1) with
			// no data copying. We use TotalAlloc which is
			// monotonically increasing to avoid underflow from
			// GC freeing memory between measurements.
			runtime.GC()
			runtime.ReadMemStats(&before)

			snap := idx.Snapshot()
			_ = snap

			runtime.GC()
			runtime.ReadMemStats(&after)

			snapTotalAlloc := after.TotalAlloc - before.TotalAlloc
			t.Logf("Snapshot alloc:    %d bytes (%.1f MiB)",
				snapTotalAlloc, float64(snapTotalAlloc)/(1024*1024))
			t.Logf("Snapshot bytes/file: %.1f",
				float64(snapTotalAlloc)/float64(idx.Len()))
		})
	}
}
// -------------------------------------------------------------------
// BenchmarkSearch_ConcurrentReads_Throughput — measures absolute
// operations per second at different concurrency levels to verify
// linear scaling.
// -------------------------------------------------------------------

func BenchmarkSearch_ConcurrentReads_Throughput(b *testing.B) {
	dir := b.TempDir()
	generateFileTree(b, dir, 10_000, 42)
	idx := buildIndex(b, dir)
	snap := idx.Snapshot()

	goroutines := []int{1, 4, 16, 64}
	plan := newQueryPlan("handler.go")
	maxCands := DefaultSearchOptions().MaxCandidates

	for _, g := range goroutines {
		b.Run(fmt.Sprintf("goroutines_%d", g), func(b *testing.B) {
			b.ResetTimer()

			var wg sync.WaitGroup
			perGoroutine := b.N / g
			if perGoroutine < 1 {
				perGoroutine = 1
			}

			for gi := 0; gi < g; gi++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for j := 0; j < perGoroutine; j++ {
						cands := searchSnapshot(plan, snap, maxCands)
						_ = cands
					}
				}()
			}
			wg.Wait()

			totalOps := float64(g * perGoroutine)
			b.ReportMetric(totalOps/b.Elapsed().Seconds(), "searches/sec")
		})
	}
}
