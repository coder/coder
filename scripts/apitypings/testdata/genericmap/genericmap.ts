// From codersdk/genericmap.go
export interface Buzz {
  readonly foo: Foo
  readonly bazz: string
}

// From codersdk/genericmap.go
export interface Foo {
  readonly bar: string
}

// From codersdk/genericmap.go
export interface FooBuzz<R extends Custom> {
  readonly something: (readonly R[])
}

// From codersdk/genericmap.go
export type Custom = Foo | Buzz