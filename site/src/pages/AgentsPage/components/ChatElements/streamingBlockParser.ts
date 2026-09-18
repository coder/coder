import { parseMarkdownIntoBlocks } from "streamdown";

type ParseBlocks = (markdown: string) => string[];

// Two constructs make block splitting depend on text outside the tail:
// streamdown keeps a document with footnote syntax as a single block so
// references resolve across paragraphs, and marked drops a link reference
// definition whose label was already defined earlier. Either one in the
// tail forces a full parse. The label grammar is marked's. The equivalence
// test pins both against streamdown's own parser.
const NEEDS_FULL_PARSE = /\[\^[\w-]{1,200}\]|^ {0,3}\[(?:\\.|[^[\]\\])+\]:/m;

function isBlank(block: string): boolean {
	return block.trim().length === 0;
}

interface Cut {
	/** Number of leading blocks that are complete. */
	count: number;
	/** Complete first line after the completed blocks, including its newline. */
	witness: string;
}

const NO_CUT: Cut = { count: 0, witness: "" };

// A blank line ends every block construct except a list or indented code
// block, and those continue only when the next line is indented. So the
// blocks before a blank block are complete as long as the first line after
// the blank stays the same, and the lexer starts fresh after the blank
// (nothing merges into a blank token). Cutting anywhere else is unsafe:
// marked's paragraph rules look several lines ahead, so text appended later
// can move a boundary between two adjacent non-blank blocks.
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
 * is O(message length) per animation frame while streaming and dominates
 * WebKit's main thread on long replies. Streamed text only grows, so this
 * parser caches the blocks before the last blank line ahead of the open
 * block and re-lexes only the text after it. Any other change, such as a
 * new stream replacing the message, falls back to a full parse.
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
			// Block raws only concatenate back to the source when marked kept
			// every token; CRLF normalization and dropped duplicate link
			// definitions break that, and then offsets mean nothing. Such a
			// message is parsed from scratch on every render.
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
