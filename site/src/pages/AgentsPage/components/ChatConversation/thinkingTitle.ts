import { sliceAtGraphemeBoundary } from "./SmoothText";

const DEFAULT_THINKING_TITLE = "Thinking";
const PREVIEW_TITLE_MAX_LENGTH = 100;
// Bounds the cleanup work: the preview is recomputed for every streamed
// chunk of reasoning, which can grow to many kilobytes.
const PREVIEW_SOURCE_MAX_LENGTH = PREVIEW_TITLE_MAX_LENGTH * 4;

type LineRange = {
	line: string;
	start: number;
	nextStart: number;
};

type HeadingMatch = {
	text: string;
	start: number;
	end: number;
};

type ThinkingDisclosureDisplay = {
	title: string;
	/** Accessible name for titles that do not say "Thinking" themselves. */
	ariaLabel?: string;
	body: string;
};

// CommonMark drops one space of padding from each side of a code span.
const getCodeSpanText = (code: string): string => {
	const text = code.replace(/\r\n?|\n/g, " ");
	return text.startsWith(" ") && text.endsWith(" ") && /[^ ]/.test(text)
		? text.slice(1, -1)
		: text;
};

const cleanHeadingText = (text: string): string => {
	// Backslash escapes and code spans render literally, so they are set aside
	// while the rules below run. One pass keeps their Markdown precedence, and
	// unmatched backtick runs are consumed whole so no code span starts inside
	// one. HTML-like text and spaced asterisks (`2 * n * m`) also stay literal.
	const literals: string[] = [];
	const setAside = (literal: string): string => {
		literals.push(literal);
		return `\uE000${literals.length - 1}\uE001`;
	};
	return text
		.replace(
			/\\([!-/:-@[-`{-~])|(`+)([^`]|[^`][\s\S]*?[^`])\2(?!`)|`+/g,
			(match, escaped?: string, _fence?: string, code?: string) => {
				if (escaped !== undefined) {
					return setAside(escaped);
				}
				return code === undefined ? match : setAside(getCodeSpanText(code));
			},
		)
		.replace(/!\[([^\]]*)\]\([^)]*\)/g, "$1")
		.replace(/\[([^\]]+)\]\([^)]*\)/g, "$1")
		.replace(/\*\*([^*\s](?:[^*]*[^*\s])?)\*\*/g, "$1")
		.replace(/\b__([^_]+)__\b/g, "$1")
		.replace(/\*([^*\s](?:[^*]*[^*\s])?)\*/g, "$1")
		.replace(/\b_([^_]+)_\b/g, "$1")
		.replace(/~~([^~]+)~~/g, "$1")
		.replace(
			/\uE000(\d+)\uE001/g,
			(match, index: string) => literals[Number(index)] ?? match,
		)
		.replace(/\s+/g, " ")
		.trim();
};

const getLines = (text: string): LineRange[] => {
	const lines: LineRange[] = [];
	const linePattern = /[^\r\n]*(?:\r\n|\r|\n|$)/g;

	for (const match of text.matchAll(linePattern)) {
		const rawLine = match[0];
		const start = match.index ?? 0;
		if (rawLine === "" && start === text.length) {
			break;
		}

		const line = rawLine.replace(/\r\n$|\r$|\n$/, "");
		lines.push({
			line,
			start,
			nextStart: start + rawLine.length,
		});
	}

	return lines;
};

const getAtxHeadingText = (line: string): string | undefined => {
	const match = line.match(/^ {0,3}#{1,6}(?:[ \t]+|$)(.*)$/);
	if (!match) {
		return undefined;
	}

	const heading = cleanHeadingText(match[1].replace(/[ \t]+#{1,}[ \t]*$/, ""));
	return heading || undefined;
};

const getFenceMarker = (
	line: string,
): { character: "`" | "~"; length: number } | undefined => {
	const match = line.match(/^ {0,3}(`{3,}|~{3,})/);
	if (!match) {
		return undefined;
	}

	const marker = match[1];
	return {
		character: marker[0] as "`" | "~",
		length: marker.length,
	};
};

const isClosingFence = (
	line: string,
	marker: { character: "`" | "~"; length: number },
): boolean => {
	const match = line.match(/^ {0,3}(`{3,}|~{3,})[ \t]*$/);
	return (
		!!match &&
		match[1][0] === marker.character &&
		match[1].length >= marker.length
	);
};

const hasBodyAfterLine = (
	lines: readonly LineRange[],
	index: number,
): boolean => lines.slice(index + 1).some(({ line }) => line.trim().length > 0);

const isListItem = (line: string): boolean =>
	/^ {0,3}(?:[-+*]|\d{1,9}[.)])(?:[ \t]|$)/.test(line);

const getEmphasizedLineHeadingText = (line: string): string | undefined => {
	const match = line.match(/^ {0,3}(?:\*\*([^*]+)\*\*|__([^_]+)__)[ \t]*$/);
	if (!match) {
		return undefined;
	}

	const heading = cleanHeadingText(match[1] ?? match[2] ?? "");
	return heading || undefined;
};

const isHeadingLikeParagraph = (
	lines: readonly LineRange[],
	index: number,
	text: string,
): string | undefined => {
	const lineRange = lines[index];
	const prefix = text.slice(0, lineRange.start);
	if (prefix.trim() || isListItem(lineRange.line)) {
		return undefined;
	}

	const emphasizedHeading = getEmphasizedLineHeadingText(lineRange.line);
	const nextLine = lines[index + 1];
	const hasBody = hasBodyAfterLine(lines, index);
	if ((!nextLine || nextLine.line.trim() || !hasBody) && !emphasizedHeading) {
		return undefined;
	}

	const heading = emphasizedHeading ?? cleanHeadingText(lineRange.line);
	if (!heading) {
		return undefined;
	}

	const wordCount = heading.split(/\s+/).length;
	if (heading.length > 96 || wordCount > 12 || /[.!?]$/.test(heading)) {
		return undefined;
	}

	if (
		/^[a-z]/.test(heading) ||
		/^(I|I'm|I’m|We|We're|We’re|Let's|Let’s)\b/.test(heading)
	) {
		return undefined;
	}

	return heading;
};

const getFirstHeading = (text: string): HeadingMatch | undefined => {
	let activeFence: { character: "`" | "~"; length: number } | undefined;
	let setextCandidate: LineRange | undefined;
	const lines = getLines(text);

	for (const [index, lineRange] of lines.entries()) {
		const { line } = lineRange;
		if (activeFence) {
			if (isClosingFence(line, activeFence)) {
				activeFence = undefined;
			}
			continue;
		}

		const openingFence = getFenceMarker(line);
		if (openingFence) {
			activeFence = openingFence;
			setextCandidate = undefined;
			continue;
		}

		const atxHeading = getAtxHeadingText(line);
		if (atxHeading) {
			return {
				text: atxHeading,
				start: lineRange.start,
				end: lineRange.nextStart,
			};
		}

		const paragraphHeading = isHeadingLikeParagraph(lines, index, text);
		if (paragraphHeading) {
			return {
				text: paragraphHeading,
				start: lineRange.start,
				end: lineRange.nextStart,
			};
		}

		if (/^ {0,3}(=+|-+)[ \t]*$/.test(line) && setextCandidate) {
			const heading = cleanHeadingText(setextCandidate.line);
			if (!heading) {
				return undefined;
			}
			return {
				text: heading,
				start: setextCandidate.start,
				end: lineRange.nextStart,
			};
		}

		setextCandidate = line.trim() && !isListItem(line) ? lineRange : undefined;
	}

	return undefined;
};

const lowercaseSentenceStart = (text: string): string => {
	const firstWord = text.match(/^[A-Za-z]+\b/)?.[0];
	if (!firstWord || !/^[A-Z][a-z]+$/.test(firstWord)) {
		return text;
	}

	return `${firstWord[0].toLowerCase()}${text.slice(1)}`;
};

const removeHeading = (text: string, heading: HeadingMatch): string => {
	const beforeHeading = text.slice(0, heading.start);
	const body = `${beforeHeading}${text.slice(heading.end)}`;
	if (beforeHeading.trim()) {
		return body;
	}
	return body.replace(/^\s+/, "");
};

const getPreviewTitle = (text: string): string => {
	const source = sliceAtGraphemeBoundary(text, PREVIEW_SOURCE_MAX_LENGTH);
	const preview = cleanHeadingText(source);
	const isSourceCut = source.length < text.length;
	if (
		!preview ||
		(preview.length <= PREVIEW_TITLE_MAX_LENGTH && !isSourceCut)
	) {
		return preview;
	}
	return `${sliceAtGraphemeBoundary(preview, PREVIEW_TITLE_MAX_LENGTH).trimEnd()}…`;
};

// A streamed heading arrives before its closing markup, so its opening
// markup would otherwise flash as the preview title.
const isHeadingInProgress = (text: string): boolean =>
	/^(?:#{1,6}[ \t]*|(\*\*|__)(?:(?!\1).)*)$/.test(text.trimStart());

export const getThinkingDisclosureDisplay = (
	text: string,
	{ isStreaming = false }: { isStreaming?: boolean } = {},
): ThinkingDisclosureDisplay => {
	const heading = getFirstHeading(text);
	if (!heading) {
		const preview =
			isStreaming && isHeadingInProgress(text) ? "" : getPreviewTitle(text);
		if (!preview) {
			return { title: DEFAULT_THINKING_TITLE, body: text };
		}
		return {
			title: preview,
			ariaLabel: `${DEFAULT_THINKING_TITLE}: ${preview}`,
			body: text,
		};
	}

	return {
		title: `${DEFAULT_THINKING_TITLE} about ${lowercaseSentenceStart(heading.text)}`,
		body: removeHeading(text, heading),
	};
};
