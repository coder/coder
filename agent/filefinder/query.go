package filefinder

import (
	"container/heap"
	"strings"
)

// candidate is a raw search hit before scoring.
type candidate struct {
	docID   uint32
	path    string
	baseOff int
	baseLen int
	depth   int
	flags   uint16
}

// Result is a scored search result returned to callers.
type Result struct {
	Path  string
	Score float32
	IsDir bool
}

// queryPlan holds a pre-processed query for reuse across multiple
// snapshot searches.
type queryPlan struct {
	original   string
	normalized string
	tokens     [][]byte
	trigrams   []uint32
	isShort    bool
	hasSlash   bool
	basenameQ  []byte
	dirTokens  [][]byte
}

// newQueryPlan normalizes the query string and extracts tokens,
// trigrams, and other metadata used by the search functions.
func newQueryPlan(q string) *queryPlan {
	norm := normalizeQuery(q)
	p := &queryPlan{
		original:   q,
		normalized: norm,
	}

	if len(norm) == 0 {
		p.isShort = true
		return p
	}

	// Split on spaces and slashes to produce tokens.
	raw := norm
	raw = strings.ReplaceAll(raw, "/", " ")
	parts := strings.Fields(raw)

	p.hasSlash = strings.ContainsRune(norm, '/')

	for _, part := range parts {
		p.tokens = append(p.tokens, []byte(part))
	}

	// basenameQ is the last token (the basename portion of the
	// query). dirTokens are everything before.
	if len(p.tokens) > 0 {
		p.basenameQ = p.tokens[len(p.tokens)-1]
		if len(p.tokens) > 1 {
			p.dirTokens = p.tokens[:len(p.tokens)-1]
		}
	}

	// Determine if this is a short query (all tokens < 3 chars).
	p.isShort = true
	for _, tok := range p.tokens {
		if len(tok) >= 3 {
			p.isShort = false
			break
		}
	}

	// Extract trigrams from the normalized query for index
	// lookups.
	if !p.isShort {
		p.trigrams = extractQueryTrigrams(p.tokens)
	}

	return p
}

// extractQueryTrigrams extracts unique trigrams from a set of
// query tokens. Each token contributes its own trigrams.
func extractQueryTrigrams(tokens [][]byte) []uint32 {
	seen := make(map[uint32]struct{})
	for _, tok := range tokens {
		if len(tok) < 3 {
			continue
		}
		for i := 0; i <= len(tok)-3; i++ {
			g := packTrigram(tok[i], tok[i+1], tok[i+2])
			seen[g] = struct{}{}
		}
	}
	if len(seen) == 0 {
		return nil
	}
	result := make([]uint32, 0, len(seen))
	for g := range seen {
		result = append(result, g)
	}
	return result
}

// packTrigram packs three bytes into a uint32 trigram key. All
// bytes are lowered for case-insensitive matching.
func packTrigram(a, b, c byte) uint32 {
	return uint32(toLowerASCII(a))<<16 |
		uint32(toLowerASCII(b))<<8 |
		uint32(toLowerASCII(c))
}

// searchSnapshot searches a frozen snapshot for candidates
// matching the query plan. It returns at most limit candidates.
func searchSnapshot(plan *queryPlan, snap *Snapshot, limit int) []candidate {
	if snap == nil || snap.count == 0 || len(plan.normalized) == 0 {
		return nil
	}

	var ids []uint32

	if plan.isShort {
		ids = searchShort(plan, snap)
	} else {
		ids = searchTrigrams(plan, snap)
		// If trigram search returned nothing and we have a
		// basename query, try the fuzzy fallback path.
		if len(ids) == 0 && len(plan.basenameQ) > 0 {
			ids = searchFuzzyFallback(plan, snap)
		}
	}

	if len(ids) == 0 {
		return nil
	}

	// Build candidates, skipping deleted docs.
	cands := make([]candidate, 0, min(len(ids), limit))
	for _, id := range ids {
		if snap.deleted[id] {
			continue
		}
		if int(id) >= len(snap.docs) {
			continue
		}
		d := snap.docs[id]
		cands = append(cands, candidate{
			docID:   id,
			path:    d.path,
			baseOff: d.baseOff,
			baseLen: d.baseLen,
			depth:   d.depth,
			flags:   d.flags,
		})
		if len(cands) >= limit {
			break
		}
	}

	return cands
}

// searchShort handles queries shorter than 3 characters by using
// the prefix indexes rather than trigrams.
func searchShort(plan *queryPlan, snap *Snapshot) []uint32 {
	if len(plan.basenameQ) == 0 {
		return nil
	}

	// Try 2-byte prefix first for better selectivity.
	if len(plan.basenameQ) >= 2 {
		p2 := prefix2(plan.basenameQ)
		if ids := snap.byPrefix2[p2]; len(ids) > 0 {
			return ids
		}
	}

	// Fall back to 1-byte prefix.
	p1 := prefix1(plan.basenameQ)
	return snap.byPrefix1[p1]
}

// searchTrigrams intersects trigram posting lists to find
// candidate doc IDs.
func searchTrigrams(plan *queryPlan, snap *Snapshot) []uint32 {
	if len(plan.trigrams) == 0 {
		return nil
	}

	lists := make([][]uint32, 0, len(plan.trigrams))
	for _, g := range plan.trigrams {
		ids, ok := snap.byGram[g]
		if !ok || len(ids) == 0 {
			// A required trigram has no matches, so the
			// intersection is empty.
			return nil
		}
		lists = append(lists, ids)
	}

	return intersectAll(lists)
}

