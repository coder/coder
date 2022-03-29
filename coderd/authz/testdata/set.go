package testdata

import (
	"strings"

	. "github.com/coder/coder/coderd/authz"
)

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
