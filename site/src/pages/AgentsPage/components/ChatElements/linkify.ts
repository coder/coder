type LinkSegment =
	| { kind: "text"; value: string }
	| { kind: "url"; value: string };

const SCHEME_PATTERN = /https?:\/\//gi;

const isAsciiAlpha = (ch: string): boolean => /[A-Za-z]/.test(ch);
const isWhitespace = (ch: string): boolean => /\s/.test(ch);
// CommonMark's "Unicode punctuation" includes symbols.
const isPunctuation = (ch: string): boolean => /[\p{P}\p{S}]/u.test(ch);
const isAsciiControl = (ch: string): boolean => {
	const code = ch.charCodeAt(0);
	return code <= 0x1f || code === 0x7f;
};

const TRAIL_CHARS = new Set([
	"!",
	'"',
	"'",
	")",
	"*",
	",",
	".",
	":",
	";",
	"?",
	"_",
	"~",
]);

// Characters that may end the URL early; scanPath checks each against
// isTrail before consuming it.
const PATH_CHECK_CHARS = new Set([...TRAIL_CHARS, "&", "<", "]"]);

/**
 * Whether everything from index up to the next URL end (whitespace, `<`, or
 * end of text) is trailing punctuation that GFM excludes from the link.
 */
const isTrail = (text: string, index: number): boolean => {
	let i = index;
	for (;;) {
		const ch = text[i];
		if (ch === undefined || ch === "<" || isWhitespace(ch)) {
			return true;
		}
		if (TRAIL_CHARS.has(ch)) {
			i += 1;
			continue;
		}
		// A trailing entity reference (`&amp;`) also counts as punctuation.
		if (ch === "&") {
			i += 1;
			if (!isAsciiAlpha(text[i] ?? "")) {
				return false;
			}
			while (isAsciiAlpha(text[i] ?? "")) {
				i += 1;
			}
			if (text[i] !== ";") {
				return false;
			}
			i += 1;
			continue;
		}
		// `]` ends the URL when followed by an end or by `(`/`[`, which could
		// start a markdown resource or reference.
		if (ch === "]") {
			i += 1;
			const next = text[i];
			if (
				next === undefined ||
				next === "(" ||
				next === "[" ||
				isWhitespace(next)
			) {
				return true;
			}
			continue;
		}
		return false;
	}
};

/**
 * Scans a GFM domain (host) after the `://`. Returns its exclusive end, or
 * null when invalid: empty, starting with punctuation or a control
 * character, or containing `_` in the last two dot-separated segments.
 */
const scanDomain = (text: string, start: number): number | null => {
	const first = text[start];
	if (
		first === undefined ||
		isAsciiControl(first) ||
		isWhitespace(first) ||
		isPunctuation(first)
	) {
		return null;
	}
	let underscoreInLastSegment = false;
	let underscoreInPenultimateSegment = false;
	let seen = false;
	let i = start;
	for (;;) {
		const ch = text[i];
		if (ch === "." || ch === "_") {
			if (isTrail(text, i)) {
				break;
			}
			if (ch === "_") {
				underscoreInLastSegment = true;
			} else {
				underscoreInPenultimateSegment = underscoreInLastSegment;
				underscoreInLastSegment = false;
			}
			i += 1;
			continue;
		}
		if (
			ch === undefined ||
			isWhitespace(ch) ||
			(ch !== "-" && isPunctuation(ch))
		) {
			break;
		}
		seen = true;
		i += 1;
	}
	if (!seen || underscoreInLastSegment || underscoreInPenultimateSegment) {
		return null;
	}
	return i;
};

/**
 * Scans the URL path after the domain, keeping balanced `()` pairs and
 * stopping before trailing punctuation, `<`, or whitespace.
 */
const scanPath = (text: string, start: number): number => {
	let openParens = 0;
	let closeParens = 0;
	let i = start;
	for (;;) {
		const ch = text[i];
		if (ch === undefined || isWhitespace(ch)) {
			break;
		}
		if (ch === "(") {
			openParens += 1;
			i += 1;
			continue;
		}
		if (ch === ")" && closeParens < openParens) {
			closeParens += 1;
			i += 1;
			continue;
		}
		if (PATH_CHECK_CHARS.has(ch)) {
			if (isTrail(text, i)) {
				break;
			}
			if (ch === ")") {
				closeParens += 1;
			}
			i += 1;
			continue;
		}
		i += 1;
	}
	return i;
};

