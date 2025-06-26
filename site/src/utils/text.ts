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

/**
 * Returns a string of the same length, using the unicode "bullet" character as
 * a replacement for each character, like a password input would.
 */
export function maskText(val: string): string {
	return "\u2022".repeat(val.length);
}
