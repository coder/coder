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

interface UrlContext {
	/**
	 * The line's leading indentation is 4+ columns (tab stop 4): an indented
	 * code block, where GFM does not autolink.
	 */
	codeIndented: boolean;
	/**
	 * GFM suppresses autolinks after an unmatched `[` (a pending link label)
	 * and inside a `[text](url)` destination. Blank lines end the paragraph
	 * and reset label state.
	 */
	inLinkContext: boolean;
}

/**
 * Incremental scanner for the markdown context GFM consults at each autolink
 * candidate. advanceTo must be called with non-decreasing indexes; each
 * character is examined once, keeping linkification linear in prompts whose
 * paragraphs contain many URLs.
 */
const createContextScanner = (text: string) => {
	let scanned = 0;
	let labelDepth = 0;
	let lastLabelCloseIndex = -1;
	let lineIsBlank = true;
	let inIndent = true;
	let indentColumns = 0;
	return (index: number): UrlContext => {
		for (; scanned < index; scanned++) {
			const ch = text[scanned];
			if (ch === "\n") {
				if (lineIsBlank) {
					labelDepth = 0;
					lastLabelCloseIndex = -1;
				}
				lineIsBlank = true;
				inIndent = true;
				indentColumns = 0;
				continue;
			}
			if (ch !== " " && ch !== "\t" && ch !== "\r") {
				lineIsBlank = false;
			}
			if (inIndent && (ch === " " || ch === "\t")) {
				indentColumns += ch === "\t" ? 4 - (indentColumns % 4) : 1;
			} else {
				inIndent = false;
			}
			if (ch === "[") {
				labelDepth += 1;
			} else if (ch === "]" && labelDepth > 0) {
				labelDepth -= 1;
				lastLabelCloseIndex = scanned;
			}
		}
		return {
			codeIndented: indentColumns >= 4,
			inLinkContext:
				labelDepth > 0 ||
				(index >= 2 &&
					text[index - 1] === "(" &&
					lastLabelCloseIndex === index - 2),
		};
	};
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
	const contextAt = createContextScanner(text);
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
		const context = contextAt(schemeStart);
		if (context.codeIndented) {
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
		if (context.inLinkContext) {
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
