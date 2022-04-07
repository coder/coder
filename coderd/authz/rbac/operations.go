package rbac

import "strings"

// Operation is an action conducted on a resource.
type Operation string

// Operations is a slice of operations.
type Operations []Operation

// Join returns a slice of operations in a string format.
func (os Operations) String() string {
	var strs []string
	for _, operation := range os {
		strs = append(strs, string(operation))
	}
	return strings.Join(strs, ",")
}
