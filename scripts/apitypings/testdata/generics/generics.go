package codersdk

import "time"

type Single interface {
	string
}

type Custom interface {
	string | bool | int | time.Duration | []string | *int
}

// Static has all generic fields defined in the field
type Static struct {
	Static Fields[string, int, time.Duration, string] `json:"static"`
}

// Dynamic has some dynamic fields.
type Dynamic[A any, S Single] struct {
	Dynamic    Fields[bool, A, string, S] `json:"dynamic"`
	Comparable bool                       `json:"comparable"`
}

type Complex[C comparable, S Single, T Custom] struct {
	Dynamic    Fields[C, bool, string, S]       `json:"dynamic"`
	Order      FieldsDiffOrder[C, string, S, T] `json:"order"`
	Comparable C                                `json:"comparable"`
	Single     S                                `json:"single"`
	Static     Static                           `json:"static"`
}

type Fields[C comparable, A any, T Custom, S Single] struct {
	Comparable C `json:"comparable"`
	Any        A `json:"any"`

	Custom           T `json:"custom"`
	Again            T `json:"again"`
	SingleConstraint S `json:"single_constraint"`
}

type FieldsDiffOrder[A any, C comparable, S Single, T Custom] struct {
	Fields Fields[C, A, T, S]
}
