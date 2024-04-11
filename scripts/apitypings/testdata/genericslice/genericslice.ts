// From codersdk/genericslice.go
export interface Bar {
  readonly Bar: string
}

// From codersdk/genericslice.go
export interface Foo<R extends any> {
  readonly Slice: (readonly R[])
  readonly TwoD: (readonly (readonly R[])[])
}