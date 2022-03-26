package authztest

import (
	"strings"
)

// Role can print all possible permutations of the given iterators.
type Role struct {
	ReturnSize     int
	Size           int
	PermissionSets []iterator
	// This is kinda werird, but the first scan should not move anything.
	first bool
}

func (r *Role) Each(ea func(set Set)) {
	for r.Next() {
		ea(r.Permissions())
	}
}

func NewRole(sets ...iterable) *Role {
	setInterfaces := make([]iterator, 0, len(sets))
	var retSize int
	var size int = 1
	for _, s := range sets {
		v := s.Iterator()
		setInterfaces = append(setInterfaces, v)
		retSize += v.ReturnSize()
		size *= v.Size()
	}
	return &Role{
		ReturnSize:     retSize,
		Size:           size,
		PermissionSets: setInterfaces,
	}
}

// Next will gr
func (s *Role) Next() bool {
	if !s.first {
		s.first = true
		return true
	}
	for i := range s.PermissionSets {
		if s.PermissionSets[i].Next() {
			break
		} else {
			s.PermissionSets[i].Reset()
			if i == len(s.PermissionSets)-1 {
				return false
			}
		}
	}
	return true
}

func (s *Role) Permissions() Set {
	all := make(Set, 0, s.Size)
	for _, set := range s.PermissionSets {
		all = append(all, set.Permissions()...)
	}
	return all
}

type Set []*Permission

func (s Set) String() string {
	var str strings.Builder
	sep := ""
	for _, v := range s {
		str.WriteString(sep)
		str.WriteString(v.String())
		sep = ", "
	}
	return str.String()
}

func (s Set) Iterator() iterator {
	return union(s)
}
