package filefinder

import (
	"testing"
)

func TestIsSubsequence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		haystack string
		needle   string
		want     bool
	}{
		{"empty needle", "anything", "", true},
		{"empty both", "", "", true},
		{"empty haystack", "", "a", false},
		{"exact match", "abc", "abc", true},
		{"scattered", "axbycz", "abc", true},
		{"prefix", "abcdef", "abc", true},
		{"suffix", "xyzabc", "abc", true},
		{"case insensitive", "AbCdEf", "ace", true},
		{"case insensitive reverse", "abcdef", "ACE", true},
		{"no match", "abcdef", "xyz", false},
		{"partial match", "abcdef", "abz", false},
		{"longer needle", "ab", "abc", false},
		{"single char match", "hello", "l", true},
		{"single char no match", "hello", "z", false},
		{"path like", "src/internal/foo.go", "sif", true},
		{"path like no match", "src/internal/foo.go", "zzz", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := isSubsequence([]byte(tt.haystack), []byte(tt.needle))
			if got != tt.want {
				t.Errorf("isSubsequence(%q, %q) = %v, want %v",
					tt.haystack, tt.needle, got, tt.want)
			}
		})
	}
}

func TestLongestContiguousMatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		haystack string
		needle   string
		want     int
	}{
		{"empty needle", "abc", "", 0},
		{"empty haystack", "", "abc", 0},
		{"full match", "abc", "abc", 3},
		{"prefix match", "abcdef", "abc", 3},
		{"middle match", "xxabcyy", "abc", 3},
		{"suffix match", "xxabc", "abc", 3},
		{"partial", "axbc", "abc", 1},
		{"scattered no contiguous", "axbxcx", "abc", 1},
		{"case insensitive", "ABCdef", "abc", 3},
		{"no match", "xyz", "abc", 0},
		{"single char", "abc", "b", 1},
		{"repeated", "aababc", "abc", 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := longestContiguousMatch([]byte(tt.haystack), []byte(tt.needle))
			if got != tt.want {
				t.Errorf("longestContiguousMatch(%q, %q) = %d, want %d",
					tt.haystack, tt.needle, got, tt.want)
			}
		})
	}
}

func TestIsBoundary(t *testing.T) {
	t.Parallel()

	for _, b := range []byte{'/', '.', '_', '-'} {
		if !isBoundary(b) {
			t.Errorf("isBoundary(%q) = false, want true", b)
		}
	}
	for _, b := range []byte{'a', 'Z', '0', ' ', '('} {
		if isBoundary(b) {
			t.Errorf("isBoundary(%q) = true, want false", b)
		}
	}
}

func TestCountBoundaryHits(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		path  string
		query string
		want  int
	}{
		{"start of string", "foo/bar", "f", 1},
		{"after slash", "foo/bar", "fb", 2},
		{"after dot", "foo.bar", "fb", 2},
		{"after underscore", "foo_bar", "fb", 2},
		{"no hits", "xxxx", "y", 0},
		{"empty query", "foo", "", 0},
		{"empty path", "", "f", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := countBoundaryHits([]byte(tt.path), []byte(tt.query))
			if got != tt.want {
				t.Errorf("countBoundaryHits(%q, %q) = %d, want %d",
					tt.path, tt.query, got, tt.want)
			}
		})
	}
}

func TestScorePath_NoSubsequenceReturnsZero(t *testing.T) {
	t.Parallel()
	path := []byte("src/internal/handler.go")
	query := []byte("zzz")
	tokens := [][]byte{[]byte("zzz")}
	params := DefaultScoreParams()
	s := scorePath(path, 13, 10, 2, query, tokens, params)
	if s != 0 {
		t.Errorf("expected 0 for no subsequence match, got %f", s)
	}
}

func TestScorePath_ExactBasenameOverPartial(t *testing.T) {
	t.Parallel()
	params := DefaultScoreParams()
	query := []byte("main")
	tokens := [][]byte{query}

	// Exact basename match: "main.go" has basename "main.go".
	// But let's make it simpler: basename IS "main".
	pathExact := []byte("src/main")
	scoreExact := scorePath(pathExact, 4, 4, 1, query, tokens, params)

	// Partial match scattered across path.
	pathPartial := []byte("module/amazing")
	scorePartial := scorePath(pathPartial, 7, 7, 1, query, tokens, params)

	if scoreExact <= scorePartial {
		t.Errorf("exact basename (%f) should score higher than partial (%f)",
			scoreExact, scorePartial)
	}
}

func TestScorePath_BasenamePrefixOverScattered(t *testing.T) {
	t.Parallel()
	params := DefaultScoreParams()
	query := []byte("han")
	tokens := [][]byte{query}

	// Basename prefix: handler.go starts with "han".
	pathPrefix := []byte("src/handler.go")
	scorePrefix := scorePath(pathPrefix, 4, 10, 1, query, tokens, params)

	// Scattered in path, not a basename prefix.
	pathScattered := []byte("has/another/thing")
	scoreScattered := scorePath(pathScattered, 12, 5, 2, query, tokens, params)

	if scorePrefix <= scoreScattered {
		t.Errorf("basename prefix (%f) should score higher than scattered (%f)",
			scorePrefix, scoreScattered)
	}
}

func TestScorePath_ShallowOverDeep(t *testing.T) {
	t.Parallel()
	params := DefaultScoreParams()
	query := []byte("foo")
	tokens := [][]byte{query}

	pathShallow := []byte("src/foo.go")
	scoreShallow := scorePath(pathShallow, 4, 6, 1, query, tokens, params)

	pathDeep := []byte("a/b/c/d/e/foo.go")
	scoreDeep := scorePath(pathDeep, 10, 6, 5, query, tokens, params)

	if scoreShallow <= scoreDeep {
		t.Errorf("shallow path (%f) should score higher than deep (%f)",
			scoreShallow, scoreDeep)
	}
}

func TestScorePath_ShorterOverLongerSameMatch(t *testing.T) {
	t.Parallel()
	params := DefaultScoreParams()
	query := []byte("foo")
	tokens := [][]byte{query}

	pathShort := []byte("x/foo")
	scoreShort := scorePath(pathShort, 2, 3, 1, query, tokens, params)

	pathLong := []byte("x/foo_extremely_long_suffix_name")
	scoreLong := scorePath(pathLong, 2, 29, 1, query, tokens, params)

	if scoreShort <= scoreLong {
		t.Errorf("shorter path (%f) should score higher than longer (%f)",
			scoreShort, scoreLong)
	}
}

func BenchmarkScorePath(b *testing.B) {
	path := []byte("src/internal/coderd/database/queries/workspaces.sql")
	query := []byte("workspace")
	tokens := [][]byte{query}
	params := DefaultScoreParams()
	baseOff, baseLen := extractBasename(path)

	// Verify it's a valid match before benchmarking.
	s := scorePath(path, baseOff, baseLen, 4, query, tokens, params)
	if s == 0 {
		b.Fatal("expected non-zero score for benchmark path")
	}

	b.ResetTimer()
	for b.Loop() {
		scorePath(path, baseOff, baseLen, 4, query, tokens, params)
	}
}
