package filefinder

import (
	"testing"
)

func TestNewQueryPlan_Simple(t *testing.T) {
	t.Parallel()

	plan := newQueryPlan("foo")
	if plan.original != "foo" {
		t.Errorf("original = %q, want %q", plan.original, "foo")
	}
	if plan.normalized != "foo" {
		t.Errorf("normalized = %q, want %q", plan.normalized, "foo")
	}
	if len(plan.tokens) != 1 {
		t.Fatalf("tokens len = %d, want 1", len(plan.tokens))
	}
	if string(plan.tokens[0]) != "foo" {
		t.Errorf("tokens[0] = %q, want %q", plan.tokens[0], "foo")
	}
	if plan.isShort {
		t.Error("expected isShort=false for 3-char query")
	}
	if plan.hasSlash {
		t.Error("expected hasSlash=false")
	}
	if string(plan.basenameQ) != "foo" {
		t.Errorf("basenameQ = %q, want %q", plan.basenameQ, "foo")
	}
}

func TestNewQueryPlan_MultiToken(t *testing.T) {
	t.Parallel()

	plan := newQueryPlan("foo bar")
	if len(plan.tokens) != 2 {
		t.Fatalf("tokens len = %d, want 2", len(plan.tokens))
	}
	if string(plan.tokens[0]) != "foo" {
		t.Errorf("tokens[0] = %q, want %q", plan.tokens[0], "foo")
	}
	if string(plan.tokens[1]) != "bar" {
		t.Errorf("tokens[1] = %q, want %q", plan.tokens[1], "bar")
	}
	if string(plan.basenameQ) != "bar" {
		t.Errorf("basenameQ = %q, want %q", plan.basenameQ, "bar")
	}
	// "foo" should be a dir token since it's before the last.
	if len(plan.dirTokens) != 1 || string(plan.dirTokens[0]) != "foo" {
		t.Errorf("dirTokens = %v, want [foo]", plan.dirTokens)
	}
}

func TestNewQueryPlan_Slash(t *testing.T) {
	t.Parallel()

	plan := newQueryPlan("internal/foo")
	if !plan.hasSlash {
		t.Error("expected hasSlash=true")
	}
	if len(plan.tokens) != 2 {
		t.Fatalf("tokens len = %d, want 2", len(plan.tokens))
	}
	if string(plan.tokens[0]) != "internal" {
		t.Errorf("tokens[0] = %q, want %q", plan.tokens[0], "internal")
	}
	if string(plan.tokens[1]) != "foo" {
		t.Errorf("tokens[1] = %q, want %q", plan.tokens[1], "foo")
	}
	if string(plan.basenameQ) != "foo" {
		t.Errorf("basenameQ = %q, want %q", plan.basenameQ, "foo")
	}
	if len(plan.dirTokens) != 1 || string(plan.dirTokens[0]) != "internal" {
		t.Errorf("dirTokens = %v, want [internal]", plan.dirTokens)
	}
}

func TestNewQueryPlan_SingleChar(t *testing.T) {
	t.Parallel()

	plan := newQueryPlan("a")
	if !plan.isShort {
		t.Error("expected isShort=true for single char")
	}
	if len(plan.trigrams) != 0 {
		t.Errorf("expected no trigrams for single char, got %d", len(plan.trigrams))
	}
}

func TestNewQueryPlan_TwoChars(t *testing.T) {
	t.Parallel()

	plan := newQueryPlan("ab")
	if !plan.isShort {
		t.Error("expected isShort=true for two chars")
	}
	if string(plan.basenameQ) != "ab" {
		t.Errorf("basenameQ = %q, want %q", plan.basenameQ, "ab")
	}
}

func TestNewQueryPlan_ThreeChars(t *testing.T) {
	t.Parallel()

	plan := newQueryPlan("abc")
	if plan.isShort {
		t.Error("expected isShort=false for 3-char query")
	}
	if len(plan.trigrams) != 1 {
		t.Fatalf("expected 1 trigram, got %d", len(plan.trigrams))
	}
	want := packTrigram('a', 'b', 'c')
	if plan.trigrams[0] != want {
		t.Errorf("trigram = %x, want %x", plan.trigrams[0], want)
	}
}

func TestNewQueryPlan_DotPrefix(t *testing.T) {
	t.Parallel()

	plan := newQueryPlan(".go")
	if plan.isShort {
		t.Error("expected isShort=false for 3-char query")
	}
	if string(plan.basenameQ) != ".go" {
		t.Errorf("basenameQ = %q, want %q", plan.basenameQ, ".go")
	}
	if plan.hasSlash {
		t.Error("expected hasSlash=false")
	}
}

func TestNewQueryPlan_UpperCase(t *testing.T) {
	t.Parallel()

	plan := newQueryPlan("FOO")
	if plan.normalized != "foo" {
		t.Errorf("normalized = %q, want %q", plan.normalized, "foo")
	}
}

