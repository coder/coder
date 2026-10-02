import type { Nodes } from "mdast";
import remarkParse from "remark-parse";
import remend from "remend";
import { defaultRemarkPlugins } from "streamdown";
import { unified } from "unified";
import { sliceAtGraphemeBoundary, sliceGraphemes } from "./SmoothText";

const DEFAULT_THINKING_TITLE = "Thinking";
const PREVIEW_TITLE_MAX_GRAPHEMES = 100;
// Bounds the Markdown parsing, which reruns for every streamed chunk of
// reasoning that can grow to many kilobytes.
const PARSED_SOURCE_MAX_LENGTH = PREVIEW_TITLE_MAX_GRAPHEMES * 4;

// Uses the Streamdown body's remark setup so titles read Markdown the way the
// body does.
const markdownParser = unified()
	.use(remarkParse)
	.use(Object.values(defaultRemarkPlugins));

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

const getNodeText = (node: Nodes): string => {
	switch (node.type) {
		case "image":
		case "imageReference":
			return node.alt ?? "";
		case "break":
			return " ";
		case "footnoteReference":
			return `[^${node.label ?? node.identifier}]`;
		case "list": {
			const start = node.start ?? 1;
			return node.children
				.map((item, index) => {
					const marker = node.ordered ? `${start + index}.` : "-";
					const checkbox =
						item.checked === true
							? "[x] "
							: item.checked === false
								? "[ ] "
								: "";
					return `${marker} ${checkbox}${getNodeText(item)}`;
				})
				.join(" ");
		}
	}
	if ("value" in node) {
		return node.value;
	}
	if (!("children" in node)) {
		return "";
	}
	const children: readonly Nodes[] = node.children;
	return children
		.map(getNodeText)
		.join(phrasingParentTypes.has(node.type) ? "" : " ");
};

const getPlainText = (node: Nodes): string =>
	getNodeText(node).replace(/\s+/g, " ").trim();

const parseHeadingCandidate = (markdown: string) =>
	markdown.length > PARSED_SOURCE_MAX_LENGTH
		? undefined
		: markdownParser.parse(markdown);

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

const getParagraphHeadingText = (
	lines: readonly LineRange[],
	index: number,
	text: string,
): string | undefined => {
	const lineRange = lines[index];
	const prefix = text.slice(0, lineRange.start);
	if (prefix.trim()) {
		return undefined;
	}

	const [paragraph] = parseHeadingCandidate(lineRange.line)?.children ?? [];
	if (paragraph?.type !== "paragraph") {
		return undefined;
	}
	const [firstChild] = paragraph.children;
	const isEmphasized =
		paragraph.children.length === 1 && firstChild.type === "strong";
	const nextLine = lines[index + 1];
	const hasBody = hasBodyAfterLine(lines, index);
	if ((!nextLine || nextLine.line.trim() || !hasBody) && !isEmphasized) {
		return undefined;
	}

	const heading = getPlainText(isEmphasized ? firstChild : paragraph);
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

// A first heading too long to parse ends the scan, so a later heading cannot
// stand in for it.
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

		if (/^ {0,3}#{1,6}(?:[ \t]|$)/.test(line)) {
			const root = parseHeadingCandidate(line);
			if (!root) {
				return undefined;
			}
			const heading = getPlainText(root);
			if (heading) {
				return {
					text: heading,
					start: lineRange.start,
					end: lineRange.nextStart,
				};
			}
		}

		const paragraphHeading = getParagraphHeadingText(lines, index, text);
		if (paragraphHeading) {
			return {
				text: paragraphHeading,
				start: lineRange.start,
				end: lineRange.nextStart,
			};
		}

		if (setextCandidate && /^ {0,3}(=+|-+)[ \t]*$/.test(line)) {
			// A line's start decides whether an underline makes it a heading, so
			// a line too long to parse is cut to classify the pair.
			const candidate = sliceAtGraphemeBoundary(
				setextCandidate.line,
				PARSED_SOURCE_MAX_LENGTH - line.length - 1,
			);
			const [block] =
				parseHeadingCandidate(`${candidate}\n${line}`)?.children ?? [];
			if (block?.type === "heading") {
				if (candidate !== setextCandidate.line) {
					return undefined;
				}
				const heading = getPlainText(block);
				if (heading) {
					return {
						text: heading,
						start: setextCandidate.start,
						end: lineRange.nextStart,
					};
				}
			}
		}

		setextCandidate = line.trim() ? lineRange : undefined;
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

// A streamed emphasized heading would otherwise show as the preview until
// its closing markup arrives and it becomes the heading.
const isHeadingInProgress = (text: string): boolean =>
	/^(\*\*|__)(?:(?!\1).)*$/.test(text.trimStart());

const getPreviewTitle = (text: string, isStreaming: boolean): string => {
	if (isStreaming && isHeadingInProgress(text)) {
		return "";
	}
	const source = sliceAtGraphemeBoundary(text, PARSED_SOURCE_MAX_LENGTH);
	const isSourceCut = source.length < text.length;
	// Streamed or cut text can stop inside markup, which the streaming body
	// repairs the same way.
	const preview = getPlainText(
		markdownParser.parse(isStreaming || isSourceCut ? remend(source) : source),
	);
	const title = sliceGraphemes(preview, PREVIEW_TITLE_MAX_GRAPHEMES);
	if (!preview || (title === preview && !isSourceCut)) {
		return preview;
	}
	return `${title.trimEnd()}…`;
};

export const getThinkingDisclosureDisplay = (
	text: string,
	{ isStreaming }: { isStreaming: boolean },
): ThinkingDisclosureDisplay => {
	const heading = getFirstHeading(text);
	if (!heading) {
		const preview = getPreviewTitle(text, isStreaming);
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
