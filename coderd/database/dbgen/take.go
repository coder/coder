package dbgen

import "net"

func takeFirstIP(values ...net.IPNet) net.IPNet {
	takeFirstSlice([]string{})

	return takeFirstF(values, func(v net.IPNet) bool {
		return len(v.IP) != 0 && len(v.Mask) != 0
	})
}

// takeFirstSlice implements takeFirst for []any.
// []any is not a comparable type.
func takeFirstSlice[T any](values ...[]T) []T {
	return takeFirstF(values, func(v []T) bool {
		return len(v) != 0
	})
}

// takeFirstF takes the first value that returns true
func takeFirstF[Value any](values []Value, take func(v Value) bool) Value {
	var empty Value
	for _, v := range values {
		if take(v) {
			return v
		}
	}
	// If all empty, return empty
	return empty
}

// takeFirst will take the first non-empty value.
func takeFirst[Value comparable](values ...Value) Value {
	var empty Value
	return takeFirstF(values, func(v Value) bool {
		return v != empty
	})
}
