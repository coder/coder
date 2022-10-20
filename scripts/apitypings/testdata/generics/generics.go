package generics

import "time"

type Single interface {
	string
}

type Custom interface {
	string | bool | int | time.Duration | []string | *int
}

// StaticGeneric has all generic fields defined in the field
type StaticGeneric struct {
	Static GenericFields[string, int, time.Duration] `json:"static"`
}

// DynamicGeneric can has some dynamic fields
type DynamicGeneric[C comparable, A any] struct {
	Dynamic    GenericFields[C, A, string] `json:"dynamic"`
	Comparable C                           `json:"comparable"`
}

type GenericFields[C comparable, A any, T Custom] struct {
	Comparable C `json:"comparable"`
	Any        A `json:"any"`

	Custom T `json:"custom"`
	Again  T `json:"again"`
}
