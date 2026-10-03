package filefinder

import (
	"container/heap"
	"slices"
	"strings"
)

type candidate struct {
	DocID   uint32
	Path    string
	BaseOff int
	BaseLen int
	Depth   int
	Flags   uint16
}

// Result is a scored search result returned to callers.
type Result struct {
	Path  string
	Score float32
	IsDir bool
}

type queryPlan struct {
	Original   string
	Normalized string
	Tokens     [][]byte
	Trigrams   []uint32
	IsShort    bool
	HasSlash   bool
	BasenameQ  []byte
	DirTokens  [][]byte
}

func newQueryPlan(q string) *queryPlan {
	norm := normalizeQuery(q)
	p := &queryPlan{Original: q, Normalized: norm}
	if len(norm) == 0 {
		p.IsShort = true
		return p
	}
	raw := strings.ReplaceAll(norm, "/", " ")
	parts := strings.Fields(raw)
	p.HasSlash = strings.ContainsRune(norm, '/')
	for _, part := range parts {
		p.Tokens = append(p.Tokens, []byte(part))
	}
	if len(p.Tokens) > 0 {
		p.BasenameQ = p.Tokens[len(p.Tokens)-1]
		if len(p.Tokens) > 1 {
			p.DirTokens = p.Tokens[:len(p.Tokens)-1]
		}
	}
	p.IsShort = true
	for _, tok := range p.Tokens {
		if len(tok) >= 3 {
			p.IsShort = false
			break
		}
	}
	if !p.IsShort {
		p.Trigrams = extractQueryTrigrams(p.Tokens)
	}
	return p
}

