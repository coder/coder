type LinkSegment =
	| { kind: "text"; value: string }
	| { kind: "url"; value: string };

// Stops at ASCII control characters so ANSI escape sequences in raw
// shell output (e.g. color codes) never leak into the URL.
// biome-ignore lint/suspicious/noControlCharactersInRegex: intentional
const URL_PATTERN = /https?:\/\/[^\s<>"'`\u0000-\u001f\u007f]+/g;

// Characters that end a sentence around a URL far more often than
// they end the URL itself.
const TRAILING_PUNCTUATION = new Set([".", ",", ";", ":", "!", "?"]);

const trimTrailingPunctuation = (url: string): string => {
	let parenBalance = 0;
	for (const char of url) {
		if (char === "(") {
			parenBalance += 1;
		} else if (char === ")") {
			parenBalance -= 1;
		}
	}
	let end = url.length;
	while (end > 0) {
		const char = url[end - 1];
		if (TRAILING_PUNCTUATION.has(char)) {
			end -= 1;
			continue;
		}
		// Trim a trailing ")" only while there are more closers than
		// openers, so "(see http://x/(a))" keeps the URL's own parens.
		if (char === ")" && parenBalance < 0) {
			parenBalance += 1;
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
