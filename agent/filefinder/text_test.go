package filefinder_test

import (
	"slices"
	"testing"

	"github.com/coder/coder/v2/agent/filefinder"
)

func TestNormalizeQuery(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "", ""},
		{"leading and trailing spaces", "  hello  ", "hello"},
		{"multiple internal spaces", "foo   bar   baz", "foo bar baz"},
		{"uppercase to lower", "FooBar", "foobar"},
		{"backslash to slash", `foo\bar\baz`, "foo/bar/baz"},
		{"mixed case and spaces", "  Hello   World  ", "hello world"},
		{"unicode passthrough", "héllo wörld", "héllo wörld"},
		{"only spaces", "     ", ""},
		{"single char", "A", "a"},
		{"slashes preserved", "/foo/bar/", "/foo/bar/"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := filefinder.NormalizeQueryForTest(tt.input)
			if got != tt.want {
				t.Errorf("normalizeQuery(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestExtractTrigrams(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  []uint32
	}{
		{"too short", "ab", nil},
		{"exactly three bytes", "abc", []uint32{uint32('a')<<16 | uint32('b')<<8 | uint32('c')}},
		{"case insensitive", "ABC", []uint32{uint32('a')<<16 | uint32('b')<<8 | uint32('c')}},
		{"deduplication", "aaaa", []uint32{uint32('a')<<16 | uint32('a')<<8 | uint32('a')}},
		{"four bytes produces two trigrams", "abcd", []uint32{
			uint32('a')<<16 | uint32('b')<<8 | uint32('c'),
			uint32('b')<<16 | uint32('c')<<8 | uint32('d'),
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := filefinder.ExtractTrigramsForTest([]byte(tt.input))
			if !slices.Equal(got, tt.want) {
				t.Errorf("extractTrigrams(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestExtractBasename(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		path     string
		wantOff  int
		wantName string
	}{
		{"full path", "/foo/bar/baz.go", 9, "baz.go"},
		{"bare filename", "baz.go", 0, "baz.go"},
		{"trailing slash", "/a/b/", 3, "b"},
		{"root slash", "/", 0, ""},
		{"empty", "", 0, ""},
		{"single dir with slash", "/foo", 1, "foo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			off, length := filefinder.ExtractBasenameForTest([]byte(tt.path))
			if off != tt.wantOff {
				t.Errorf("extractBasename(%q) offset = %d, want %d", tt.path, off, tt.wantOff)
			}
			gotName := string([]byte(tt.path)[off : off+length])
			if gotName != tt.wantName {
				t.Errorf("extractBasename(%q) name = %q, want %q", tt.path, gotName, tt.wantName)
			}
		})
	}
}

func TestExtractSegments(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		path string
		want []string
	}{
		{"absolute path", "/foo/bar/baz", []string{"foo", "bar", "baz"}},
		{"relative path", "foo/bar", []string{"foo", "bar"}},
		{"trailing slash", "/a/b/", []string{"a", "b"}},
		{"multiple slashes", "//a///b//", []string{"a", "b"}},
		{"empty", "", nil},
		{"single segment", "foo", []string{"foo"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := filefinder.ExtractSegmentsForTest([]byte(tt.path))
			if len(got) != len(tt.want) {
				t.Fatalf("extractSegments(%q) got %d segments, want %d", tt.path, len(got), len(tt.want))
			}
			for i := range got {
				if string(got[i]) != tt.want[i] {
					t.Errorf("extractSegments(%q)[%d] = %q, want %q", tt.path, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestPrefix1(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want byte
	}{
		{"lowercase", "foo", 'f'},
		{"uppercase", "Foo", 'f'},
		{"empty", "", 0},
		{"digit", "1abc", '1'},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := filefinder.Prefix1ForTest([]byte(tt.in))
			if got != tt.want {
				t.Errorf("prefix1(%q) = %d (%c), want %d (%c)", tt.in, got, got, tt.want, tt.want)
			}
		})
	}
}

func TestPrefix2(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want uint16
	}{
		{"two chars", "ab", uint16('a')<<8 | uint16('b')},
		{"uppercase", "AB", uint16('a')<<8 | uint16('b')},
		{"single char", "A", uint16('a') << 8},
		{"empty", "", 0},
		{"longer string", "Hello", uint16('h')<<8 | uint16('e')},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := filefinder.Prefix2ForTest([]byte(tt.in))
			if got != tt.want {
				t.Errorf("prefix2(%q) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestNormalizePathBytes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"backslash to slash", `C:\Users\test`, "c:/users/test"},
		{"collapse slashes", "//foo///bar//", "/foo/bar/"},
		{"lowercase", "FooBar", "foobar"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			buf := []byte(tt.input)
			got := string(filefinder.NormalizePathBytesForTest(buf))
			if got != tt.want {
				t.Errorf("normalizePathBytes(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

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
			got := filefinder.IsSubsequenceForTest([]byte(tt.haystack), []byte(tt.needle))
			if got != tt.want {
				t.Errorf("isSubsequence(%q, %q) = %v, want %v", tt.haystack, tt.needle, got, tt.want)
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
			got := filefinder.LongestContiguousMatchForTest([]byte(tt.haystack), []byte(tt.needle))
			if got != tt.want {
				t.Errorf("longestContiguousMatch(%q, %q) = %d, want %d", tt.haystack, tt.needle, got, tt.want)
			}
		})
	}
}

func TestIsBoundary(t *testing.T) {
	t.Parallel()
	for _, b := range []byte{'/', '.', '_', '-'} {
		if !filefinder.IsBoundaryForTest(b) {
			t.Errorf("isBoundary(%q) = false, want true", b)
		}
	}
	for _, b := range []byte{'a', 'Z', '0', ' ', '('} {
		if filefinder.IsBoundaryForTest(b) {
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
			got := filefinder.CountBoundaryHitsForTest([]byte(tt.path), []byte(tt.query))
			if got != tt.want {
				t.Errorf("countBoundaryHits(%q, %q) = %d, want %d", tt.path, tt.query, got, tt.want)
			}
		})
	}
}

func TestScorePath_NoSubsequenceReturnsZero(t *testing.T) {
	t.Parallel()
	path := []byte("src/internal/handler.go")
	query := []byte("zzz")
	tokens := [][]byte{[]byte("zzz")}
	params := filefinder.DefaultScoreParamsForTest()
	s := filefinder.ScorePathForTest(path, 13, 10, 2, query, tokens, params)
	if s != 0 {
		t.Errorf("expected 0 for no subsequence match, got %f", s)
	}
}

func TestScorePath_ExactBasenameOverPartial(t *testing.T) {
	t.Parallel()
	params := filefinder.DefaultScoreParamsForTest()
	query := []byte("main")
	tokens := [][]byte{query}
	pathExact := []byte("src/main")
	scoreExact := filefinder.ScorePathForTest(pathExact, 4, 4, 1, query, tokens, params)
	pathPartial := []byte("module/amazing")
	scorePartial := filefinder.ScorePathForTest(pathPartial, 7, 7, 1, query, tokens, params)
	if scoreExact <= scorePartial {
		t.Errorf("exact basename (%f) should score higher than partial (%f)", scoreExact, scorePartial)
	}
}

func TestScorePath_BasenamePrefixOverScattered(t *testing.T) {
	t.Parallel()
	params := filefinder.DefaultScoreParamsForTest()
	query := []byte("han")
	tokens := [][]byte{query}
	pathPrefix := []byte("src/handler.go")
	scorePrefix := filefinder.ScorePathForTest(pathPrefix, 4, 10, 1, query, tokens, params)
	pathScattered := []byte("has/another/thing")
	scoreScattered := filefinder.ScorePathForTest(pathScattered, 12, 5, 2, query, tokens, params)
	if scorePrefix <= scoreScattered {
		t.Errorf("basename prefix (%f) should score higher than scattered (%f)", scorePrefix, scoreScattered)
	}
}

func TestScorePath_ShallowOverDeep(t *testing.T) {
	t.Parallel()
	params := filefinder.DefaultScoreParamsForTest()
	query := []byte("foo")
	tokens := [][]byte{query}
	pathShallow := []byte("src/foo.go")
	scoreShallow := filefinder.ScorePathForTest(pathShallow, 4, 6, 1, query, tokens, params)
	pathDeep := []byte("a/b/c/d/e/foo.go")
	scoreDeep := filefinder.ScorePathForTest(pathDeep, 10, 6, 5, query, tokens, params)
	if scoreShallow <= scoreDeep {
		t.Errorf("shallow path (%f) should score higher than deep (%f)", scoreShallow, scoreDeep)
	}
}

func TestScorePath_ShorterOverLongerSameMatch(t *testing.T) {
	t.Parallel()
	params := filefinder.DefaultScoreParamsForTest()
	query := []byte("foo")
	tokens := [][]byte{query}
	pathShort := []byte("x/foo")
	scoreShort := filefinder.ScorePathForTest(pathShort, 2, 3, 1, query, tokens, params)
	pathLong := []byte("x/foo_extremely_long_suffix_name")
	scoreLong := filefinder.ScorePathForTest(pathLong, 2, 29, 1, query, tokens, params)
	if scoreShort <= scoreLong {
		t.Errorf("shorter path (%f) should score higher than longer (%f)", scoreShort, scoreLong)
	}
}

func BenchmarkScorePath(b *testing.B) {
	path := []byte("src/internal/coderd/database/queries/workspaces.sql")
	query := []byte("workspace")
	tokens := [][]byte{query}
	params := filefinder.DefaultScoreParamsForTest()
	baseOff, baseLen := filefinder.ExtractBasenameForTest(path)
	s := filefinder.ScorePathForTest(path, baseOff, baseLen, 4, query, tokens, params)
	if s == 0 {
		b.Fatal("expected non-zero score for benchmark path")
	}
	b.ResetTimer()
	for b.Loop() {
		filefinder.ScorePathForTest(path, baseOff, baseLen, 4, query, tokens, params)
	}
}
