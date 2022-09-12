export const formatTemplateActiveDevelopers = (num?: number): string => {
  if (num === undefined || num < 0) {
    // Loading
    return "-"
  }
  return num.toString()
}
