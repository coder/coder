/**
 * A Nullable may be its concrete type, `null` or `undefined`
 * @remark Exact opposite of the native TS type NonNullable<T>
 */
export type Nullable<T> = null | undefined | T;
