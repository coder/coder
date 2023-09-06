/**
 * firstLetter extracts the first character and returns it, uppercased.
 */
export const firstLetter = (str: string): string => {
  if (str.length > 0) {
    return str[0].toLocaleUpperCase();
  }

  return "";
};
