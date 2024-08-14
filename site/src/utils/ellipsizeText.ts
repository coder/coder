import type { Nullable } from "./nullable";

/** Truncates and ellipsizes text if it's longer than maxLength */
export const ellipsizeText = (
  text: Nullable<string>,
  maxLength = 80,
): string | undefined => {
  if (typeof text !== "string") {
    return;
  }
  return text.length <= maxLength
    ? text
    : `${text.substr(0, maxLength - 3)}...`;
};
