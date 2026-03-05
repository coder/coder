package filefinder_test

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
	"github.com/coder/coder/v2/agent/filefinder"
)

var (
	dirNames = []string{
		"cmd", "internal", "pkg", "api", "auth", "database", "server", "client", "middleware",
		"handler", "config", "utils", "models", "service", "worker", "scheduler", "notification",
		"provisioner", "template", "workspace", "agent", "proxy", "crypto", "telemetry", "billing",
	}
	fileExts = []string{
		".go", ".ts", ".tsx", ".js", ".py", ".sql", ".yaml", ".json", ".md", ".proto", ".sh",
	}
	fileStems = []string{
		"main", "handler", "middleware", "service", "model", "query", "config", "utils", "helpers",
		"types", "interface", "test", "mock", "factory", "builder", "adapter", "observer", "provider",
		"resolver", "schema", "migration", "fixture", "snapshot", "checkpoint",
	}
)

// generateFileTree creates n files under root in a realistic nested directory structure.
func generateFileTree(t testing.TB, root string, n int, seed int64) {
	t.Helper()
	rng := rand.New(rand.NewSource(seed)) //nolint:gosec // deterministic benchmarks

	numDirs := n / 5
	if numDirs < 10 {
		numDirs = 10
	}
	dirs := make([]string, 0, numDirs)
	for i := 0; i < numDirs; i++ {
		depth := rng.Intn(6) + 1
		parts := make([]string, depth)
		for d := 0; d < depth; d++ {
			parts[d] = dirNames[rng.Intn(len(dirNames))]
		}
		dirs = append(dirs, filepath.Join(parts...))
	}

	created := make(map[string]struct{})
	for _, d := range dirs {
		full := filepath.Join(root, d)
		if _, ok := created[full]; ok {
			continue
		}
		require.NoError(t, os.MkdirAll(full, 0o755))
		created[full] = struct{}{}
	}

	for i := 0; i < n; i++ {
		dir := dirs[rng.Intn(len(dirs))]
		stem := fileStems[rng.Intn(len(fileStems))]
		ext := fileExts[rng.Intn(len(fileExts))]
		name := fmt.Sprintf("%s_%d%s", stem, i, ext)
		full := filepath.Join(root, dir, name)
		f, err := os.Create(full)
		require.NoError(t, err)
		_ = f.Close()
	}
}

// buildIndex walks root and returns a populated Index, the same
// way Engine.AddRoot does but without starting a watcher.
func buildIndex(t testing.TB, root string) *filefinder.Index {
	t.Helper()
	absRoot, err := filepath.Abs(root)
	require.NoError(t, err)
	idx, err := filefinder.BuildTestIndex(absRoot)
	require.NoError(t, err)
	return idx
}

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

			idx := buildIndex(b, dir)
			b.ReportMetric(float64(idx.Len())/b.Elapsed().Seconds(), "files/sec")
		})
	}
}

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
			snap := idx.Snapshot()
			opts := filefinder.DefaultSearchOptions()

			for _, q := range queries {
				b.Run(q.name, func(b *testing.B) {
					p := filefinder.NewQueryPlanForTest(q.query)
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						_ = filefinder.SearchSnapshotForTest(p, snap, opts.MaxCandidates)
					}
				})
			}
		})
	}
}

func BenchmarkSearch_ConcurrentReads(b *testing.B) {
	dir := b.TempDir()
	generateFileTree(b, dir, 10_000, 42)

	logger := slogtest.Make(b, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelError)
	ctx := context.Background()
	eng := filefinder.NewEngine(logger)
	require.NoError(b, eng.AddRoot(ctx, dir))
	b.Cleanup(func() { _ = eng.Close() })

	opts := filefinder.DefaultSearchOptions()
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

func BenchmarkDeltaUpdate(b *testing.B) {
	dir := b.TempDir()
	generateFileTree(b, dir, 10_000, 42)

	addCounts := []int{1, 10, 100}

	for _, count := range addCounts {
		b.Run(fmt.Sprintf("add_%d_files", count), func(b *testing.B) {
			paths := make([]string, count)
			for i := range paths {
				paths[i] = fmt.Sprintf("injected/dir_%d/newfile_%d.go", i%10, i)
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
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

	b.Run("search_after_100_additions", func(b *testing.B) {
		idx := buildIndex(b, dir)
		for i := 0; i < 100; i++ {
			idx.Add(fmt.Sprintf("injected/extra/file_%d.go", i), 0)
		}
		snap := idx.Snapshot()
		plan := filefinder.NewQueryPlanForTest("handler")
		opts := filefinder.DefaultSearchOptions()

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = filefinder.SearchSnapshotForTest(plan, snap, opts.MaxCandidates)
		}
	})
}

func BenchmarkMemoryProfile(b *testing.B) {
	scales := []struct {
		name string
		n    int
	}{
		{"10K", 10_000},
		{"100K", 100_000},
	}

	for _, sc := range scales {
		b.Run(sc.name, func(b *testing.B) {
			if sc.n >= 100_000 && testing.Short() {
				b.Skip("skipping large-scale memory profile")
			}
			dir := b.TempDir()
			generateFileTree(b, dir, sc.n, 42)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				idx := buildIndex(b, dir)
				_ = idx.Snapshot()
			}
			b.StopTimer()

			// Report memory stats on the last iteration.
			runtime.GC()
			var before runtime.MemStats
			runtime.ReadMemStats(&before)
			idx := buildIndex(b, dir)
			var after runtime.MemStats
			runtime.ReadMemStats(&after)

			allocDelta := after.TotalAlloc - before.TotalAlloc
			b.ReportMetric(float64(allocDelta)/float64(idx.Len()), "bytes/file")

			runtime.GC()
			runtime.ReadMemStats(&before)
			snap := idx.Snapshot()
			_ = snap
			runtime.GC()
			runtime.ReadMemStats(&after)

			snapAlloc := after.TotalAlloc - before.TotalAlloc
			b.ReportMetric(float64(snapAlloc)/float64(idx.Len()), "snap-bytes/file")
		})
	}
}

func BenchmarkSearch_ConcurrentReads_Throughput(b *testing.B) {
	dir := b.TempDir()
	generateFileTree(b, dir, 10_000, 42)
	idx := buildIndex(b, dir)
	snap := idx.Snapshot()

	goroutines := []int{1, 4, 16, 64}
	plan := filefinder.NewQueryPlanForTest("handler.go")
	maxCands := filefinder.DefaultSearchOptions().MaxCandidates

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
						_ = filefinder.SearchSnapshotForTest(plan, snap, maxCands)
					}
				}()
			}
			wg.Wait()
			totalOps := float64(g * perGoroutine)
			b.ReportMetric(totalOps/b.Elapsed().Seconds(), "searches/sec")
		})
	}
}
