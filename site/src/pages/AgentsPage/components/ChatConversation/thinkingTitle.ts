import remarkGfm from "remark-gfm";
import remarkParse from "remark-parse";
import remend from "remend";
import { unified } from "unified";
import { sliceAtGraphemeBoundary } from "./SmoothText";

const DEFAULT_THINKING_TITLE = "Thinking";
const PREVIEW_TITLE_MAX_LENGTH = 100;
// Bounds the Markdown parsing, which reruns for every streamed chunk of
// reasoning that can grow to many kilobytes.
const PARSED_SOURCE_MAX_LENGTH = PREVIEW_TITLE_MAX_LENGTH * 4;

// Parses like the Streamdown body so titles keep exactly the text it renders.
const markdownParser = unified().use(remarkParse).use(remarkGfm);

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

type MarkdownNode = {
	type: string;
	value?: string;
	alt?: string | null;
	ordered?: boolean | null;
	start?: number | null;
	children?: MarkdownNode[];
};

const phrasingParentTypes = new Set([
	"paragraph",
	"heading",
	"emphasis",
	"strong",
	"delete",
	"link",
	"linkReference",
	"tableCell",
]);

const getNodeText = (node: MarkdownNode): string => {
	if (node.type === "image" || node.type === "imageReference") {
		return node.alt ?? "";
	}
	if (node.type === "break") {
		return " ";
	}
	if (node.value !== undefined) {
		return node.value;
	}
	const children = node.children ?? [];
	if (node.type === "list") {
		const start = node.start ?? 1;
		return children
			.map((item, index) => {
				const marker = node.ordered ? `${start + index}.` : "-";
				return `${marker} ${getNodeText(item)}`;
			})
			.join(" ");
	}
	return children
		.map(getNodeText)
		.join(phrasingParentTypes.has(node.type) ? "" : " ");
};

const getPlainText = (node: MarkdownNode): string =>
	getNodeText(node).replace(/\s+/g, " ").trim();

const cleanHeadingText = (markdown: string): string =>
	getPlainText(markdownParser.parse(markdown));

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
	if (!/^ {0,3}#{1,6}(?:[ \t]|$)/.test(line)) {
		return undefined;
	}
	return cleanHeadingText(line) || undefined;
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
	const [paragraph] = markdownParser.parse(line).children;
	if (paragraph?.type !== "paragraph" || paragraph.children.length !== 1) {
		return undefined;
	}
	const [strong] = paragraph.children;
	return strong.type === "strong"
		? getPlainText(strong) || undefined
		: undefined;
};

const isHeadingLikeParagraph = (
	lines: readonly LineRange[],
	index: number,
	text: string,
): string | undefined => {
	const lineRange = lines[index];
	const prefix = text.slice(0, lineRange.start);
	if (
		prefix.trim() ||
		isListItem(lineRange.line) ||
		lineRange.line.length > PARSED_SOURCE_MAX_LENGTH
	) {
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

const getPreviewTitle = (text: string, isStreaming: boolean): string => {
	const source = sliceAtGraphemeBoundary(text, PARSED_SOURCE_MAX_LENGTH);
	const isSourceCut = source.length < text.length;
	// Streamed or cut text can stop inside markup, which the streaming body
	// repairs the same way.
	const preview = cleanHeadingText(
		isStreaming || isSourceCut ? remend(source) : source,
	);
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
			isStreaming && isHeadingInProgress(text)
				? ""
				: getPreviewTitle(text, isStreaming);
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
