/**
 * `firstOrOnly` handles disambiguation of a value that is either a single item or array.
 *
 * If an array is passed in, the first item will be returned.
 */
export const firstOrOnly = <T>(itemOrItems: T | T[]) => {
  if (Array.isArray(itemOrItems)) {
    return itemOrItems[0]
  } else {
    return itemOrItems
  }
}
