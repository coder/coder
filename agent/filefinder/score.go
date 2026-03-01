package filefinder

type ScoreParams struct {
	BasenameMatch  float32
	BasenamePrefix float32
	ExactSegment   float32
	BoundaryHit    float32
	ContiguousRun  float32
	DirTokenHit    float32
	DepthPenalty   float32
	LengthPenalty  float32
}

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

func isBoundary(b byte) bool {
	return b == '/' || b == '.' || b == '_' || b == '-'
}

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

func scorePath(
	path []byte,
	baseOff int,
	baseLen int,
	depth int,
	query []byte,
	queryTokens [][]byte,
	params ScoreParams,
) float32 {
	if !isSubsequence(path, query) {
		return 0
	}

	var score float32

	basename := path[baseOff : baseOff+baseLen]

	if isSubsequence(basename, query) {
		score += params.BasenameMatch
	}

	if hasPrefixFoldASCII(basename, query) {
		score += params.BasenamePrefix
	}

	segments := extractSegments(path)
	for _, token := range queryTokens {
		for _, seg := range segments {
			if equalFoldASCII(seg, token) {
				score += params.ExactSegment
				break
			}
		}
	}

	bh := countBoundaryHits(path, query)
	score += float32(bh) * params.BoundaryHit

	lcm := longestContiguousMatch(path, query)
	score += float32(lcm) * params.ContiguousRun

	score -= float32(depth) * params.DepthPenalty
	score -= float32(len(path)) * params.LengthPenalty

	return score
}
