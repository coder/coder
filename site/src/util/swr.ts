/**
 * unsafeSWRArgument
 *
 * Helper function for working with SWR / useSWR in the TypeScript world.
 * TypeScript is helpful in enforcing type-safety, but SWR is designed to
 * with the expectation that, if the argument is not available, an exception
 * will be thrown.
 *
 * This just helps in abiding by those rules, explicitly, and lets us suppress
 * the lint warning in a single place.
 */
export const unsafeSWRArgument = <T>(arg: T | null | undefined): T => {
  if (typeof arg === "undefined" || arg === null) {
    throw "SWR: Expected exception because the argument is not available"
  }
  return arg
}
