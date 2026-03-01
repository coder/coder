package filefinder

import (
	"slices"
	"testing"
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
			plan := newQueryPlan(tt.query)
			if plan.normalized != tt.wantNorm {
				t.Errorf("normalized = %q, want %q", plan.normalized, tt.wantNorm)
			}
			if plan.isShort != tt.wantShort {
				t.Errorf("isShort = %v, want %v", plan.isShort, tt.wantShort)
			}
			if plan.hasSlash != tt.wantSlash {
				t.Errorf("hasSlash = %v, want %v", plan.hasSlash, tt.wantSlash)
			}
			if string(plan.basenameQ) != tt.wantBase {
				t.Errorf("basenameQ = %q, want %q", plan.basenameQ, tt.wantBase)
			}
			if tt.wantTokens == nil {
				if len(plan.tokens) != 0 {
					t.Errorf("expected 0 tokens, got %d", len(plan.tokens))
				}
			} else {
				if len(plan.tokens) != len(tt.wantTokens) {
					t.Fatalf("tokens len = %d, want %d", len(plan.tokens), len(tt.wantTokens))
				}
				for i, tok := range plan.tokens {
					if string(tok) != tt.wantTokens[i] {
						t.Errorf("tokens[%d] = %q, want %q", i, tok, tt.wantTokens[i])
					}
				}
			}
			if tt.wantDirTok != nil {
				if len(plan.dirTokens) != len(tt.wantDirTok) {
					t.Fatalf("dirTokens len = %d, want %d", len(plan.dirTokens), len(tt.wantDirTok))
				}
				for i, tok := range plan.dirTokens {
					if string(tok) != tt.wantDirTok[i] {
						t.Errorf("dirTokens[%d] = %q, want %q", i, tok, tt.wantDirTok[i])
					}
				}
			}
			if tt.wantTriCnt >= 0 && len(plan.trigrams) != tt.wantTriCnt {
				t.Errorf("trigram count = %d, want %d", len(plan.trigrams), tt.wantTriCnt)
			}
		})
	}

	// ThreeChars: verify the actual trigram value.
	plan := newQueryPlan("abc")
	if want := packTrigram('a', 'b', 'c'); plan.trigrams[0] != want {
		t.Errorf("trigram = %x, want %x", plan.trigrams[0], want)
	}

	// ShortMultiToken: both tokens < 3 chars so isShort should be true.
	plan = newQueryPlan("ab cd")
	if !plan.isShort {
		t.Error("expected isShort=true when all tokens < 3 chars")
	}
	// One token >= 3 chars, so isShort should be false.
	plan = newQueryPlan("ab cde")
	if plan.isShort {
		t.Error("expected isShort=false when any token >= 3 chars")
	}
}

func makeTestSnapshot(paths []string) *Snapshot {
	idx := NewIndex()
	for _, p := range paths {
		idx.Add(p, 0)
	}
	return idx.Snapshot()
}

func requireCandHasPath(t *testing.T, cands []candidate, path string) {
	t.Helper()
	for _, c := range cands {
		if c.path == path {
			return
		}
	}
	t.Errorf("expected to find %q in candidates", path)
}

func TestSearchSnapshot_TrigramMatch(t *testing.T) {
	t.Parallel()
	snap := makeTestSnapshot([]string{"src/handler.go", "src/router.go", "lib/utils.go"})
	cands := searchSnapshot(newQueryPlan("handler"), snap, 100)
	if len(cands) == 0 {
		t.Fatal("expected at least 1 candidate for 'handler'")
	}
	requireCandHasPath(t, cands, "src/handler.go")
}

func TestSearchSnapshot_ShortQuery(t *testing.T) {
	t.Parallel()
	snap := makeTestSnapshot([]string{"foo.go", "bar.go", "fab.go"})
	cands := searchSnapshot(newQueryPlan("fo"), snap, 100)
	if len(cands) == 0 {
		t.Fatal("expected at least 1 candidate for 'fo'")
	}
	requireCandHasPath(t, cands, "foo.go")
}

func TestSearchSnapshot_FuzzyFallback(t *testing.T) {
	t.Parallel()
	snap := makeTestSnapshot([]string{"src/handler.go", "src/router.go", "lib/utils.go"})
	cands := searchSnapshot(newQueryPlan("hndlr"), snap, 100)
	if len(cands) == 0 {
		t.Fatal("expected fuzzy fallback to find 'handler.go' for query 'hndlr'")
	}
	requireCandHasPath(t, cands, "src/handler.go")
}

func TestSearchSnapshot_FuzzyFallbackNoFirstCharMatch(t *testing.T) {
	t.Parallel()
	snap := makeTestSnapshot([]string{"src/xylophone.go", "lib/extra.go"})
	cands := searchSnapshot(newQueryPlan("xylo"), snap, 100)
	if len(cands) == 0 {
		t.Fatal("expected at least 1 candidate for 'xylo'")
	}
	requireCandHasPath(t, cands, "src/xylophone.go")
}

func TestSearchSnapshot_NilSnapshot(t *testing.T) {
	t.Parallel()
	cands := searchSnapshot(newQueryPlan("foo"), nil, 100)
	if cands != nil {
		t.Errorf("expected nil for nil snapshot, got %v", cands)
	}
}

