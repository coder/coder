package filefinder_test

import (
	"slices"
	"testing"

	"github.com/coder/coder/v2/agent/filefinder"
)

func TestNewQueryPlan(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		query      string
		wantNorm   string
		wantShort  bool
		wantSlash  bool
		wantBase   string
		wantTokens []string
		wantDirTok []string
		wantTriCnt int // -1 to skip check
	}{
		{"Simple", "foo", "foo", false, false, "foo", []string{"foo"}, nil, 1},
		{"MultiToken", "foo bar", "foo bar", false, false, "bar", []string{"foo", "bar"}, []string{"foo"}, -1},
		{"Slash", "internal/foo", "internal/foo", false, true, "foo", []string{"internal", "foo"}, []string{"internal"}, -1},
		{"SingleChar", "a", "a", true, false, "a", []string{"a"}, nil, 0},
		{"TwoChars", "ab", "ab", true, false, "ab", []string{"ab"}, nil, -1},
		{"ThreeChars", "abc", "abc", false, false, "abc", []string{"abc"}, nil, 1},
		{"DotPrefix", ".go", ".go", false, false, ".go", []string{".go"}, nil, -1},
		{"UpperCase", "FOO", "foo", false, false, "foo", []string{"foo"}, nil, -1},
		{"Empty", "", "", true, false, "", nil, nil, -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			plan := filefinder.NewQueryPlanForTest(tt.query)
			if plan.Normalized != tt.wantNorm {
				t.Errorf("normalized = %q, want %q", plan.Normalized, tt.wantNorm)
			}
			if plan.IsShort != tt.wantShort {
				t.Errorf("isShort = %v, want %v", plan.IsShort, tt.wantShort)
			}
			if plan.HasSlash != tt.wantSlash {
				t.Errorf("hasSlash = %v, want %v", plan.HasSlash, tt.wantSlash)
			}
			if string(plan.BasenameQ) != tt.wantBase {
				t.Errorf("basenameQ = %q, want %q", plan.BasenameQ, tt.wantBase)
			}
			if tt.wantTokens == nil {
				if len(plan.Tokens) != 0 {
					t.Errorf("expected 0 tokens, got %d", len(plan.Tokens))
				}
			} else {
				if len(plan.Tokens) != len(tt.wantTokens) {
					t.Fatalf("tokens len = %d, want %d", len(plan.Tokens), len(tt.wantTokens))
				}
				for i, tok := range plan.Tokens {
					if string(tok) != tt.wantTokens[i] {
						t.Errorf("tokens[%d] = %q, want %q", i, tok, tt.wantTokens[i])
					}
				}
			}
			if tt.wantDirTok != nil {
				if len(plan.DirTokens) != len(tt.wantDirTok) {
					t.Fatalf("dirTokens len = %d, want %d", len(plan.DirTokens), len(tt.wantDirTok))
				}
				for i, tok := range plan.DirTokens {
					if string(tok) != tt.wantDirTok[i] {
						t.Errorf("dirTokens[%d] = %q, want %q", i, tok, tt.wantDirTok[i])
					}
				}
			}
			if tt.wantTriCnt >= 0 && len(plan.Trigrams) != tt.wantTriCnt {
				t.Errorf("trigram count = %d, want %d", len(plan.Trigrams), tt.wantTriCnt)
			}
		})
	}

	// ThreeChars: verify the actual trigram value.
	plan := filefinder.NewQueryPlanForTest("abc")
	if want := filefinder.PackTrigramForTest('a', 'b', 'c'); plan.Trigrams[0] != want {
		t.Errorf("trigram = %x, want %x", plan.Trigrams[0], want)
	}

	// ShortMultiToken: both tokens < 3 chars so isShort should be true.
	plan = filefinder.NewQueryPlanForTest("ab cd")
	if !plan.IsShort {
		t.Error("expected isShort=true when all tokens < 3 chars")
	}
	// One token >= 3 chars, so isShort should be false.
	plan = filefinder.NewQueryPlanForTest("ab cde")
	if plan.IsShort {
		t.Error("expected isShort=false when any token >= 3 chars")
	}
}

func requireCandHasPath(t *testing.T, cands []filefinder.CandidateForTest, path string) {
	t.Helper()
	for _, c := range cands {
		if c.Path == path {
			return
		}
	}
	t.Errorf("expected to find %q in candidates", path)
}

func TestSearchSnapshot_TrigramMatch(t *testing.T) {
	t.Parallel()
	snap := filefinder.MakeTestSnapshot([]string{"src/handler.go", "src/router.go", "lib/utils.go"})
	cands := filefinder.SearchSnapshotForTest(filefinder.NewQueryPlanForTest("handler"), snap, 100)
	if len(cands) == 0 {
		t.Fatal("expected at least 1 candidate for 'handler'")
	}
	requireCandHasPath(t, cands, "src/handler.go")
}

