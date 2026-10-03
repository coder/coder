package filefinder

import "slices"

func toLowerASCII(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b + ('a' - 'A')
	}
	return b
}

func normalizeQuery(q string) string {
	b := make([]byte, 0, len(q))
	prevSpace := true
	for i := 0; i < len(q); i++ {
		c := q[i]
		if c == '\\' {
			c = '/'
		}
		c = toLowerASCII(c)
		if c == ' ' {
			if prevSpace {
				continue
			}
			prevSpace = true
		} else {
			prevSpace = false
		}
		b = append(b, c)
	}
	if len(b) > 0 && b[len(b)-1] == ' ' {
		b = b[:len(b)-1]
	}
	return string(b)
}

func normalizePathBytes(p []byte) []byte {
	j := 0
	prevSlash := false
	for i := 0; i < len(p); i++ {
		c := p[i]
		if c == '\\' {
			c = '/'
		}
		c = toLowerASCII(c)
		if c == '/' {
			if prevSlash {
				continue
			}
			prevSlash = true
		} else {
			prevSlash = false
		}
		p[j] = c
		j++
	}
	return p[:j]
}

// extractTrigrams returns deduplicated, sorted trigrams (three-byte
// subsequences) from s. Trigrams are the primary index key: a
// document matches a query only if every query trigram appears in
// the document, giving O(1) candidate filtering per trigram.
func extractTrigrams(s []byte) []uint32 {
	if len(s) < 3 {
		return nil
	}
	seen := make(map[uint32]struct{}, len(s))
	for i := 0; i <= len(s)-3; i++ {
		b0 := toLowerASCII(s[i])
		b1 := toLowerASCII(s[i+1])
		b2 := toLowerASCII(s[i+2])
		gram := uint32(b0)<<16 | uint32(b1)<<8 | uint32(b2)
		seen[gram] = struct{}{}
	}
	result := make([]uint32, 0, len(seen))
	for g := range seen {
		result = append(result, g)
	}
	slices.Sort(result)
	return result
}

func extractBasename(path []byte) (offset int, length int) {
	end := len(path)
	if end > 0 && path[end-1] == '/' {
		end--
	}
	if end == 0 {
		return 0, 0
	}
	i := end - 1
	for i >= 0 && path[i] != '/' {
		i--
	}
	start := i + 1
	return start, end - start
}

func extractSegments(path []byte) [][]byte {
	var segments [][]byte
	start := 0
	for i := 0; i <= len(path); i++ {
		if i == len(path) || path[i] == '/' {
			if i > start {
				segments = append(segments, path[start:i])
			}
			start = i + 1
		}
	}
	return segments
}

func prefix1(name []byte) byte {
	if len(name) == 0 {
		return 0
	}
	return toLowerASCII(name[0])
}

func prefix2(name []byte) uint16 {
	if len(name) == 0 {
		return 0
	}
	hi := uint16(toLowerASCII(name[0])) << 8
	if len(name) < 2 {
		return hi
	}
	return hi | uint16(toLowerASCII(name[1]))
}

// scoreParams controls the weights for each scoring signal.
type scoreParams struct {
	BasenameMatch  float32
	BasenamePrefix float32
	ExactSegment   float32
	BoundaryHit    float32
	ContiguousRun  float32
	DirTokenHit    float32
	DepthPenalty   float32
	LengthPenalty  float32
}

func defaultScoreParams() scoreParams {
	return scoreParams{
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

// scorePath computes a relevance score for a candidate path
// against a query. The score combines several signals:
// basename match, basename prefix, exact segment match,
// word-boundary hits, longest contiguous run, and penalties
// for depth and length. A return value of 0 means no match
// (the query is not a subsequence of the path).
func scorePath(
	path []byte,
	baseOff int,
	baseLen int,
	depth int,
	query []byte,
	queryTokens [][]byte,
	params scoreParams,
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
