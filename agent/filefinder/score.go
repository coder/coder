package filefinder

// ScoreParams controls scoring weights.
type ScoreParams struct {
	BasenameMatch  float32 // huge bonus for basename subsequence match
	BasenamePrefix float32 // big bonus for basename prefix match
	ExactSegment   float32 // bonus for exact directory segment match
	BoundaryHit    float32 // bonus for match starting at word boundary (after /, ., _, -)
	ContiguousRun  float32 // bonus for contiguous character runs in match
	DirTokenHit    float32 // small bonus for directory token match
	DepthPenalty   float32 // penalty per depth level
	LengthPenalty  float32 // penalty per path character
}

// DefaultScoreParams returns tuned defaults.
func DefaultScoreParams() ScoreParams {
	return ScoreParams{
		BasenameMatch:  6.0,
		BasenamePrefix: 3.5,
		ExactSegment:   2.5,
		BoundaryHit:    1.8,
		ContiguousRun:  1.2,
		DirTokenHit:    0.4,
		DepthPenalty:   0.08,
		LengthPenalty:  0.01,
	}
}

// isSubsequence reports whether needle is a case-insensitive ASCII
// subsequence of haystack.
func isSubsequence(haystack, needle []byte) bool {
	if len(needle) == 0 {
		return true
	}
	ni := 0
	for _, hb := range haystack {
		if toLowerASCII(hb) == toLowerASCII(needle[ni]) {
			ni++
			if ni == len(needle) {
				return true
			}
		}
	}
	return false
}

// longestContiguousMatch returns the length of the longest run
// of consecutive needle characters that appear contiguously in
// haystack (case-insensitive ASCII). O(len(haystack)).
func longestContiguousMatch(haystack, needle []byte) int {
	if len(needle) == 0 || len(haystack) == 0 {
		return 0
	}
	best := 0
	ni := 0
	run := 0
	for _, hb := range haystack {
		if ni < len(needle) && toLowerASCII(hb) == toLowerASCII(needle[ni]) {
			run++
			ni++
			if run > best {
				best = run
			}
		} else {
			run = 0
			// Reset needle position: try matching from start
			// of needle at this haystack position.
			ni = 0
			if ni < len(needle) && toLowerASCII(hb) == toLowerASCII(needle[ni]) {
				run = 1
				ni = 1
				if run > best {
					best = run
				}
			}
		}
	}
	return best
}

// isBoundary reports whether b is a word boundary character.
func isBoundary(b byte) bool {
	return b == '/' || b == '.' || b == '_' || b == '-'
}

// countBoundaryHits counts how many characters in query match at
// boundary positions in path (right after a boundary character or
// at position 0). Case-insensitive ASCII.
func countBoundaryHits(path []byte, query []byte) int {
	if len(query) == 0 || len(path) == 0 {
		return 0
	}
	hits := 0
	qi := 0
	for pi := 0; pi < len(path) && qi < len(query); pi++ {
		atBoundary := pi == 0 || isBoundary(path[pi-1])
		if atBoundary && toLowerASCII(path[pi]) == toLowerASCII(query[qi]) {
			hits++
			qi++
		}
	}
	return hits
}

// equalFoldASCII reports whether a and b are equal under
// case-insensitive ASCII comparison.
func equalFoldASCII(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if toLowerASCII(a[i]) != toLowerASCII(b[i]) {
			return false
		}
	}
	return true
}

// hasPrefixFoldASCII reports whether haystack starts with prefix
// under case-insensitive ASCII comparison.
func hasPrefixFoldASCII(haystack, prefix []byte) bool {
	if len(prefix) > len(haystack) {
		return false
	}
	for i := range prefix {
		if toLowerASCII(haystack[i]) != toLowerASCII(prefix[i]) {
			return false
		}
	}
	return true
}

// scorePath scores a single candidate path against a query. It
// returns 0 if there is no subsequence match at all in the full
// path, meaning the candidate should be dropped.
//
// Parameters:
//   - path:        full path bytes
//   - baseOff:     offset of basename within path
//   - baseLen:     length of basename
//   - depth:       directory depth of the path
//   - query:       the raw query bytes (lowered)
//   - queryTokens: pre-split query tokens
//   - params:      scoring weights
func scorePath(
	path []byte,
	baseOff int,
	baseLen int,
	depth int,
	query []byte,
	queryTokens [][]byte,
	params ScoreParams,
) float32 {
	// No match at all in the full path: reject.
	if !isSubsequence(path, query) {
		return 0
	}

	var score float32

	basename := path[baseOff : baseOff+baseLen]

	// 1. Basename subsequence match bonus.
	if isSubsequence(basename, query) {
		score += params.BasenameMatch
	}

	// 2. Basename prefix match bonus.
	if hasPrefixFoldASCII(basename, query) {
		score += params.BasenamePrefix
	}

	// 3. Exact segment matches for query tokens.
	segments := extractSegments(path)
	for _, token := range queryTokens {
		for _, seg := range segments {
			if equalFoldASCII(seg, token) {
				score += params.ExactSegment
				break
			}
		}
	}

	// 4. Boundary hits.
	bh := countBoundaryHits(path, query)
	score += float32(bh) * params.BoundaryHit

	// 5. Contiguous run bonus scaled by run length.
	lcm := longestContiguousMatch(path, query)
	score += float32(lcm) * params.ContiguousRun

	// 6. Depth penalty.
	score -= float32(depth) * params.DepthPenalty

	// 7. Length penalty.
	score -= float32(len(path)) * params.LengthPenalty

	return score
}
