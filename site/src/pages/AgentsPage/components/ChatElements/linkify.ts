type LinkSegment =
	| { kind: "text"; value: string }
	| { kind: "url"; value: string };

// Whitespace, angle brackets, quotes, backticks, and ASCII control
// characters (so ANSI escapes in pasted logs stay out of hrefs) end a URL.
// biome-ignore lint/suspicious/noControlCharactersInRegex: control characters must end URLs
const URL_PATTERN = /https?:\/\/[^\s<>"'`\u0000-\u001f\u007f]+/gi;

// Trailing characters that common linkifiers exclude from a URL: sentence
// punctuation and markdown emphasis delimiters.
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
		// Trim unmatched closers so "(http://x)" and "[http://x]" shed their
		// wrappers while balanced pairs, like Wikipedia paths and IPv6
		// hosts, keep theirs.
		if (char === ")" && parenBalance < 0) {
			parenBalance += 1;
			end -= 1;
			continue;
		}
		if (char === "]" && bracketBalance < 0) {
			bracketBalance += 1;
			end -= 1;
			continue;
		}
		break;
	}
	return url.slice(0, end);
};

/**
 * Splits prompt text into literal text and URL segments; concatenating the
 * segment values reproduces the input byte for byte. Prompts are plain
 * text, not markdown, so every http(s) URL becomes a link regardless of
 * surrounding syntax, including inside backticks or code fences. Trailing
 * sentence punctuation and unmatched closing brackets are excluded from the
 * URL, matching common linkifiers.
 */
export const splitTextForLinks = (text: string): LinkSegment[] => {
	const segments: LinkSegment[] = [];
	let lastIndex = 0;
	for (const match of text.matchAll(URL_PATTERN)) {
		// A scheme glued to a preceding word (myhttp://x) is not a URL.
		const previous = text[match.index - 1];
		if (previous !== undefined && /[a-z]/i.test(previous)) {
			continue;
		}
		const url = trimTrailingPunctuation(match[0]);
		// Skip matches whose host trimmed away entirely, like "http://.".
		if (/^https?:\/\/$/i.test(url)) {
			continue;
		}
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