func extractQueryTrigrams(tokens [][]byte) []uint32 {
	seen := make(map[uint32]struct{})
	for _, tok := range tokens {
		if len(tok) < 3 {
			continue
		}
		for i := 0; i <= len(tok)-3; i++ {
			seen[packTrigram(tok[i], tok[i+1], tok[i+2])] = struct{}{}
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

func packTrigram(a, b, c byte) uint32 {
	return uint32(toLowerASCII(a))<<16 | uint32(toLowerASCII(b))<<8 | uint32(toLowerASCII(c))
}

// searchSnapshot runs the full search pipeline against a single
// root snapshot: it selects a strategy (prefix, trigram, or
// fuzzy fallback) based on query length, retrieves candidate
// doc IDs, and converts them into candidate structs.
func searchSnapshot(plan *queryPlan, snap *Snapshot, limit int) []candidate {
	if snap == nil || len(snap.docs) == 0 || len(plan.Normalized) == 0 {
		return nil
	}
	var ids []uint32
	if plan.IsShort {
		ids = searchShort(plan, snap)
	} else {
		ids = searchTrigrams(plan, snap)
		if len(ids) == 0 && len(plan.BasenameQ) > 0 {
			ids = searchFuzzyFallback(plan, snap)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	cands := make([]candidate, 0, min(len(ids), limit))
	for _, id := range ids {
		if snap.deleted[id] || int(id) >= len(snap.docs) {
			continue
		}
		d := snap.docs[id]
		cands = append(cands, candidate{
			DocID: id, Path: d.path, BaseOff: d.baseOff,
			BaseLen: d.baseLen, Depth: d.depth, Flags: d.flags,
		})
		if len(cands) >= limit {
			break
		}
	}
	return cands
}

func searchShort(plan *queryPlan, snap *Snapshot) []uint32 {
	if len(plan.BasenameQ) == 0 {
		return nil
	}
	if len(plan.BasenameQ) >= 2 {
		if ids := snap.byPrefix2[prefix2(plan.BasenameQ)]; len(ids) > 0 {
			return ids
		}
	}
	return snap.byPrefix1[prefix1(plan.BasenameQ)]
}

func searchTrigrams(plan *queryPlan, snap *Snapshot) []uint32 {
	if len(plan.Trigrams) == 0 {
		return nil
	}
	lists := make([][]uint32, 0, len(plan.Trigrams))
	for _, g := range plan.Trigrams {
		ids, ok := snap.byGram[g]
		if !ok || len(ids) == 0 {
			return nil
		}
		lists = append(lists, ids)
	}
	return intersectAll(lists)
}

func searchFuzzyFallback(plan *queryPlan, snap *Snapshot) []uint32 {
	if len(plan.BasenameQ) == 0 {
		return nil
	}
	bucket := snap.byPrefix1[prefix1(plan.BasenameQ)]
	if len(bucket) == 0 {
		return searchSubsequenceScan(plan, snap, 5000)
	}
	var ids []uint32
	for _, id := range bucket {
		if snap.deleted[id] || int(id) >= len(snap.docs) {
			continue
		}
		if isSubsequence([]byte(snap.docs[id].path), plan.BasenameQ) {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return searchSubsequenceScan(plan, snap, 5000)
	}
	return ids
}

func searchSubsequenceScan(plan *queryPlan, snap *Snapshot, maxCheck int) []uint32 {
	if len(plan.BasenameQ) == 0 {
		return nil
	}
	var ids []uint32
	checked := 0
	for id := 0; id < len(snap.docs) && checked < maxCheck; id++ {
		uid := uint32(id) //nolint:gosec // Snapshot count is bounded well below 2^32.
		if snap.deleted[uid] {
			continue
		}
		checked++
		if isSubsequence([]byte(snap.docs[id].path), plan.BasenameQ) {
			ids = append(ids, uid)
		}
	}
	return ids
}

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

func intersectAll(lists [][]uint32) []uint32 {
	if len(lists) == 0 {
		return nil
	}
	if len(lists) == 1 {
		return lists[0]
	}
	slices.SortFunc(lists, func(a, b []uint32) int { return len(a) - len(b) })
	result := lists[0]
	for i := 1; i < len(lists) && len(result) > 0; i++ {
		result = intersectSorted(result, lists[i])
	}
	return result
}

func mergeAndScore(cands []candidate, plan *queryPlan, params scoreParams, topK int) []Result {
	if topK <= 0 || len(cands) == 0 {
		return nil
	}
	query := []byte(plan.Normalized)
	h := &resultHeap{}
	heap.Init(h)
	for i := range cands {
		c := &cands[i]
		s := scorePath([]byte(c.Path), c.BaseOff, c.BaseLen, c.Depth, query, plan.Tokens, params)
		if s <= 0 {
			continue
		}
		// DirTokenHit is applied here rather than in scorePath because
		// it depends on the query plan's directory tokens, which are
		// split from the full query during planning. scorePath operates
		// on raw query bytes without knowledge of token boundaries.
		if len(plan.DirTokens) > 0 {
			segments := extractSegments([]byte(c.Path))
			for _, dt := range plan.DirTokens {
				for _, seg := range segments {
					if equalFoldASCII(seg, dt) {
						s += params.DirTokenHit
						break
					}
				}
			}
		}
		r := Result{Path: c.Path, Score: s, IsDir: c.Flags == uint16(FlagDir)}
		if h.Len() < topK {
			heap.Push(h, r)
		} else if s > (*h)[0].Score {
			(*h)[0] = r
			heap.Fix(h, 0)
		}
	}
	n := h.Len()
	results := make([]Result, n)
	for i := n - 1; i >= 0; i-- {
		v := heap.Pop(h)
		if r, ok := v.(Result); ok {
			results[i] = r
		}
	}
	return results
}

type resultHeap []Result

func (h resultHeap) Len() int           { return len(h) }
func (h resultHeap) Less(i, j int) bool { return h[i].Score < h[j].Score }
func (h resultHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *resultHeap) Push(x interface{}) {
	r, ok := x.(Result)
	if ok {
		*h = append(*h, r)
	}
}

func (h *resultHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}
