package codersdk

type Bar struct {
	Bar string
}

type Foo[R any] struct {
	Slice []R
	TwoD  [][]R
}
