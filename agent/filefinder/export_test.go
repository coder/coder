package filefinder

// Test helpers that need internal access.

// MakeTestSnapshot builds a Snapshot from a list of paths. Useful for
// query-level tests that don't need a real filesystem.
func MakeTestSnapshot(paths []string) *Snapshot {
	idx := NewIndex()
	for _, p := range paths {
		idx.Add(p, 0)
	}
	return idx.Snapshot()
}

// BuildTestIndex walks root and returns a populated Index, the same
// way Engine.AddRoot does but without starting a watcher.
func BuildTestIndex(root string) (*Index, error) {
	return walkRoot(root)
}

// IndexIsDeleted reports whether the document at id is tombstoned.
func IndexIsDeleted(idx *Index, id uint32) bool {
	return idx.deleted[id]
}

// IndexByGramLen returns the number of entries in the trigram index.
func IndexByGramLen(idx *Index) int {
	return len(idx.byGram)
}

// IndexByPrefix1Len returns the number of posting-list entries for
// the given single-byte prefix.
func IndexByPrefix1Len(idx *Index, b byte) int {
	return len(idx.byPrefix1[b])
}

// SnapshotCount returns the number of documents in a Snapshot.
func SnapshotCount(snap *Snapshot) int {
	return len(snap.docs)
}

// EngineSnapLen returns the number of root snapshots currently held
// by the engine, or -1 if the pointer is nil.
func EngineSnapLen(eng *Engine) int {
	p := eng.snap.Load()
	if p == nil {
		return -1
	}
	return len(*p)
}

// DefaultScoreParamsForTest exposes defaultScoreParams for tests.
var DefaultScoreParamsForTest = defaultScoreParams

// ScoreParamsForTest is a type alias for scoreParams.
type ScoreParamsForTest = scoreParams

// Exported aliases for internal functions used in tests.
var (
	NewQueryPlanForTest           = newQueryPlan
	SearchSnapshotForTest         = searchSnapshot
	IntersectSortedForTest        = intersectSorted
	IntersectAllForTest           = intersectAll
	MergeAndScoreForTest          = mergeAndScore
	NormalizeQueryForTest         = normalizeQuery
	NormalizePathBytesForTest     = normalizePathBytes
	ExtractTrigramsForTest        = extractTrigrams
	ExtractBasenameForTest        = extractBasename
	ExtractSegmentsForTest        = extractSegments
	Prefix1ForTest                = prefix1
	Prefix2ForTest                = prefix2
	IsSubsequenceForTest          = isSubsequence
	LongestContiguousMatchForTest = longestContiguousMatch
	IsBoundaryForTest             = isBoundary
	CountBoundaryHitsForTest      = countBoundaryHits
	EqualFoldASCIIForTest         = equalFoldASCII
	ScorePathForTest              = scorePath
	PackTrigramForTest            = packTrigram
)

// Type aliases for internal types used in tests.
type (
	CandidateForTest = candidate
	QueryPlanForTest = queryPlan
)
