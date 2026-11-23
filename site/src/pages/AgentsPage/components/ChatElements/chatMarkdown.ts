import type { List, ListItem, Nodes, Root } from "mdast";
import remarkGfm from "remark-gfm";
import { defaultRemarkPlugins } from "streamdown";
import type { Data, PluggableList, Plugin } from "unified";

type FromMarkdownExtension = Exclude<
	NonNullable<Data["fromMarkdownExtensions"]>[number],
	readonly unknown[]
>;

// mdast keeps only the first item's number, as the list's start, so the
// number the parser read for each item is kept here until the transform.
const writtenListItemNumbers = new WeakMap<ListItem, number>();

const listItemNumbersFromMarkdown: FromMarkdownExtension = {
	exit: {
		listItemValue(token) {
			const item = this.stack.at(-1);
			if (item?.type === "listItem") {
				writtenListItemNumbers.set(
					item,
					Number.parseInt(this.sliceSerialize(token), 10),
				);
			}
		},
	},
};

const keepWrittenNumbers = (list: List) => {
	if (!list.ordered) {
		return;
	}
	const numbers: number[] = [];
	for (const item of list.children) {
		const number = writtenListItemNumbers.get(item);
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

const visitLists = (node: Nodes) => {
	if (node.type === "list") {
		keepWrittenNumbers(node);
	}
	if (!("children" in node)) {
		return;
	}
	const children: readonly Nodes[] = node.children;
	for (const child of children) {
		visitLists(child);
	}
};

/**
 * Shows each ordered list item with the number written in the source.
 * CommonMark keeps only the first item's number and the browser counts up
 * from it, so a reply that skips a step ("1. 3. 4.") would show 1, 2, 3.
 */
const remarkLiteralListNumbers: Plugin<[], Root> = function () {
	const data = this.data();
	data.fromMarkdownExtensions ??= [];
	data.fromMarkdownExtensions.push(listItemNumbersFromMarkdown);
	return (tree) => {
		visitLists(tree);
	};
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
