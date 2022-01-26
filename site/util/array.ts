export const firstOrItem = <T>(itemOrItems: T | T[]): T | null => {
  if (Array.isArray(itemOrItems)) {
    return itemOrItems.length > 0 ? itemOrItems[0] : null
  }

  return itemOrItems
}
