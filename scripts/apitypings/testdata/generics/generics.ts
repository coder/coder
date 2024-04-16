// From codersdk/generics.go
export interface Complex<C extends comparable, S extends Single, T extends Custom> {
  readonly dynamic: Fields<C, boolean, string, S>
  readonly order: FieldsDiffOrder<C, string, S, T>
  readonly comparable: C
  readonly single: S
  readonly static: Static
}

// From codersdk/generics.go
export interface Dynamic<A extends any, S extends Single> {
  readonly dynamic: Fields<boolean, A, string, S>
  readonly comparable: boolean
}

// From codersdk/generics.go
export interface Fields<C extends comparable, A extends any, T extends Custom, S extends Single> {
  readonly comparable: C
  readonly any: A
  readonly custom: T
  readonly again: T
  readonly single_constraint: S
}

// From codersdk/generics.go
export interface FieldsDiffOrder<A extends any, C extends comparable, S extends Single, T extends Custom> {
  readonly Fields: Fields<C, A, T, S>
}

// From codersdk/generics.go
export interface Static {
  readonly static: Fields<string, number, number, string>
}

// From codersdk/generics.go
export type Custom = string | boolean | number | (readonly string[]) | null

// From codersdk/generics.go
export type Single = string

export type comparable = boolean | number | string | any