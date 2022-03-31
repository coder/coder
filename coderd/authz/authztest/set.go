package authztest

import (
	"strings"

	. "github.com/coder/coder/coderd/authz"
)

type Set []*Permission

var _ iterable = (Set)(nil)

func (s Set) Iterator() iterator {
	return union(s)
}

func (s Set) String() string {
	var str strings.Builder
	sep := ""
	for _, v := range s {
		if v == nil {
			continue
		}
		str.WriteString(sep)
		str.WriteString(v.String())
		sep = ", "
	}
	return str.String()
}
