type LinkSegment =
	| { kind: "text"; value: string }
	| { kind: "url"; value: string };

// Length of the case-insensitive http(s) scheme at `at`, or 0.
const schemeLength = (text: string, at: number): number => {
	const slice = text.slice(at, at + 8).toLowerCase();
	if (slice.startsWith("https://")) {
		return 8;
	}
	if (slice.startsWith("http://")) {
		return 7;
	}
	return 0;
};

const isAsciiAlpha = (ch: string): boolean => /[A-Za-z]/.test(ch);
const isWhitespace = (ch: string): boolean => /\s/.test(ch);
// CommonMark's "Unicode punctuation" includes symbols.
const isPunctuation = (ch: string): boolean => /[\p{P}\p{S}]/u.test(ch);
const isAsciiPunctuation = (ch: string | undefined): boolean =>
	ch !== undefined && /[!-/:-@[-`{-~]/.test(ch);
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

interface BacktickRunIndex {
	/** Exclusive end: code spans cannot cross the next blank line. */
	limit: number;
	byLength: Map<number, { positions: number[]; cursor: number }>;
}

const BLANK_LINE_PATTERN = /\n[ \t\r]*\n/g;

/**
 * Indexes the contiguous backtick runs from `from` to the next blank line,
 * grouped by run length. Escapes are deliberately ignored: CommonMark scans
 * for a closing backtick string without processing backslashes.
 */
const buildBacktickRunIndex = (
	text: string,
	from: number,
): BacktickRunIndex => {
	BLANK_LINE_PATTERN.lastIndex = from;
	const boundary = BLANK_LINE_PATTERN.exec(text);
	const limit = boundary ? boundary.index : text.length;
	const byLength = new Map<number, { positions: number[]; cursor: number }>();
	let i = from;
	while (i < limit) {
		if (text[i] === "`") {
			const start = i;
			while (i < limit && text[i] === "`") {
				i += 1;
			}
			const length = i - start;
			let entry = byLength.get(length);
			if (!entry) {
				entry = { positions: [], cursor: 0 };
				byLength.set(length, entry);
			}
			entry.positions.push(start);
		} else {
			i += 1;
		}
	}
	return { limit, byLength };
};

/**
 * First backtick run of exactly `length` at or after `after`, or null.
 * Callers query with non-decreasing positions, so each per-length cursor
 * advances monotonically and lookups stay amortized constant.
 */
const findCloser = (
	index: BacktickRunIndex,
	length: number,
	after: number,
): number | null => {
	const entry = index.byLength.get(length);
	if (!entry) {
		return null;
	}
	while (
		entry.cursor < entry.positions.length &&
		entry.positions[entry.cursor] < after
	) {
		entry.cursor += 1;
	}
	return entry.cursor < entry.positions.length
		? entry.positions[entry.cursor]
		: null;
};

/**
 * Concatenating the returned segment values reproduces the input.
 * The scanning rules are ported from micromark-extension-gfm-autolink-literal
 * (the GFM autolink parser Streamdown uses for agent messages) so prompt
 * links match agent-message links without depending on the markdown parser.
 * Like micromark's text tokenizer, one forward walk attempts each construct
 * (backslash escape, code span, angle-bracket autolink, autolink literal)
 * and consumed regions are mutually exclusive, so URLs inside inline code
 * stay literal. Only explicit http(s) literals become URLs.
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

	// GFM suppresses autolink literals after an unmatched `[` (a pending
	// link label) and inside a `[text](url)` destination; blank lines end
	// the paragraph and reset label state.
	let labelDepth = 0;
	let lastLabelCloseIndex = -1;
	let lineIsBlank = true;
	let inIndent = true;
	let indentColumns = 0;
	let runIndex: BacktickRunIndex | null = null;
	let i = 0;

	// Updates only line state over a region consumed by a code span, which
	// may contain single line endings but never a blank line.
	const passThrough = (from: number, to: number) => {
		for (let k = from; k < to; k++) {
			const ch = text[k];
			if (ch === "\n") {
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
		}
	};

	while (i < text.length) {
		const ch = text[i];
		if (ch === "\n") {
			if (lineIsBlank) {
				labelDepth = 0;
				lastLabelCloseIndex = -1;
			}
			lineIsBlank = true;
			inIndent = true;
			indentColumns = 0;
			i += 1;
			continue;
		}
		if (ch !== " " && ch !== "\t" && ch !== "\r") {
			lineIsBlank = false;
		}
		if (inIndent && (ch === " " || ch === "\t")) {
			indentColumns += ch === "\t" ? 4 - (indentColumns % 4) : 1;
			i += 1;
			continue;
		}
		inIndent = false;
		// A 4+ column indent (tab stop 4) is an indented code block, where
		// GFM does not autolink.
		const codeIndented = indentColumns >= 4;

		if (ch === "\\" && isAsciiPunctuation(text[i + 1])) {
			i += 2;
			continue;
		}

		if (ch === "`") {
			let runEnd = i + 1;
			while (text[runEnd] === "`") {
				runEnd += 1;
			}
			if (!runIndex || i >= runIndex.limit) {
				runIndex = buildBacktickRunIndex(text, i);
			}
			const closer = findCloser(runIndex, runEnd - i, runEnd);
			if (closer !== null) {
				// Code span: skip its content and closer so nothing inside
				// is linkified or tracked as label syntax.
				const spanEnd = closer + (runEnd - i);
				passThrough(runEnd, spanEnd);
				i = spanEnd;
			} else {
				// No closer: the run is literal text.
				i = runEnd;
			}
			continue;
		}

		// CommonMark angle-bracket autolink: `<https://url>` linkifies the
		// inner URL verbatim and the brackets stay text. The URI may contain
		// anything except `<`, `>`, ASCII space, and ASCII control
		// characters.
		if (ch === "<" && !codeIndented) {
			const scheme = schemeLength(text, i + 1);
			if (scheme > 0) {
				let end = i + 1 + scheme;
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
					pushUrl(i + 1, end);
					i = end + 1;
					continue;
				}
			}
			i += 1;
			continue;
		}

		if (ch === "h" || ch === "H") {
			const scheme = schemeLength(text, i);
			const previous = text[i - 1];
			const inLinkContext =
				labelDepth > 0 ||
				(i >= 2 && previous === "(" && lastLabelCloseIndex === i - 2);
			if (
				scheme > 0 &&
				!codeIndented &&
				!inLinkContext &&
				!(previous !== undefined && isAsciiAlpha(previous))
			) {
				const domainEnd = scanDomain(text, i + scheme);
				if (domainEnd !== null) {
					const end = scanPath(text, domainEnd);
					pushUrl(i, end);
					i = end;
					continue;
				}
			}
			i += 1;
			continue;
		}

		if (ch === "[") {
			labelDepth += 1;
		} else if (ch === "]" && labelDepth > 0) {
			labelDepth -= 1;
			lastLabelCloseIndex = i;
		}
		i += 1;
	}
	if (cursor < text.length) {
		segments.push({ kind: "text", value: text.slice(cursor) });
	}
	return segments;
};
