package authztest

import (
	"fmt"
	"testing"
)

func Test_PermissionSetWPlusSearchSpace(t *testing.T) {
	all := GroupedPermissions(AllPermissions())
	wplus := NewRole(
		all.Wildcard().Positive(),
		union(all.Wildcard().Abstain()[:1], nilSet),

		union(all.Site().Positive()[:1], nilSet),
		union(all.Site().Negative()[:1], nilSet),
		union(all.Site().Abstain()[:1], nilSet),

		union(all.AllOrgs().Positive()[:1], nilSet),
		union(all.AllOrgs().Negative()[:1], nilSet),
		union(all.AllOrgs().Abstain()[:1], nilSet),

		union(all.User().Positive()[:1], nilSet),
		union(all.User().Negative()[:1], nilSet),
		union(all.User().Abstain()[:1], nilSet),
	)
	fmt.Println(wplus.N)
	fmt.Println(len(AllPermissions()))
	for k, v := range all {
		fmt.Printf("%s=%d\n", string(k), len(v.All()))
	}

	var i int
	wplus.Each(func(set Set) {
		fmt.Printf("%d: %s\n", i, set.String())
		i++
	})
}
