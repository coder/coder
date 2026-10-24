import { parseMarkdownIntoBlocks } from "streamdown";

type ParseBlocks = (markdown: string) => string[];

// Footnotes make Streamdown keep the document as one block, and marked drops
// a repeated link reference definition, so either in the tail forces a full
// parse. Labels follow https://github.com/markedjs/marked/blob/v17.0.5/src/rules.ts#L120
const NEEDS_FULL_PARSE = /\[\^[\w-]{1,200}\]|^ {0,3}\[(?:\\.|[^[\]\\])+\]:/m;

function isBlank(block: string): boolean {
	return block.trim().length === 0;
}

type Cut = {
	/** Number of leading blocks that are complete. */
	count: number;
	/** Complete first line after the completed blocks, including its newline. */
	witness: string;
};

const NO_CUT: Cut = { count: 0, witness: "" };

// Blocks before a blank block are final while the line after that blank
// block is unchanged. A cut between two non-blank blocks is unsafe because
// marked's paragraph rules look ahead.
function findCut(markdown: string, blocks: readonly string[]): Cut {
	let open = blocks.length - 1;
	while (open >= 0 && isBlank(blocks[open])) {
		open--;
	}
	let end = 0;
	for (let i = 0; i < open; i++) {
		end += blocks[i].length;
	}
	for (let i = open - 1; i >= 0; i--) {
		if (isBlank(blocks[i])) {
			const newline = markdown.indexOf("\n", end);
			if (newline >= 0) {
				return { count: i + 1, witness: markdown.slice(end, newline + 1) };
			}
		}
		end -= blocks[i].length;
	}
	return NO_CUT;
}

/**
 * Returns a `parseMarkdownIntoBlocksFn` for one growing message. The result
 * equals `parse(markdown)`; input that does not start with the cached
 * complete blocks gets a full parse.
 */
export function createStreamingBlockParser(
	parse: ParseBlocks = parseMarkdownIntoBlocks,
): ParseBlocks {
	let completedBlocks: string[] = [];
	let completedText = "";
	let witness = "";

	const remember = (
		markdown: string,
		blocks: string[],
		sharesPrefix: boolean,
	) => {
		const cut = findCut(markdown, blocks);
		if (!sharesPrefix || cut.count !== completedBlocks.length) {
			completedBlocks = blocks.slice(0, cut.count);
			completedText = completedBlocks.join("");
			// Block raws concatenate back to the source only when marked did
			// not normalize or drop input; CRLF and duplicate link definitions
			// break that. Such a message gets a full parse on every render.
			if (!markdown.startsWith(completedText)) {
				completedBlocks = [];
				completedText = "";
				witness = "";
				return;
			}
		}
		witness = cut.witness;
	};

	return (markdown) => {
		if (markdown.startsWith(completedText)) {
			const rest = markdown.slice(completedText.length);
			if (rest.startsWith(witness) && !NEEDS_FULL_PARSE.test(rest)) {
				const blocks = completedBlocks.concat(parse(rest));
				remember(markdown, blocks, true);
				return blocks;
			}
		}

		const blocks = parse(markdown);
		remember(markdown, blocks, false);
		return blocks;
	};
}