func TestNewQueryPlan_Empty(t *testing.T) {
	t.Parallel()

	plan := newQueryPlan("")
	if !plan.isShort {
		t.Error("expected isShort=true for empty query")
	}
	if len(plan.tokens) != 0 {
		t.Errorf("expected 0 tokens, got %d", len(plan.tokens))
	}
}

func TestNewQueryPlan_ShortMultiToken(t *testing.T) {
	t.Parallel()

	// Both tokens are < 3 chars so isShort should be true.
	plan := newQueryPlan("ab cd")
	if !plan.isShort {
		t.Error("expected isShort=true when all tokens < 3 chars")
	}

	// One token >= 3 chars, so isShort should be false.
	plan2 := newQueryPlan("ab cde")
	if plan2.isShort {
		t.Error("expected isShort=false when any token >= 3 chars")
	}
}

// makeTestSnapshot builds a Snapshot from a list of paths using
// the Index type from delta.go. This avoids constructing internal
// fields by hand.
func makeTestSnapshot(paths []string) *Snapshot {
	idx := NewIndex()
	for _, p := range paths {
		idx.Add(p, 0)
	}
	return idx.Snapshot()
}

func TestSearchSnapshot_TrigramMatch(t *testing.T) {
	t.Parallel()

	snap := makeTestSnapshot([]string{
		"src/handler.go",
		"src/router.go",
		"lib/utils.go",
	})

	plan := newQueryPlan("handler")
	cands := searchSnapshot(plan, snap, 100)

	if len(cands) == 0 {
		t.Fatal("expected at least 1 candidate for 'handler'")
	}
	found := false
	for _, c := range cands {
		if c.path == "src/handler.go" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected to find 'src/handler.go' in candidates")
	}
}

func TestSearchSnapshot_ShortQuery(t *testing.T) {
	t.Parallel()

	snap := makeTestSnapshot([]string{
		"foo.go",
		"bar.go",
		"fab.go",
	})

	plan := newQueryPlan("fo")
	cands := searchSnapshot(plan, snap, 100)

	if len(cands) == 0 {
		t.Fatal("expected at least 1 candidate for 'fo'")
	}
	found := false
	for _, c := range cands {
		if c.path == "foo.go" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected to find 'foo.go' for short query 'fo'")
	}
}

func TestSearchSnapshot_FuzzyFallback(t *testing.T) {
	t.Parallel()

	snap := makeTestSnapshot([]string{
		"src/handler.go",
		"src/router.go",
		"lib/utils.go",
	})

	// "hndlr" has no trigram overlap with "handler.go"
	// because there are no 3-char substrings in common.
	// The fuzzy fallback should find it via subsequence
	// matching.
	plan := newQueryPlan("hndlr")
	cands := searchSnapshot(plan, snap, 100)

	if len(cands) == 0 {
		t.Fatal("expected fuzzy fallback to find 'handler.go' for query 'hndlr'")
	}
	found := false
	for _, c := range cands {
		if c.path == "src/handler.go" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'src/handler.go' in candidates, got %v", cands)
	}
}

func TestSearchSnapshot_FuzzyFallbackNoFirstCharMatch(t *testing.T) {
	t.Parallel()

	// Query starts with 'x' which won't match any prefix1
	// bucket, so searchFuzzyFallback should fall through to
	// searchSubsequenceScan.
	snap := makeTestSnapshot([]string{
		"src/xylophone.go",
		"lib/extra.go",
	})

	plan := newQueryPlan("xylo")
	cands := searchSnapshot(plan, snap, 100)

	if len(cands) == 0 {
		t.Fatal("expected at least 1 candidate for 'xylo'")
	}
	found := false
	for _, c := range cands {
		if c.path == "src/xylophone.go" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected to find 'src/xylophone.go'")
	}
}

func TestSearchSnapshot_NilSnapshot(t *testing.T) {
	t.Parallel()

	plan := newQueryPlan("foo")
	cands := searchSnapshot(plan, nil, 100)
	if cands != nil {
		t.Errorf("expected nil for nil snapshot, got %v", cands)
	}
}

