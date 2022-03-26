package authztest

import (
	"fmt"
	"testing"
)

func TestRole(t *testing.T) {
	all := GroupedPermissions(AllPermissions())
	testCases := []struct {
		Name         string
		Permutations *Role
		Access       bool
	}{
		{
			Name:         "W+",
			Permutations: ParseRole(all, "w(pa) s(*e) o(*e) u(*e)"),
			Access:       true,
		},
		{
			Name:         "W-",
			Permutations: ParseRole(all, "w(n) w(pae) s(*e) o(*e) u(*e)"),
			Access:       false,
		},
		{
			Name:         "S+",
			Permutations: ParseRole(all, "w(a) s(pa) o(*e) u(*e)"),
			Access:       true,
		},
		{
			Name:         "S-",
			Permutations: ParseRole(all, "w(a) s(n) s(pae) o(*e) u(*e)"),
			Access:       false,
		},
		{
			Name:         "O+",
			Permutations: ParseRole(all, "w(a) s(a) o(pa) u(*e)"),
			Access:       true,
		},
		{
			Name:         "O-",
			Permutations: ParseRole(all, "w(a) s(a) o(n) o(pae) u(*e)"),
			Access:       false,
		},
		{
			Name:         "U+",
			Permutations: ParseRole(all, "w(a) s(a) o(a) u(pa)"),
			Access:       true,
		},
		{
			Name:         "U-",
			Permutations: ParseRole(all, "w(a) s(a) o(a) u(n) u(pa)"),
			Access:       false,
		},
		{
			Name:         "A0",
			Permutations: ParseRole(all, "w(a) s(a) o(a) u(a)"),
			Access:       false,
		},
	}

	var total uint64
	for _, c := range testCases {
		fmt.Println(c.Name)
		fmt.Printf("\tSize=%d\n", c.Permutations.Size)
		total += uint64(c.Permutations.Size)
	}
	fmt.Printf("Total cases=%d\n", total)

	// This is how you run the test cases
	//for _, c := range testCases {
	//	t.Run(c.Name, func(t *testing.T) {
	//		c.Permutations.Each(func(set Set) {
	//			// Actually printing all the errors would be insane
	//			//require.Equal(t, c.Access, FakeAuthorize(set))
	//			FakeAuthorize(set)
	//		})
	//	})
	//}
}

func FakeAuthorize(s Set) bool {
	var f bool
	for _, i := range s {
		if i.Type() == "+" {
			f = true
		}
	}
	return f
}