func TestSearchSnapshot_EmptyQuery(t *testing.T) {
	t.Parallel()
	snap := makeTestSnapshot([]string{"foo.go"})
	cands := searchSnapshot(newQueryPlan(""), snap, 100)
	if cands != nil {
		t.Errorf("expected nil for empty query, got %v", cands)
	}
}

func TestSearchSnapshot_DeletedDocsExcluded(t *testing.T) {
	t.Parallel()
	idx := NewIndex()
	idx.Add("handler.go", 0)
	idx.Remove("handler.go")
	snap := idx.Snapshot()
	cands := searchSnapshot(newQueryPlan("handler"), snap, 100)
	for _, c := range cands {
		if c.path == "handler.go" {
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
	snap := makeTestSnapshot(paths)
	cands := searchSnapshot(newQueryPlan("handler"), snap, 3)
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
			got := intersectSorted(tt.a, tt.b)
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
		if got := intersectAll(nil); got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})
	t.Run("single", func(t *testing.T) {
		t.Parallel()
		if got := intersectAll([][]uint32{{1, 2, 3}}); len(got) != 3 {
			t.Fatalf("len = %d, want 3", len(got))
		}
	})
	t.Run("multiple", func(t *testing.T) {
		t.Parallel()
		got := intersectAll([][]uint32{{1, 2, 3, 4, 5}, {2, 3, 5}, {3, 5, 7}})
		if !slices.Equal(got, []uint32{3, 5}) {
			t.Errorf("got %v, want [3 5]", got)
		}
	})
	t.Run("no overlap", func(t *testing.T) {
		t.Parallel()
		if got := intersectAll([][]uint32{{1, 2}, {3, 4}}); got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})
}

func TestMergeAndScore_SortedDescending(t *testing.T) {
	t.Parallel()
	plan := newQueryPlan("foo")
	params := DefaultScoreParams()
	cands := []candidate{
		{docID: 0, path: "a/b/c/d/e/foo", baseOff: 10, baseLen: 3, depth: 5},
		{docID: 1, path: "src/foo", baseOff: 4, baseLen: 3, depth: 1},
		{docID: 2, path: "foo", baseOff: 0, baseLen: 3, depth: 0},
	}
	results := mergeAndScore(cands, plan, params, 10)
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
	plan := newQueryPlan("f")
	params := DefaultScoreParams()
	var cands []candidate
	for i := range 20 {
		p := "f" + string(rune('a'+i))
		cands = append(cands, candidate{docID: uint32(i), path: p, baseOff: 0, baseLen: len(p), depth: 0})
	}
	if results := mergeAndScore(cands, plan, params, 5); len(results) != 5 {
		t.Errorf("expected 5 results, got %d", len(results))
	}
}

func TestMergeAndScore_ZeroTopK(t *testing.T) {
	t.Parallel()
	plan := newQueryPlan("foo")
	cands := []candidate{{docID: 0, path: "foo", baseOff: 0, baseLen: 3, depth: 0}}
	if results := mergeAndScore(cands, plan, DefaultScoreParams(), 0); len(results) != 0 {
		t.Errorf("expected 0 results for topK=0, got %d", len(results))
	}
}

func TestMergeAndScore_NoMatchCandidatesDropped(t *testing.T) {
	t.Parallel()
	plan := newQueryPlan("xyz")
	cands := []candidate{
		{docID: 0, path: "abc", baseOff: 0, baseLen: 3, depth: 0},
		{docID: 1, path: "def", baseOff: 0, baseLen: 3, depth: 0},
	}
	if results := mergeAndScore(cands, plan, DefaultScoreParams(), 10); len(results) != 0 {
		t.Errorf("expected 0 results for non-matching candidates, got %d", len(results))
	}
}

func TestMergeAndScore_IsDirFlag(t *testing.T) {
	t.Parallel()
	plan := newQueryPlan("foo")
	cands := []candidate{
		{docID: 0, path: "foo", baseOff: 0, baseLen: 3, depth: 0, flags: uint16(FlagDir)},
	}
	results := mergeAndScore(cands, plan, DefaultScoreParams(), 10)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].IsDir {
		t.Error("expected IsDir=true for FlagDir candidate")
	}
}

func TestMergeAndScore_EmptyCandidates(t *testing.T) {
	t.Parallel()
	if results := mergeAndScore(nil, newQueryPlan("foo"), DefaultScoreParams(), 10); len(results) != 0 {
		t.Errorf("expected 0 results for nil candidates, got %d", len(results))
	}
}

func TestSearchSnapshot_FuzzyFallbackEndToEnd(t *testing.T) {
	t.Parallel()
	snap := makeTestSnapshot([]string{"src/handler.go", "src/middleware.go", "pkg/config.go"})
	plan := newQueryPlan("hndlr")
	results := mergeAndScore(searchSnapshot(plan, snap, 100), plan, DefaultScoreParams(), 10)
	if len(results) == 0 {
		t.Fatal("expected fuzzy fallback to produce scored results for 'hndlr'")
	}
	if results[0].Path != "src/handler.go" {
		t.Errorf("expected top result 'src/handler.go', got %q", results[0].Path)
	}
}
