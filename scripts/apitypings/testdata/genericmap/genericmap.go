package codersdk

type Foo struct {
	Bar string `json:"bar"`
}

type Buzz struct {
	Foo  `json:"foo"`
	Bazz string `json:"bazz"`
}

type Custom interface {
	Foo | Buzz
}

type FooBuzz[R Custom] struct {
	Something []R `json:"something"`
}

// Not yet supported
//type FooBuzzMap[R Custom] struct {
//	Something map[string]R `json:"something"`
//}

// Not yet supported
//type FooBuzzAnonymousUnion[R Foo | Buzz] struct {
//	Something []R `json:"something"`
//}
