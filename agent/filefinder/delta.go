package filefinder

// doc is a single indexed document (file or directory).
// Once appended to the docs slice, all fields are immutable.
type doc struct {
	path    string // normalized relative path (lowercase, forward slashes)
	baseOff int    // offset of basename in path
	baseLen int    // length of basename
	depth   int    // number of '/' separators
	flags   uint16 // FileFlag bits
}

// Index is an in-memory file index. The writer (engine) must
// synchronize mutations externally. Readers use Snapshot() which
// returns a frozen, race-free view.
//
// The docs slice is strictly append-only — entries are never
// modified after creation. Deletions are tracked separately in
// the deleted map so that Snapshot can cheaply capture a frozen
// copy of just the (typically small) deletion set.
type Index struct {
	docs []doc

	// Trigram index: gram -> list of doc IDs (append-only).
	byGram map[uint32][]uint32

	// Prefix indexes for short queries.
	byPrefix1 [256][]uint32       // first byte of basename (lowered)
	byPrefix2 map[uint16][]uint32 // first two bytes of basename

	// Path -> doc ID for O(1) lookup/delete. Maps normalized
	// path to the LAST (most recent) doc ID for that path.
	byPath map[string]uint32

	// Deleted doc IDs. Tracked separately from docs so the docs
	// slice stays truly immutable and can be shared with
	// snapshots without races.
	deleted map[uint32]bool
}

// Snapshot is a frozen, read-only view of the index at a point
// in time. The docs slice is shared with the Index (safe because
// docs are immutable). The deleted set and index maps are
// shallow-copied at snapshot time.
type Snapshot struct {
	docs      []doc              // shared backing array (immutable entries)
	count     int                // number of docs visible to this snapshot
	deleted   map[uint32]bool    // frozen copy of deleted set
	byGram    map[uint32][]uint32
	byPrefix1 [256][]uint32
	byPrefix2 map[uint16][]uint32
}

// NewIndex creates an empty Index.
func NewIndex() *Index {
	return &Index{
		byGram:    make(map[uint32][]uint32),
		byPrefix2: make(map[uint16][]uint32),
		byPath:    make(map[string]uint32),
		deleted:   make(map[uint32]bool),
	}
}

// Add inserts a path into the index. If the path already exists
// (exact match after normalization), the old entry is tombstoned
// and a new one appended. Returns the doc ID.
//
// Not safe for concurrent use — caller must synchronize.
func (idx *Index) Add(path string, flags uint16) uint32 {
	// Normalize path.
	norm := string(normalizePathBytes([]byte(path)))

	// Tombstone any existing entry for this path.
	if oldID, ok := idx.byPath[norm]; ok {
		idx.deleted[oldID] = true
	}

	id := uint32(len(idx.docs))

	baseOff, baseLen := extractBasename([]byte(norm))
	depth := 0
	for _, c := range norm {
		if c == '/' {
			depth++
		}
	}

	idx.docs = append(idx.docs, doc{
		path:    norm,
		baseOff: baseOff,
		baseLen: baseLen,
		depth:   depth,
		flags:   flags,
	})

	// Update path map.
	idx.byPath[norm] = id

	// Trigram index.
	for _, g := range extractTrigrams([]byte(norm)) {
		idx.byGram[g] = append(idx.byGram[g], id)
	}

	// Prefix indexes from basename.
	if baseLen > 0 {
		basename := []byte(norm[baseOff : baseOff+baseLen])
		p1 := prefix1(basename)
		idx.byPrefix1[p1] = append(idx.byPrefix1[p1], id)

		p2 := prefix2(basename)
		idx.byPrefix2[p2] = append(idx.byPrefix2[p2], id)
	}

	return id
}

// Remove marks the entry for path as deleted. Returns true if
// found.
//
// Not safe for concurrent use — caller must synchronize.
func (idx *Index) Remove(path string) bool {
	norm := string(normalizePathBytes([]byte(path)))
	id, ok := idx.byPath[norm]
	if !ok {
		return false
	}
	idx.deleted[id] = true
	delete(idx.byPath, norm)
	return true
}

// Has reports whether path exists (not deleted) in the index.
//
// Not safe for concurrent use — caller must synchronize.
func (idx *Index) Has(path string) bool {
	norm := string(normalizePathBytes([]byte(path)))
	_, ok := idx.byPath[norm]
	return ok
}

// Len returns the number of live (non-deleted) documents.
//
// Not safe for concurrent use — caller must synchronize.
func (idx *Index) Len() int {
	return len(idx.byPath)
}

// Snapshot returns a frozen read-only view of the index. The
// docs slice is shared (entries are immutable). The deleted set
// is shallow-copied (typically small). The trigram and prefix
// maps are also shared — this is safe because the search path
// only reads map values that existed at snapshot time (values
// are []uint32 slices that may grow after the snapshot, but Go
// slice headers captured here won't see appended elements).
//
// Not safe for concurrent use — caller must synchronize.
func (idx *Index) Snapshot() *Snapshot {
	// Copy the deleted set. This is the only allocation.
	// Typically very small (only files removed since last
	// rebuild).
	del := make(map[uint32]bool, len(idx.deleted))
	for id := range idx.deleted {
		del[id] = true
	}

	// Copy the index maps by capturing current slice headers.
	// The backing arrays are shared but reads are safe: a
	// snapshot's copy of a []uint32 slice has a fixed length,
	// so even if the Index appends more elements later, the
	// snapshot won't see them (Go slice semantics).
	gramCopy := make(map[uint32][]uint32, len(idx.byGram))
	for g, ids := range idx.byGram {
		gramCopy[g] = ids[:len(ids):len(ids)]
	}

	var p1Copy [256][]uint32
	for i, ids := range idx.byPrefix1 {
		if len(ids) > 0 {
			p1Copy[i] = ids[:len(ids):len(ids)]
		}
	}

	p2Copy := make(map[uint16][]uint32, len(idx.byPrefix2))
	for p, ids := range idx.byPrefix2 {
		p2Copy[p] = ids[:len(ids):len(ids)]
	}

	return &Snapshot{
		docs:      idx.docs[:len(idx.docs):len(idx.docs)],
		count:     len(idx.docs),
		deleted:   del,
		byGram:    gramCopy,
		byPrefix1: p1Copy,
		byPrefix2: p2Copy,
	}
}
