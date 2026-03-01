package filefinder

import (
	"container/heap"
	"slices"
	"strings"
)

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

func newQueryPlan(q string) *queryPlan {
	norm := normalizeQuery(q)
	p := &queryPlan{original: q, normalized: norm}
	if len(norm) == 0 {
		p.isShort = true
		return p
	}
	raw := strings.ReplaceAll(norm, "/", " ")
	parts := strings.Fields(raw)
	p.hasSlash = strings.ContainsRune(norm, '/')
	for _, part := range parts {
		p.tokens = append(p.tokens, []byte(part))
	}
	if len(p.tokens) > 0 {
		p.basenameQ = p.tokens[len(p.tokens)-1]
		if len(p.tokens) > 1 {
			p.dirTokens = p.tokens[:len(p.tokens)-1]
		}
	}
	p.isShort = true
	for _, tok := range p.tokens {
		if len(tok) >= 3 {
			p.isShort = false
			break
		}
	}
	if !p.isShort {
		p.trigrams = extractQueryTrigrams(p.tokens)
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
func searchSnapshot(plan *queryPlan, snap *Snapshot, limit int) []candidate {
	if snap == nil || snap.count == 0 || len(plan.normalized) == 0 {
		return nil
	}
	var ids []uint32
	if plan.isShort {
		ids = searchShort(plan, snap)
	} else {
		ids = searchTrigrams(plan, snap)
		if len(ids) == 0 && len(plan.basenameQ) > 0 {
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
			docID: id, path: d.path, baseOff: d.baseOff,
			baseLen: d.baseLen, depth: d.depth, flags: d.flags,
		})
		if len(cands) >= limit {
			break
		}
	}
	return cands
}
func searchShort(plan *queryPlan, snap *Snapshot) []uint32 {
	if len(plan.basenameQ) == 0 {
		return nil
	}
	if len(plan.basenameQ) >= 2 {
		if ids := snap.byPrefix2[prefix2(plan.basenameQ)]; len(ids) > 0 {
			return ids
		}
	}
	return snap.byPrefix1[prefix1(plan.basenameQ)]
}
func searchTrigrams(plan *queryPlan, snap *Snapshot) []uint32 {
	if len(plan.trigrams) == 0 {
		return nil
	}
	lists := make([][]uint32, 0, len(plan.trigrams))
	for _, g := range plan.trigrams {
		ids, ok := snap.byGram[g]
		if !ok || len(ids) == 0 {
			return nil
		}
		lists = append(lists, ids)
	}
	return intersectAll(lists)
}
func searchFuzzyFallback(plan *queryPlan, snap *Snapshot) []uint32 {
	if len(plan.basenameQ) == 0 {
		return nil
	}
	bucket := snap.byPrefix1[prefix1(plan.basenameQ)]
	if len(bucket) == 0 {
		return searchSubsequenceScan(plan, snap, 5000)
	}
	var ids []uint32
	for _, id := range bucket {
		if snap.deleted[id] || int(id) >= len(snap.docs) {
			continue
		}
		if isSubsequence([]byte(snap.docs[id].path), plan.basenameQ) {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return searchSubsequenceScan(plan, snap, 5000)
	}
	return ids
}
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
		if isSubsequence([]byte(snap.docs[id].path), plan.basenameQ) {
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
func mergeAndScore(cands []candidate, plan *queryPlan, params ScoreParams, topK int) []Result {
	if topK <= 0 || len(cands) == 0 {
		return nil
	}
	query := []byte(plan.normalized)
	h := &resultHeap{}
	heap.Init(h)
	for i := range cands {
		c := &cands[i]
		s := scorePath([]byte(c.path), c.baseOff, c.baseLen, c.depth, query, plan.tokens, params)
		if s <= 0 {
			continue
		}
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
		r := Result{Path: c.path, Score: s, IsDir: c.flags == uint16(FlagDir)}
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
		results[i] = heap.Pop(h).(Result)
	}
	return results
}

type resultHeap []Result

func (h resultHeap) Len() int            { return len(h) }
func (h resultHeap) Less(i, j int) bool   { return h[i].Score < h[j].Score }
func (h resultHeap) Swap(i, j int)        { h[i], h[j] = h[j], h[i] }
func (h *resultHeap) Push(x interface{}) { *h = append(*h, x.(Result)) }
func (h *resultHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}
