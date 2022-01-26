/**
 * Helper function that, given an array or a single item:
 * - If an array with no elements, returns null
 * - If an array with 1 or more elements, returns the first element
 * - If a single item, returns that item
 */
export const firstOrItem = <T>(itemOrItems: T | T[]): T | null => {
  if (Array.isArray(itemOrItems)) {
    return itemOrItems.length > 0 ? itemOrItems[0] : null
  }

  return itemOrItems
}