/**
 * Start offset of the markdown paragraph containing index: just after the
 * closest preceding blank line.
 */
const paragraphStart = (text: string, index: number): number => {
	let lineStart = text.lastIndexOf("\n", index - 1) + 1;
	while (lineStart > 0) {
		const previousLineStart = text.lastIndexOf("\n", lineStart - 2) + 1;
		if (/^[ \t\r]*$/.test(text.slice(previousLineStart, lineStart - 1))) {
			return lineStart;
		}
		lineStart = previousLineStart;
	}
	return 0;
};

// GFM does not autolink inside indented code blocks (4+ columns, tab = 4).
const isCodeIndented = (text: string, index: number): boolean => {
	const lineStart = text.lastIndexOf("\n", index - 1) + 1;
	let columns = 0;
	for (let i = lineStart; i < index; i++) {
		const ch = text[i];
		if (ch === " ") {
			columns += 1;
		} else if (ch === "\t") {
			columns += 4 - (columns % 4);
		} else {
			break;
		}
		if (columns >= 4) {
			return true;
		}
	}
	return false;
};

/**
 * GFM suppresses autolinks after an unmatched `[` (a pending link label) and
 * inside a `[text](url)` destination.
 */
const isInLinkContext = (text: string, index: number): boolean => {
	let depth = 0;
	let lastLabelCloseIndex = -1;
	for (let i = paragraphStart(text, index); i < index; i++) {
		const ch = text[i];
		if (ch === "[") {
			depth += 1;
		} else if (ch === "]" && depth > 0) {
			depth -= 1;
			lastLabelCloseIndex = i;
		}
	}
	if (depth > 0) {
		return true;
	}
	return (
		index >= 2 && text[index - 1] === "(" && lastLabelCloseIndex === index - 2
	);
};

/**
 * Concatenating the returned segment values reproduces the input.
 * The scanning rules are ported from micromark-extension-gfm-autolink-literal
 * (the GFM autolink parser Streamdown uses for agent messages) so prompt
 * links match agent-message links without depending on the markdown parser.
 * Only explicit http(s) literals become URLs.
 */
export const splitTextForLinks = (text: string): LinkSegment[] => {
	const segments: LinkSegment[] = [];
	let cursor = 0;
	const pushUrl = (start: number, end: number) => {
		if (start > cursor) {
			segments.push({ kind: "text", value: text.slice(cursor, start) });
		}
		segments.push({ kind: "url", value: text.slice(start, end) });
		cursor = end;
	};
	for (const match of text.matchAll(SCHEME_PATTERN)) {
		const schemeStart = match.index;
		// Skip schemes consumed by an earlier URL's path.
		if (schemeStart < cursor) {
			continue;
		}
		if (isCodeIndented(text, schemeStart)) {
			continue;
		}
		// CommonMark angle-bracket autolink: `<https://url>` linkifies the
		// inner URL verbatim and the brackets stay text. The URI may contain
		// anything except `<`, `>`, ASCII space, and ASCII control characters.
		if (text[schemeStart - 1] === "<") {
			let end = schemeStart + match[0].length;
			while (
				end < text.length &&
				text[end] !== ">" &&
				text[end] !== "<" &&
				text[end] !== " " &&
				!isAsciiControl(text[end])
			) {
				end += 1;
			}
			if (text[end] === ">") {
				pushUrl(schemeStart, end);
				continue;
			}
		}
		const previous = text[schemeStart - 1];
		if (previous !== undefined && isAsciiAlpha(previous)) {
			continue;
		}
		if (isInLinkContext(text, schemeStart)) {
			continue;
		}
		const domainEnd = scanDomain(text, schemeStart + match[0].length);
		if (domainEnd === null) {
			continue;
		}
		pushUrl(schemeStart, scanPath(text, domainEnd));
	}
	if (cursor < text.length) {
		segments.push({ kind: "text", value: text.slice(cursor) });
	}
	return segments;
};
