type LinkSegment =
	| { kind: "text"; value: string }
	| { kind: "url"; value: string };

// Control characters must end URLs so ANSI escapes cannot become part of them.
// Schemes match case-insensitively, like GFM's autolinker.
// biome-ignore lint/suspicious/noControlCharactersInRegex: intentional
const URL_PATTERN = /https?:\/\/[^\s<>"'`\u0000-\u001f\u007f]+/gi;

// GFM autolinks treat these characters as trailing punctuation.
const TRAILING_PUNCTUATION = new Set([
	".",
	",",
	";",
	":",
	"!",
	"?",
	"*",
	"_",
	"~",
]);

const trimTrailingPunctuation = (url: string): string => {
	let parenBalance = 0;
	let bracketBalance = 0;
	for (const char of url) {
		if (char === "(") {
			parenBalance += 1;
		} else if (char === ")") {
			parenBalance -= 1;
		} else if (char === "[") {
			bracketBalance += 1;
		} else if (char === "]") {
			bracketBalance -= 1;
		}
	}
	let end = url.length;
	while (end > 0) {
		const char = url[end - 1];
		if (TRAILING_PUNCTUATION.has(char)) {
			end -= 1;
			continue;
		}
		// Preserve balanced URL parentheses while trimming unmatched closers.
		if (char === ")" && parenBalance < 0) {
			parenBalance += 1;
			end -= 1;
			continue;
		}
		// Same for brackets, so "[http://x]" sheds its wrapper while IPv6
		// hosts like http://[::1]:8080/ keep theirs.
		if (char === "]" && bracketBalance < 0) {
			bracketBalance += 1;
			end -= 1;
			continue;
		}
		break;
	}
	return url.slice(0, end);
};

/** Concatenating the returned segment values reproduces the input. */
export const splitTextForLinks = (text: string): LinkSegment[] => {
	const segments: LinkSegment[] = [];
	let lastIndex = 0;
	for (const match of text.matchAll(URL_PATTERN)) {
		const url = trimTrailingPunctuation(match[0]);
		if (match.index > lastIndex) {
			segments.push({
				kind: "text",
				value: text.slice(lastIndex, match.index),
			});
		}
		segments.push({ kind: "url", value: url });
		lastIndex = match.index + url.length;
	}
	if (lastIndex < text.length) {
		segments.push({ kind: "text", value: text.slice(lastIndex) });
	}
	return segments;
};
