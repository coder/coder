package authztest

import (
	. "github.com/coder/coder/coderd/authz"
)

type iterable interface {
	Iterator() iterator
}

type iterator interface {
	iterable

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

func union(sets ...Set) *unionIterator {
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

func (si unionIterator) Permission() *Permission {
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

func (si *unionIterator) Iterator() iterator {
	return si
}

type productIterator struct {
	i, j   int
	a      Set
	b      Set
	buffer Set
}

func product(a, b Set) *productIterator {
	i := &productIterator{
		i: 0,
		j: 0,
		a: a,
		b: b,
	}
	i.buffer = make(Set, i.ReturnSize())
	return i
}

func (s *productIterator) Next() bool {
	s.i++
	if s.i >= len(s.a) {
		s.i = 0
		s.j++
	}
	if s.j >= len(s.b) {
		return false
	}
	return true
}

func (s productIterator) Permissions() Set {
	s.buffer[0] = s.a[s.i]
	s.buffer[1] = s.b[s.j]
	return s.buffer
}

func (s *productIterator) Reset() {
	s.i, s.j = 0, 0
}

func (s *productIterator) ReturnSize() int {
	return 2
}

func (s *productIterator) Size() int {
	return len(s.a) * len(s.b)
}

func (s *productIterator) Iterator() iterator {
	return s
}
