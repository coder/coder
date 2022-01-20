export const firstOrOnly = <T>(itemOrItems: T | T[]) => {
  if (Array.isArray(itemOrItems)) {
    return itemOrItems[0]
  } else {
    return itemOrItems
  }
}
