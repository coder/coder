import type { List, Nodes, Root } from "mdast";
import remarkGfm from "remark-gfm";
import { defaultRemarkPlugins } from "streamdown";
import type { PluggableList, Plugin } from "unified";

const orderedListMarkerPattern = /^[ \t]*(\d{1,9})[.)]/;

const readListItemNumber = (
	source: string,
	offset: number | undefined,
): number | undefined => {
	if (offset === undefined) {
		return undefined;
	}
	const match = orderedListMarkerPattern.exec(
		source.slice(offset, offset + 16),
	);
	return match ? Number(match[1]) : undefined;
};

const keepLiteralNumbers = (list: List, source: string) => {
	if (!list.ordered) {
		return;
	}
	const numbers: number[] = [];
	for (const item of list.children) {
		const number = readListItemNumber(source, item.position?.start.offset);
		if (number === undefined) {
			return;
		}
		numbers.push(number);
	}
	const [first] = numbers;
	// Lists written as "1. 1. 1." ask for automatic numbering, and lists that
	// already count up from their first number render the same either way.
	const isRepeated = numbers.every((number) => number === first);
	const isSequential = numbers.every(
		(number, index) => number === first + index,
	);
	if (isRepeated || isSequential) {
		return;
	}
	for (const [index, item] of list.children.entries()) {
		item.data = {
			...item.data,
			hProperties: {
				...item.data?.hProperties,
				value: String(numbers[index]),
			},
		};
	}
};

const visitLists = (node: Nodes, source: string) => {
	if (node.type === "list") {
		keepLiteralNumbers(node, source);
	}
	if (!("children" in node)) {
		return;
	}
	const children: readonly Nodes[] = node.children;
	for (const child of children) {
		visitLists(child, source);
	}
};

/**
 * Shows each ordered list item with the number written in the source.
 * CommonMark keeps only the first item's number and the browser counts up
 * from it, so a reply that skips a step ("1. 3. 4.") would show 1, 2, 3.
 * The source comes from the file being parsed, because Streamdown parses
 * each block (and, while streaming, remend's repaired text) separately.
 */
export const remarkLiteralListNumbers: Plugin<[], Root> =
	() => (tree, file) => {
		visitLists(tree, String(file));
	};

/**
 * Remark plugins for chat Markdown: Streamdown's defaults without the
 * conveniences that misread chat text.
 */
export const chatRemarkPlugins: PluggableList = [
	// In chat a single tilde means "about" (~5 min) or a range (10~15) far
	// more often than strikethrough, so only ~~double tildes~~ strike text.
	[remarkGfm, { singleTilde: false }],
	defaultRemarkPlugins.codeMeta,
	remarkLiteralListNumbers,
];
