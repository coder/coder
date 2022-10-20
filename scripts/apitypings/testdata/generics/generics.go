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
	Static GenericFields[string, int, time.Duration, string] `json:"static"`
}

// DynamicGeneric can has some dynamic fields
type DynamicGeneric[A any, S Single] struct {
	Dynamic    GenericFields[bool, A, string, S] `json:"dynamic"`
	Comparable bool                              `json:"comparable"`
}

type ComplexGeneric[C comparable, S Single, T Custom] struct {
	Dynamic    GenericFields[C, bool, string, S]       `json:"dynamic"`
	Order      GenericFieldsDiffOrder[C, string, S, T] `json:"order"`
	Comparable C                                       `json:"comparable"`
	Single     S                                       `json:"single"`
	Static     StaticGeneric                           `json:"static"`
}

type GenericFields[C comparable, A any, T Custom, S Single] struct {
	Comparable C `json:"comparable"`
	Any        A `json:"any"`

	Custom           T `json:"custom"`
	Again            T `json:"again"`
	SingleConstraint S `json:"single_constraint"`
}

type GenericFieldsDiffOrder[A any, C comparable, S Single, T Custom] struct {
	GenericFields[C, A, T, S]
}
