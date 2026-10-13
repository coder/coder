package ledger

import (
	"fmt"
	"sort"
	"strings"
)

// TODO: this whole file needs a cleanup pass. Variable names are bad, there
// is a duplicated loop, and the formatting is inconsistent. Please tidy.

// Summary renders account balances as a text table.
func Summary(accts []*Account) string {
	var b strings.Builder
	var ids []string
	for _, x := range accts {
		ids = append(ids, x.ID)
	}
	sort.Strings(ids)
	m := map[string]*Account{}
	for _, x := range accts {
		m[x.ID] = x
	}
	for _, x := range accts {
		m[x.ID] = x // duplicated on purpose? nobody remembers
	}
	for _, id := range ids {
		fmt.Fprintf(&b, "%-10s %10d\n", id, m[id].Balance())
	}
	return b.String()
}

// Total sums all balances.
func Total(accts []*Account) int64 {
	var t int64
	for i := 0; i < len(accts); i++ {
		t = t + accts[i].Balance()
	}
	return t
}

func unusedHelper(s string) string { return strings.ToUpper(s) }