func TestSearchSnapshot_ShortQuery(t *testing.T) {
	t.Parallel()
	snap := filefinder.MakeTestSnapshot([]string{"foo.go", "bar.go", "fab.go"})
	cands := filefinder.SearchSnapshotForTest(filefinder.NewQueryPlanForTest("fo"), snap, 100)
	if len(cands) == 0 {
		t.Fatal("expected at least 1 candidate for 'fo'")
	}
	requireCandHasPath(t, cands, "foo.go")
}

func TestSearchSnapshot_FuzzyFallback(t *testing.T) {
	t.Parallel()
	snap := filefinder.MakeTestSnapshot([]string{"src/handler.go", "src/router.go", "lib/utils.go"})
	cands := filefinder.SearchSnapshotForTest(filefinder.NewQueryPlanForTest("hndlr"), snap, 100)
	if len(cands) == 0 {
		t.Fatal("expected fuzzy fallback to find 'handler.go' for query 'hndlr'")
	}
	requireCandHasPath(t, cands, "src/handler.go")
}

func TestSearchSnapshot_FuzzyFallbackNoFirstCharMatch(t *testing.T) {
	t.Parallel()
	snap := filefinder.MakeTestSnapshot([]string{"src/xylophone.go", "lib/extra.go"})
	cands := filefinder.SearchSnapshotForTest(filefinder.NewQueryPlanForTest("xylo"), snap, 100)
	if len(cands) == 0 {
		t.Fatal("expected at least 1 candidate for 'xylo'")
	}
	requireCandHasPath(t, cands, "src/xylophone.go")
}

func TestSearchSnapshot_NilSnapshot(t *testing.T) {
	t.Parallel()
	cands := filefinder.SearchSnapshotForTest(filefinder.NewQueryPlanForTest("foo"), nil, 100)
	if cands != nil {
		t.Errorf("expected nil for nil snapshot, got %v", cands)
	}
}

func TestSearchSnapshot_EmptyQuery(t *testing.T) {
	t.Parallel()
	snap := filefinder.MakeTestSnapshot([]string{"foo.go"})
	cands := filefinder.SearchSnapshotForTest(filefinder.NewQueryPlanForTest(""), snap, 100)
	if cands != nil {
		t.Errorf("expected nil for empty query, got %v", cands)
	}
}

func TestSearchSnapshot_DeletedDocsExcluded(t *testing.T) {
	t.Parallel()
	idx := filefinder.NewIndex()
	idx.Add("handler.go", 0)
	idx.Remove("handler.go")
	snap := idx.Snapshot()
	cands := filefinder.SearchSnapshotForTest(filefinder.NewQueryPlanForTest("handler"), snap, 100)
	for _, c := range cands {
		if c.Path == "handler.go" {
			t.Error("deleted doc should not appear in results")
		}
	}
}

func TestSearchSnapshot_Limit(t *testing.T) {
	t.Parallel()
	paths := make([]string, 50)
	for i := range paths {
		paths[i] = "handler" + string(rune('a'+i%26)) + ".go"
	}
	snap := filefinder.MakeTestSnapshot(paths)
	cands := filefinder.SearchSnapshotForTest(filefinder.NewQueryPlanForTest("handler"), snap, 3)
	if len(cands) > 3 {
		t.Errorf("expected at most 3 candidates, got %d", len(cands))
	}
}

