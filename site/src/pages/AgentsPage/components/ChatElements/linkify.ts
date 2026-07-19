type LinkSegment =
	| { kind: "text"; value: string }
	| { kind: "url"; value: string };

const URL_PATTERN = /https?:\/\/[^\s<>"'`]+/g;

// Characters that end a sentence around a URL far more often than
// they end the URL itself.
const TRAILING_PUNCTUATION = new Set([".", ",", ";", ":", "!", "?"]);

const trimTrailingPunctuation = (url: string): string => {
	let end = url.length;
	while (end > 0) {
		const char = url[end - 1];
		if (TRAILING_PUNCTUATION.has(char)) {
			end -= 1;
			continue;
		}
		if (char === ")") {
			const candidate = url.slice(0, end);
			const opens = candidate.split("(").length - 1;
			const closes = candidate.split(")").length - 1;
			if (closes > opens) {
				end -= 1;
				continue;
			}
		}
		break;
	}
	return url.slice(0, end);
};

/**
 * Concatenating the returned segment values reproduces the input, so
 * whitespace in preformatted output is preserved.
 */
export const splitTextForLinks = (text: string): LinkSegment[] => {
	const segments: LinkSegment[] = [];
	let lastIndex = 0;
	for (const match of text.matchAll(URL_PATTERN)) {
		const url = trimTrailingPunctuation(match[0]);
		if (url.length === 0) {
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
