package authztest

import (
	"github.com/coder/coder/coderd/authz"
)

type Iterable interface {
	Iterator() Iterator
}

type Iterator interface {
	Iterable

	Next() bool
	Permissions() Set
	Reset()
	ReturnSize() int
	Size() int
}

// unionIterator is very primitive, just used to hold a place in a set.
type unionIterator struct {
	// setIdx determines which set the offset is for
	setIdx int
	// offset is which permission for a given setIdx
	offset int
	sets   []Set
	// buffer is used to prevent allocations when `Permissions` is called, as we must
	// return a set.
	buffer Set

	N int
}

func Union(sets ...Set) *unionIterator {
	var n int
	for _, s := range sets {
		n += len(s)
	}
	return &unionIterator{
		sets:   sets,
		buffer: make(Set, 1),
		N:      n,
	}
}

func (si *unionIterator) Next() bool {
	si.offset++
	if si.offset >= len(si.sets[si.setIdx]) {
		si.setIdx++
		si.offset = 0
	}

	return si.setIdx < len(si.sets)
}

func (si *unionIterator) Permissions() Set {
	si.buffer[0] = si.Permission()
	return si.buffer
}

func (si unionIterator) Permission() *authz.Permission {
	return si.sets[si.setIdx][si.offset]
}

func (si *unionIterator) Reset() {
	si.setIdx = 0
	si.offset = 0
}

func (si *unionIterator) ReturnSize() int {
	return 1
}

func (si *unionIterator) Size() int {
	return si.N
}

func (si *unionIterator) Iterator() Iterator {
	return si
}