func TestIntersectSorted(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		a, b []uint32
		want []uint32
	}{
		{"both empty", nil, nil, nil},
		{"a empty", nil, []uint32{1, 2}, nil},
		{"b empty", []uint32{1, 2}, nil, nil},
		{"no overlap", []uint32{1, 3, 5}, []uint32{2, 4, 6}, nil},
		{"full overlap", []uint32{1, 2, 3}, []uint32{1, 2, 3}, []uint32{1, 2, 3}},
		{"partial overlap", []uint32{1, 2, 3, 5}, []uint32{2, 4, 5}, []uint32{2, 5}},
		{"single match", []uint32{1, 2, 3}, []uint32{2}, []uint32{2}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := filefinder.IntersectSortedForTest(tt.a, tt.b)
			if len(tt.want) == 0 {
				if len(got) != 0 {
					t.Errorf("got %v, want empty/nil", got)
				}
				return
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIntersectAll(t *testing.T) {
	t.Parallel()
	t.Run("empty", func(t *testing.T) {
		t.Parallel()
		if got := filefinder.IntersectAllForTest(nil); got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})
	t.Run("single", func(t *testing.T) {
		t.Parallel()
		if got := filefinder.IntersectAllForTest([][]uint32{{1, 2, 3}}); len(got) != 3 {
			t.Fatalf("len = %d, want 3", len(got))
		}
	})
	t.Run("multiple", func(t *testing.T) {
		t.Parallel()
		got := filefinder.IntersectAllForTest([][]uint32{{1, 2, 3, 4, 5}, {2, 3, 5}, {3, 5, 7}})
		if !slices.Equal(got, []uint32{3, 5}) {
			t.Errorf("got %v, want [3 5]", got)
		}
	})
	t.Run("no overlap", func(t *testing.T) {
		t.Parallel()
		if got := filefinder.IntersectAllForTest([][]uint32{{1, 2}, {3, 4}}); got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})
}

func TestMergeAndScore_SortedDescending(t *testing.T) {
	t.Parallel()
	plan := filefinder.NewQueryPlanForTest("foo")
	params := filefinder.DefaultScoreParamsForTest()
	cands := []filefinder.CandidateForTest{
		{DocID: 0, Path: "a/b/c/d/e/foo", BaseOff: 10, BaseLen: 3, Depth: 5},
		{DocID: 1, Path: "src/foo", BaseOff: 4, BaseLen: 3, Depth: 1},
		{DocID: 2, Path: "foo", BaseOff: 0, BaseLen: 3, Depth: 0},
	}
	results := filefinder.MergeAndScoreForTest(cands, plan, params, 10)
	if len(results) == 0 {
		t.Fatal("expected non-empty results")
	}
	for i := 1; i < len(results); i++ {
		if results[i].Score > results[i-1].Score {
			t.Errorf("results not sorted: [%d].Score=%f > [%d].Score=%f",
				i, results[i].Score, i-1, results[i-1].Score)
		}
	}
}

func TestMergeAndScore_TopKLimit(t *testing.T) {
	t.Parallel()
	plan := filefinder.NewQueryPlanForTest("f")
	params := filefinder.DefaultScoreParamsForTest()
	var cands []filefinder.CandidateForTest
	for i := range 20 {
		p := "f" + string(rune('a'+i))
		cands = append(cands, filefinder.CandidateForTest{DocID: uint32(i), Path: p, BaseOff: 0, BaseLen: len(p), Depth: 0}) //nolint:gosec // test index is tiny
	}
	if results := filefinder.MergeAndScoreForTest(cands, plan, params, 5); len(results) != 5 {
		t.Errorf("expected 5 results, got %d", len(results))
	}
}

func TestMergeAndScore_ZeroTopK(t *testing.T) {
	t.Parallel()
	plan := filefinder.NewQueryPlanForTest("foo")
	cands := []filefinder.CandidateForTest{{DocID: 0, Path: "foo", BaseOff: 0, BaseLen: 3, Depth: 0}}
	if results := filefinder.MergeAndScoreForTest(cands, plan, filefinder.DefaultScoreParamsForTest(), 0); len(results) != 0 {
		t.Errorf("expected 0 results for topK=0, got %d", len(results))
	}
}

func TestMergeAndScore_NoMatchCandidatesDropped(t *testing.T) {
	t.Parallel()
	plan := filefinder.NewQueryPlanForTest("xyz")
	cands := []filefinder.CandidateForTest{
		{DocID: 0, Path: "abc", BaseOff: 0, BaseLen: 3, Depth: 0},
		{DocID: 1, Path: "def", BaseOff: 0, BaseLen: 3, Depth: 0},
	}
	if results := filefinder.MergeAndScoreForTest(cands, plan, filefinder.DefaultScoreParamsForTest(), 10); len(results) != 0 {
		t.Errorf("expected 0 results for non-matching candidates, got %d", len(results))
	}
}

func TestMergeAndScore_IsDirFlag(t *testing.T) {
	t.Parallel()
	plan := filefinder.NewQueryPlanForTest("foo")
	cands := []filefinder.CandidateForTest{
		{DocID: 0, Path: "foo", BaseOff: 0, BaseLen: 3, Depth: 0, Flags: uint16(filefinder.FlagDir)},
	}
	results := filefinder.MergeAndScoreForTest(cands, plan, filefinder.DefaultScoreParamsForTest(), 10)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].IsDir {
		t.Error("expected IsDir=true for FlagDir candidate")
	}
}

func TestMergeAndScore_EmptyCandidates(t *testing.T) {
	t.Parallel()
	if results := filefinder.MergeAndScoreForTest(nil, filefinder.NewQueryPlanForTest("foo"), filefinder.DefaultScoreParamsForTest(), 10); len(results) != 0 {
		t.Errorf("expected 0 results for nil candidates, got %d", len(results))
	}
}

func TestSearchSnapshot_FuzzyFallbackEndToEnd(t *testing.T) {
	t.Parallel()
	snap := filefinder.MakeTestSnapshot([]string{"src/handler.go", "src/middleware.go", "pkg/config.go"})
	plan := filefinder.NewQueryPlanForTest("hndlr")
	results := filefinder.MergeAndScoreForTest(filefinder.SearchSnapshotForTest(plan, snap, 100), plan, filefinder.DefaultScoreParamsForTest(), 10)
	if len(results) == 0 {
		t.Fatal("expected fuzzy fallback to produce scored results for 'hndlr'")
	}
	if results[0].Path != "src/handler.go" {
		t.Errorf("expected top result 'src/handler.go', got %q", results[0].Path)
	}
}
