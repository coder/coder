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

/** Caches isTrail verdicts by position so each character is classified once. */
type TrailMemo = Map<number, boolean>;

/**
 * Whether everything from index up to the next URL end (whitespace, `<`, or
 * end of text) is trailing punctuation that GFM excludes from the link.
 * The verdict is a pure suffix property, so every construct-start position
 * visited on the way shares the final result; memoizing them keeps repeated
 * checks over long punctuation runs linear instead of quadratic.
 */
const isTrail = (text: string, index: number, memo: TrailMemo): boolean => {
	const pending: number[] = [];
	let i = index;
	let result: boolean | undefined;
	while (result === undefined) {
		const known = memo.get(i);
		if (known !== undefined) {
			result = known;
			break;
		}
		const ch = text[i];
		if (ch === undefined || ch === "<" || isWhitespace(ch)) {
			memo.set(i, true);
			result = true;
			break;
		}
		if (TRAIL_CHARS.has(ch)) {
			pending.push(i);
			i += 1;
			continue;
		}
		// A trailing entity reference (`&amp;`) also counts as punctuation.
		if (ch === "&") {
			pending.push(i);
			let k = i + 1;
			if (!isAsciiAlpha(text[k] ?? "")) {
				result = false;
				break;
			}
			while (isAsciiAlpha(text[k] ?? "")) {
				k += 1;
			}
			if (text[k] !== ";") {
				result = false;
				break;
			}
			i = k + 1;
			continue;
		}
		// `]` ends the URL when followed by an end or by `(`/`[`, which could
		// start a markdown resource or reference.
		if (ch === "]") {
			pending.push(i);
			const next = text[i + 1];
			if (
				next === undefined ||
				next === "(" ||
				next === "[" ||
				isWhitespace(next)
			) {
				result = true;
				break;
			}
			i += 1;
			continue;
		}
		memo.set(i, false);
		result = false;
		break;
	}
	for (const position of pending) {
		memo.set(position, result);
	}
	return result;
};

/**
 * Scans a GFM domain (host) after the `://`. Returns its exclusive end, or
 * null when invalid: empty, starting with punctuation or a control
 * character, or containing `_` in the last two dot-separated segments.
 */
const scanDomain = (
	text: string,
	start: number,
	memo: TrailMemo,
): number | null => {
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
			if (isTrail(text, i, memo)) {
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
const scanPath = (text: string, start: number, memo: TrailMemo): number => {
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
			if (isTrail(text, i, memo)) {
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

interface FenceRegion {
	start: number;
	end: number;
}

/**
 * Fenced code block regions: an opening line of 3+ backticks or tildes
 * indented at most 3 columns, closed by a line of at least as many of the
 * same character with nothing else on it, or running to the end of text.
 * Backtick-fence info strings cannot contain a backtick. GFM does not
 * autolink inside fenced code, and an opening line interrupts a paragraph.
 */
const computeFenceRegions = (text: string): FenceRegion[] => {
	const regions: FenceRegion[] = [];
	let open: { char: string; length: number; start: number } | null = null;
	let lineStart = 0;
	while (lineStart < text.length) {
		let lineEnd = text.indexOf("\n", lineStart);
		if (lineEnd === -1) {
			lineEnd = text.length;
		}
		let columns = 0;
		let i = lineStart;
		while (i < lineEnd && (text[i] === " " || text[i] === "\t")) {
			columns += text[i] === "\t" ? 4 - (columns % 4) : 1;
			i += 1;
		}
		const ch = text[i];
		if (columns <= 3 && (ch === "`" || ch === "~")) {
			let runEnd = i;
			while (runEnd < lineEnd && text[runEnd] === ch) {
				runEnd += 1;
			}
			if (runEnd - i >= 3) {
				if (open) {
					let k = runEnd;
					while (
						k < lineEnd &&
						(text[k] === " " || text[k] === "\t" || text[k] === "\r")
					) {
						k += 1;
					}
					if (ch === open.char && runEnd - i >= open.length && k === lineEnd) {
						regions.push({
							start: open.start,
							end: Math.min(lineEnd + 1, text.length),
						});
						open = null;
					}
				} else if (ch === "~" || !text.slice(runEnd, lineEnd).includes("`")) {
					open = { char: ch, length: runEnd - i, start: lineStart };
				}
			}
		}
		lineStart = lineEnd + 1;
	}
	if (open) {
		regions.push({ start: open.start, end: text.length });
	}
	return regions;
};

interface BacktickRunIndex {
	/**
	 * Exclusive end: code spans cannot cross the next blank line or fence
	 * opener, both of which end the paragraph.
	 */
	limit: number;
	byLength: Map<number, { positions: number[]; cursor: number }>;
}

const BLANK_LINE_PATTERN = /\n[ \t\r]*\n/g;

/**
 * Indexes the contiguous backtick runs from `from` to the next blank line
 * or `capAt`, grouped by run length. Escapes are deliberately ignored:
 * CommonMark scans for a closing backtick string without processing
 * backslashes.
 */
const buildBacktickRunIndex = (
	text: string,
	from: number,
	capAt: number,
): BacktickRunIndex => {
	BLANK_LINE_PATTERN.lastIndex = from;
	const boundary = BLANK_LINE_PATTERN.exec(text);
	const limit = Math.min(boundary ? boundary.index : text.length, capAt);
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
 * and fenced code blocks stay literal. Only explicit http(s) literals
 * become URLs.
 */
export const splitTextForLinks = (text: string): LinkSegment[] => {
	const segments: LinkSegment[] = [];
	const trailMemo: TrailMemo = new Map();
	const fenceRegions = computeFenceRegions(text);
	let fenceCursor = 0;
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
		while (
			fenceCursor < fenceRegions.length &&
			fenceRegions[fenceCursor].end <= i
		) {
			fenceCursor += 1;
		}
		const fence = fenceRegions[fenceCursor];
		if (fence !== undefined && i >= fence.start) {
			// Fenced code: nothing inside is linkified, and its opening line
			// interrupts the paragraph. The region ends at a line start.
			labelDepth = 0;
			lastLabelCloseIndex = -1;
			lineIsBlank = true;
			inIndent = true;
			indentColumns = 0;
			i = fence.end;
			fenceCursor += 1;
			continue;
		}
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
				runIndex = buildBacktickRunIndex(text, i, fence?.start ?? text.length);
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
				const domainEnd = scanDomain(text, i + scheme, trailMemo);
				if (domainEnd !== null) {
					const end = scanPath(text, domainEnd, trailMemo);
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
