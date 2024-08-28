package slice_test

import (
	"fmt"

	"github.com/coder/coder/v2/coderd/util/slice"
)

//nolint:revive // They want me to error check my Printlns
func ExampleSymmetricDifference() {
	// The goal of this function is to find the elements to add & remove from
	// set 'a' to make it equal to set 'b'.
	a := []int{1, 2, 5, 6, 6, 6}
	b := []int{2, 3, 3, 3, 4, 5}
	add, remove := slice.SymmetricDifference(a, b)
	fmt.Println("Elements to add:", add)
	fmt.Println("Elements to remove:", remove)
	// Output:
	// Elements to add: [3 4]
	// Elements to remove: [1 6]
}