func TestSearchSnapshot_EmptyQuery(t *testing.T) {
	t.Parallel()

	snap := makeTestSnapshot([]string{"foo.go"})
	plan := newQueryPlan("")
	cands := searchSnapshot(plan, snap, 100)
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

	plan := newQueryPlan("handler")
	cands := searchSnapshot(plan, snap, 100)

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

	plan := newQueryPlan("handler")
	cands := searchSnapshot(plan, snap, 3)

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
			if tt.want == nil {
				if got != nil && len(got) != 0 {
					t.Errorf("got %v, want nil", got)
				}
				return
			}
			if len(got) != len(tt.want) {
				t.Fatalf("len = %d, want %d", len(got), len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("[%d] = %d, want %d", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestIntersectAll(t *testing.T) {
	t.Parallel()

	t.Run("empty", func(t *testing.T) {
		t.Parallel()
		got := intersectAll(nil)
		if got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})

	t.Run("single", func(t *testing.T) {
		t.Parallel()
		got := intersectAll([][]uint32{{1, 2, 3}})
		if len(got) != 3 {
			t.Fatalf("len = %d, want 3", len(got))
		}
	})

	t.Run("multiple", func(t *testing.T) {
		t.Parallel()
		lists := [][]uint32{
			{1, 2, 3, 4, 5},
			{2, 3, 5},
			{3, 5, 7},
		}
		got := intersectAll(lists)
		want := []uint32{3, 5}
		if len(got) != len(want) {
			t.Fatalf("len = %d, want %d", len(got), len(want))
		}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("[%d] = %d, want %d", i, got[i], want[i])
			}
		}
	})

	t.Run("no overlap", func(t *testing.T) {
		t.Parallel()
		lists := [][]uint32{
			{1, 2},
			{3, 4},
		}
		got := intersectAll(lists)
		if got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})
}

func TestMergeAndScore_SortedDescending(t *testing.T) {
	t.Parallel()

	plan := newQueryPlan("foo")
	params := DefaultScoreParams()

	cands := []candidate{
		{
			docID: 0, path: "a/b/c/d/e/foo",
			baseOff: 10, baseLen: 3, depth: 5,
		},
		{
			docID: 1, path: "src/foo",
			baseOff: 4, baseLen: 3, depth: 1,
		},
		{
			docID: 2, path: "foo",
			baseOff: 0, baseLen: 3, depth: 0,
		},
	}

	results := mergeAndScore(cands, plan, params, 10)

	if len(results) == 0 {
		t.Fatal("expected non-empty results")
	}

	// Results should be sorted by score descending.
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
		cands = append(cands, candidate{
			docID:   uint32(i),
			path:    p,
			baseOff: 0,
			baseLen: len(p),
			depth:   0,
		})
	}

	results := mergeAndScore(cands, plan, params, 5)
	if len(results) != 5 {
		t.Errorf("expected 5 results, got %d", len(results))
	}
}

func TestMergeAndScore_ZeroTopK(t *testing.T) {
	t.Parallel()

	plan := newQueryPlan("foo")
	params := DefaultScoreParams()

	cands := []candidate{
		{docID: 0, path: "foo", baseOff: 0, baseLen: 3, depth: 0},
	}

	results := mergeAndScore(cands, plan, params, 0)
	if len(results) != 0 {
		t.Errorf("expected 0 results for topK=0, got %d", len(results))
	}
}

func TestMergeAndScore_NoMatchCandidatesDropped(t *testing.T) {
	t.Parallel()

	plan := newQueryPlan("xyz")
	params := DefaultScoreParams()

	// These candidates don't contain "xyz" as a subsequence.
	cands := []candidate{
		{docID: 0, path: "abc", baseOff: 0, baseLen: 3, depth: 0},
		{docID: 1, path: "def", baseOff: 0, baseLen: 3, depth: 0},
	}

	results := mergeAndScore(cands, plan, params, 10)
	if len(results) != 0 {
		t.Errorf("expected 0 results for non-matching candidates, got %d", len(results))
	}
}

func TestMergeAndScore_IsDirFlag(t *testing.T) {
	t.Parallel()

	plan := newQueryPlan("foo")
	params := DefaultScoreParams()

	cands := []candidate{
		{
			docID: 0, path: "foo",
			baseOff: 0, baseLen: 3, depth: 0,
			flags: uint16(FlagDir),
		},
	}

	results := mergeAndScore(cands, plan, params, 10)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].IsDir {
		t.Error("expected IsDir=true for FlagDir candidate")
	}
}

func TestMergeAndScore_EmptyCandidates(t *testing.T) {
	t.Parallel()

	plan := newQueryPlan("foo")
	params := DefaultScoreParams()

	results := mergeAndScore(nil, plan, params, 10)
	if len(results) != 0 {
		t.Errorf("expected 0 results for nil candidates, got %d", len(results))
	}
}

func TestSearchSnapshot_FuzzyFallbackEndToEnd(t *testing.T) {
	t.Parallel()

	// End-to-end test: build a snapshot, search with a fuzzy
	// query that has no trigram overlap, and verify scored
	// results come back.
	snap := makeTestSnapshot([]string{
		"src/handler.go",
		"src/middleware.go",
		"pkg/config.go",
	})

	plan := newQueryPlan("hndlr")
	cands := searchSnapshot(plan, snap, 100)
	results := mergeAndScore(cands, plan, DefaultScoreParams(), 10)

	if len(results) == 0 {
		t.Fatal("expected fuzzy fallback to produce scored results for 'hndlr'")
	}
	if results[0].Path != "src/handler.go" {
		t.Errorf("expected top result 'src/handler.go', got %q", results[0].Path)
	}
}
