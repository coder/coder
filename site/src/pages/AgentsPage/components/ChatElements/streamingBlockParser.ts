import { parseMarkdownIntoBlocks } from "streamdown";

type ParseBlocks = (markdown: string) => string[];

// Two constructs make block splitting depend on text before the tail:
// Streamdown keeps a document with footnote syntax as one block, and marked
// drops a link reference definition whose label was defined earlier. Either
// one in the tail forces a full parse. The label pattern is marked's.
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

// A blank line ends every block except a list or indented code block, and
// those continue only when the next line is indented. So the blocks up to a
// blank block are complete while the first line after it stays the same,
// and nothing merges into a blank token. A cut between two non-blank blocks
// is unsafe: marked's paragraph rules look several lines ahead, so appended
// text can move that boundary.
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
 * Creates a `parseMarkdownIntoBlocksFn` for one streaming message.
 *
 * Streamdown re-splits the entire text into blocks on every render, which
 * costs time proportional to the message length on every animation frame
 * while streaming. Streamed text only grows, so this parser caches the
 * blocks up to the last blank block before the open block and re-lexes only
 * the text after them. Any other change, such as a new stream replacing the
 * message, gets a full parse.
 *
 * The result is identical to `parse(markdown)`.
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
