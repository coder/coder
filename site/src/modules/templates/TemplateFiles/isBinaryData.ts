export function isBinaryData(s: string): boolean {
  // Remove unicode characters from the string like emojis.
  const asciiString = s.replace(/[\u007F-\uFFFF]/g, "");

  // Create a set of all printable ASCII characters (and some control characters).
  const textChars = new Set(
    [7, 8, 9, 10, 12, 13, 27].concat(
      Array.from({ length: 128 }, (_, i) => i + 32),
    ),
  );

  const isBinaryString = (str: string): boolean => {
    for (let i = 0; i < str.length; i++) {
      if (!textChars.has(str.charCodeAt(i))) {
        return true;
      }
    }
    return false;
  };

  return isBinaryString(asciiString);
}
