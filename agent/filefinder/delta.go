package filefinder

import "strings"

// FileFlag represents the type of filesystem entry.
type FileFlag uint16

const (
	FlagFile    FileFlag = 0
	FlagDir     FileFlag = 1
	FlagSymlink FileFlag = 2
)

type doc struct {
	path    string
	baseOff int
	baseLen int
	depth   int
	flags   uint16
}

// Index is an append-only in-memory file index with snapshot support.
type Index struct {
	docs      []doc
	byGram    map[uint32][]uint32
	byPrefix1 [256][]uint32
	byPrefix2 map[uint16][]uint32
	byPath    map[string]uint32
	deleted   map[uint32]bool
}

// Snapshot is a frozen, read-only view of the index at a point in time.
type Snapshot struct {
	docs      []doc
	deleted   map[uint32]bool
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

// Add inserts a path into the index, tombstoning any previous entry.
func (idx *Index) Add(path string, flags uint16) uint32 {
	norm := string(normalizePathBytes([]byte(path)))
	if oldID, ok := idx.byPath[norm]; ok {
		idx.deleted[oldID] = true
	}
	id := uint32(len(idx.docs)) //nolint:gosec // Index will never exceed 2^32 docs.
	baseOff, baseLen := extractBasename([]byte(norm))
	idx.docs = append(idx.docs, doc{
		path: norm, baseOff: baseOff, baseLen: baseLen,
		depth: strings.Count(norm, "/"), flags: flags,
	})
	idx.byPath[norm] = id
	for _, g := range extractTrigrams([]byte(norm)) {
		idx.byGram[g] = append(idx.byGram[g], id)
	}
	if baseLen > 0 {
		basename := []byte(norm[baseOff : baseOff+baseLen])
		p1 := prefix1(basename)
		idx.byPrefix1[p1] = append(idx.byPrefix1[p1], id)
		p2 := prefix2(basename)
		idx.byPrefix2[p2] = append(idx.byPrefix2[p2], id)
	}
	return id
}

// Remove marks the entry for path as deleted.
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
func (idx *Index) Has(path string) bool {
	_, ok := idx.byPath[string(normalizePathBytes([]byte(path)))]
	return ok
}

// Len returns the number of live (non-deleted) documents.
func (idx *Index) Len() int { return len(idx.byPath) }

func copyPostings[K comparable](m map[K][]uint32) map[K][]uint32 {
	cp := make(map[K][]uint32, len(m))
	for k, v := range m {
		cp[k] = v[:len(v):len(v)]
	}
	return cp
}

// Snapshot returns a frozen read-only view of the index.
func (idx *Index) Snapshot() *Snapshot {
	del := make(map[uint32]bool, len(idx.deleted))
	for id := range idx.deleted {
		del[id] = true
	}
	var p1Copy [256][]uint32
	for i, ids := range idx.byPrefix1 {
		if len(ids) > 0 {
			p1Copy[i] = ids[:len(ids):len(ids)]
		}
	}
	return &Snapshot{
		docs:      idx.docs[:len(idx.docs):len(idx.docs)],
		deleted:   del,
		byGram:    copyPostings(idx.byGram),
		byPrefix1: p1Copy,
		byPrefix2: copyPostings(idx.byPrefix2),
	}
}