// searchFuzzyFallback attempts a looser search when trigrams fail.
// It scans the prefix1 bucket for the first character of the
// basename query and checks for a subsequence match.
func searchFuzzyFallback(plan *queryPlan, snap *Snapshot) []uint32 {
	if len(plan.basenameQ) == 0 {
		return nil
	}

	p1 := prefix1(plan.basenameQ)
	bucket := snap.byPrefix1[p1]
	if len(bucket) == 0 {
		// No entries start with this character. Fall through
		// to brute-force subsequence scan.
		return searchSubsequenceScan(plan, snap, 5000)
	}

	var ids []uint32
	for _, id := range bucket {
		if snap.deleted[id] {
			continue
		}
		if int(id) >= len(snap.docs) {
			continue
		}
		d := snap.docs[id]
		if isSubsequence([]byte(d.path), plan.basenameQ) {
			ids = append(ids, id)
		}
	}

	if len(ids) == 0 {
		return searchSubsequenceScan(plan, snap, 5000)
	}
	return ids
}

// searchSubsequenceScan performs a brute-force subsequence check
// over the entire snapshot. It checks at most maxCheck documents
// to bound work.
func searchSubsequenceScan(plan *queryPlan, snap *Snapshot, maxCheck int) []uint32 {
	if len(plan.basenameQ) == 0 {
		return nil
	}

	var ids []uint32
	checked := 0
	for id := 0; id < snap.count && checked < maxCheck; id++ {
		uid := uint32(id)
		if snap.deleted[uid] {
			continue
		}
		checked++
		d := snap.docs[id]
		if isSubsequence([]byte(d.path), plan.basenameQ) {
			ids = append(ids, uid)
		}
	}
	return ids
}

// intersectSorted returns the sorted intersection of two sorted
// uint32 slices.
func intersectSorted(a, b []uint32) []uint32 {
	if len(a) == 0 || len(b) == 0 {
		return nil
	}
	var result []uint32
	ai, bi := 0, 0
	for ai < len(a) && bi < len(b) {
		switch {
		case a[ai] < b[bi]:
			ai++
		case a[ai] > b[bi]:
			bi++
		default:
			result = append(result, a[ai])
			ai++
			bi++
		}
	}
	return result
}

// intersectAll intersects multiple sorted posting lists. It sorts
// the lists by length first (shortest first) for efficiency.
func intersectAll(lists [][]uint32) []uint32 {
	if len(lists) == 0 {
		return nil
	}
	if len(lists) == 1 {
		return lists[0]
	}

	sortByLen(lists)

	result := lists[0]
	for i := 1; i < len(lists) && len(result) > 0; i++ {
		result = intersectSorted(result, lists[i])
	}
	return result
}

// sortByLen performs an insertion sort on lists by their length
// (shortest first). Insertion sort is ideal here because the
// number of trigram lists is typically very small (< 20).
func sortByLen(lists [][]uint32) {
	for i := 1; i < len(lists); i++ {
		key := lists[i]
		j := i - 1
		for j >= 0 && len(lists[j]) > len(key) {
			lists[j+1] = lists[j]
			j--
		}
		lists[j+1] = key
	}
}

// mergeAndScore scores candidates and returns the top-K results
// sorted by score descending. It uses a min-heap to efficiently
// keep only the best results.
func mergeAndScore(cands []candidate, plan *queryPlan, params ScoreParams, topK int) []Result {
	if topK <= 0 || len(cands) == 0 {
		return nil
	}

	query := []byte(plan.normalized)
	h := &resultHeap{}
	heap.Init(h)

	for i := range cands {
		c := &cands[i]
		s := scorePath(
			[]byte(c.path),
			c.baseOff,
			c.baseLen,
			c.depth,
			query,
			plan.tokens,
			params,
		)
		if s <= 0 {
			continue
		}

		// Add dir-token bonus.
		if len(plan.dirTokens) > 0 {
			segments := extractSegments([]byte(c.path))
			for _, dt := range plan.dirTokens {
				for _, seg := range segments {
					if equalFoldASCII(seg, dt) {
						s += params.DirTokenHit
						break
					}
				}
			}
		}

		sr := scoredResult{
			Path:  c.path,
			Score: s,
			IsDir: c.flags == uint16(FlagDir),
		}

		if h.Len() < topK {
			heap.Push(h, sr)
		} else if s > (*h)[0].Score {
			(*h)[0] = sr
			heap.Fix(h, 0)
		}
	}

	// Extract results in descending score order.
	n := h.Len()
	results := make([]Result, n)
	for i := n - 1; i >= 0; i-- {
		sr := heap.Pop(h).(scoredResult)
		results[i] = Result{
			Path:  sr.Path,
			Score: sr.Score,
			IsDir: sr.IsDir,
		}
	}
	return results
}

// scoredResult pairs a result with its score for heap operations.
type scoredResult struct {
	Path  string
	Score float32
	IsDir bool
}

// resultHeap is a min-heap of scoredResult ordered by Score. The
// minimum-score element sits at index 0 so we can efficiently
// evict the weakest candidate when the heap is full.
type resultHeap []scoredResult

func (h resultHeap) Len() int            { return len(h) }
func (h resultHeap) Less(i, j int) bool   { return h[i].Score < h[j].Score }
func (h resultHeap) Swap(i, j int)        { h[i], h[j] = h[j], h[i] }
func (h *resultHeap) Push(x interface{}) { *h = append(*h, x.(scoredResult)) }
func (h *resultHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}
