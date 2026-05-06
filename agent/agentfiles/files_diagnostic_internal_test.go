package agentfiles

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestBuildNotFoundDetail verifies that buildNotFoundDetail returns
// useful diagnostic strings when a fuzzyReplace search fails.
func TestBuildNotFoundDetail(t *testing.T) {
	t.Parallel()

	t.Run("NoLineFoundAnywhere", func(t *testing.T) {
		t.Parallel()
		// Content and search have no line in common; even trimSpace-equal
		// lines are absent. Use raw slices to avoid empty-string artifacts
		// from strings.Split on newline-terminated strings.
		content := []string{"line1\n", "line2\n", "line3\n"}
		search := []string{"zzz_not_in_file\n", "yyy_not_in_file\n"}
		detail := buildNotFoundDetail(content, search)
		require.Contains(t, detail, "No line of the search string was found anywhere in the file")
	})

	t.Run("PartialMatchReported", func(t *testing.T) {
		t.Parallel()
		// File has 4 lines; search matches 3 of them but the 4th differs.
		content := []string{"a\n", "b\n", "c\n", "d\n"}
		search := []string{"a\n", "b\n", "c\n", "X\n"}
		detail := buildNotFoundDetail(content, search)
		require.NotEmpty(t, detail)
		// The partial match scanner should find lines 1-4 as the best
		// window (3/4 matched) and report the first difference.
		require.Contains(t, detail, "Closest match")
	})

	t.Run("EmptySearch_ReturnsEmpty", func(t *testing.T) {
		t.Parallel()
		content := strings.Split("a\nb\n", "\n")
		detail := buildNotFoundDetail(content, nil)
		require.Empty(t, detail)
	})

	t.Run("SearchLongerThanMaxDiagnostic_ReturnsEmpty", func(t *testing.T) {
		t.Parallel()
		// Build a search longer than maxSearchLinesForDiagnostic.
		lines := make([]string, maxSearchLinesForDiagnostic+1)
		for i := range lines {
			lines[i] = "line"
		}
		detail := buildNotFoundDetail([]string{"a", "b"}, lines)
		require.Empty(t, detail)
	})
}

// TestFindBestPartialMatch verifies the partial-match scanner returns
// the window with the highest number of matching lines.
func TestFindBestPartialMatch(t *testing.T) {
	t.Parallel()

	t.Run("PerfectMatchAllLines", func(t *testing.T) {
		t.Parallel()
		content := []string{"a\n", "b\n", "c\n"}
		search := []string{"a\n", "b\n", "c\n"}
		pm := findBestPartialMatch(content, search)
		require.Equal(t, 3, pm.matched)
		require.Equal(t, -1, pm.diffIdx, "all lines match; no diff index")
	})

	t.Run("PartialMatchPicksBestWindow", func(t *testing.T) {
		t.Parallel()
		// content: a b c d e
		// search:  a b X
		// Best window is [0,2]: 2 of 3 match.
		content := []string{"a\n", "b\n", "c\n", "d\n", "e\n"}
		search := []string{"a\n", "b\n", "X\n"}
		pm := findBestPartialMatch(content, search)
		require.Equal(t, 2, pm.matched)
		require.Equal(t, 0, pm.fileLineStart)
		require.Equal(t, 2, pm.diffIdx)
	})

	t.Run("EmptySearch_ReturnsZero", func(t *testing.T) {
		t.Parallel()
		pm := findBestPartialMatch([]string{"a"}, nil)
		require.Equal(t, 0, pm.matched)
	})

	t.Run("SearchLongerThanContent_ReturnsZero", func(t *testing.T) {
		t.Parallel()
		pm := findBestPartialMatch([]string{"a"}, []string{"a", "b", "c"})
		require.Equal(t, 0, pm.matched)
	})
}

// TestUnicodeCountHint verifies that unicodeCountHint produces a
// human-readable description of Unicode codepoint differences.
func TestUnicodeCountHint(t *testing.T) {
	t.Parallel()

	t.Run("BoxDrawingCountDiffers", func(t *testing.T) {
		t.Parallel()
		// Simulate the exact failure that triggered the doctrine in bb8dc3b9:
		// search had 32 U+2500 (box-drawing dash), file had 37.
		search := strings.Repeat("\u2500", 32) // 32 box-drawing chars
		file := strings.Repeat("\u2500", 37)   // 37 box-drawing chars
		hint := unicodeCountHint(search, file)
		require.Contains(t, hint, "U+2500")
		require.Contains(t, hint, "32")
		require.Contains(t, hint, "37")
	})

	t.Run("NoUnicode_FallsBackToLength", func(t *testing.T) {
		t.Parallel()
		hint := unicodeCountHint("abc", "abcde")
		require.NotEmpty(t, hint)
		// Should report length difference since there are no non-ASCII chars.
	})

	t.Run("SameContent_StillReportsBytes", func(t *testing.T) {
		t.Parallel()
		hint := unicodeCountHint("abc", "abc")
		require.NotEmpty(t, hint)
	})
}
