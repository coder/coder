package testdata

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

// SetIterator is very primitive, just used to hold a place in a set.
type SetIterator struct {
	i      int
	set    Set
	buffer Set
}

func union(sets ...Set) *SetIterator {
	all := Set{}
	for _, set := range sets {
		all = append(all, set...)
	}
	return &SetIterator{
		i:      0,
		set:    all,
		buffer: make(Set, 1),
	}
}

func (si *SetIterator) Next() bool {
	si.i++
	return si.i < len(si.set)
}

func (si *SetIterator) Permissions() Set {
	si.buffer[0] = si.set[si.i]
	return si.buffer
}

func (si *SetIterator) Permission() *Permission {
	return si.set[si.i]
}

func (si *SetIterator) Reset() {
	si.i = 0
}

func (si *SetIterator) ReturnSize() int {
	return 1
}

func (si *SetIterator) Size() int {
	return len(si.set)
}

func (si *SetIterator) Iterator() iterator {
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
