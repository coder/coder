declare module "match-sorter" {
  export enum rankings {
    CASE_SENSITIVE_EQUAL = 9,
    EQUAL = 8,
    STARTS_WITH = 7,
    WORD_STARTS_WITH = 6,
    STRING_CASE = 5,
    STRING_CASE_ACRONYM = 4,
    CONTAINS = 3,
    ACRONYM = 2,
    MATCHES = 1,
    NO_MATCH = 0,
  }

  export type KeyOptions<T> = (item: T) => string[] | string

  export type ExtendedKeyOptions<T> = { key: KeyOptions<T> } & (
    | { maxRanking: number }
    | { minRanking: number }
    | { threshold: number }
  )

  export interface Options<T> {
    keys?: Array<ExtendedKeyOptions<T> | KeyOptions<T> | keyof T>
    threshold?: number
    keepDiacritics?: boolean
    baseSort?: (a: T, b: T) => number
  }

  declare function matchSorter<T>(items: ReadonlyArray<T>, value: string, options?: Options<T>): T[]

  export default matchSorter
}
