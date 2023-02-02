package dbgen

import "net"

func takeFirstIP(values ...net.IPNet) net.IPNet {
	return takeFirstF(values, func(v net.IPNet) bool {
		return len(v.IP) != 0 && len(v.Mask) != 0
	})
}

// takeFirstBytes implements takeFirst for []byte.
// []byte is not a comparable type.
func takeFirstBytes(values ...[]byte) []byte {
	return takeFirstF(values, func(v []byte) bool {
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
