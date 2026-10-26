import { parseMarkdownIntoBlocks } from "streamdown";

type ParseBlocks = (markdown: string) => string[];

// Streamdown's footnote check (the first pattern) keeps the document as one
// block, and marked drops a repeated link definition, so either in the tail
// forces a full parse. Labels follow https://github.com/markedjs/marked/blob/v17.0.5/src/rules.ts#L120
const NEEDS_FULL_PARSE =
	/\[\^[\w-]{1,200}\]|^ {0,3}\[(?:\\[\s\S]|[^[\]\\])+\]:/m;

// A link definition label can span blank lines, so a cut must not leave one
// open at the end of the cached text.
const ENDS_IN_OPEN_LABEL = /^ {0,3}\[(?:\\[\s\S]|[^[\]\\])*(?![\s\S])/m;

function isBlank(block: string): boolean {
	return block.trim().length === 0;
}

type Cut = {
	/** Number of leading blocks that are complete. */
	count: number;
	/** First line after the complete blocks, including its newline. */
	lineAfterCut: string;
};

const NO_CUT: Cut = { count: 0, lineAfterCut: "" };

// Blocks before a blank block are complete while the line after that blank
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
				return { count: i + 1, lineAfterCut: markdown.slice(end, newline + 1) };
			}
		}
		end -= blocks[i].length;
	}
	return NO_CUT;
}

/**
 * Returns a `parseMarkdownIntoBlocksFn` for one growing message. The result
 * equals `parse(markdown)`. It re-lexes only the text after the cached
 * complete blocks, and parses the whole input when that is not safe.
 */
export function createStreamingBlockParser(
	parse: ParseBlocks = parseMarkdownIntoBlocks,
): ParseBlocks {
	let completeBlocks: string[] = [];
	let completeText = "";
	let lineAfterCut = "";

	const remember = (
		markdown: string,
		blocks: string[],
		sharesPrefix: boolean,
	) => {
		const cut = findCut(markdown, blocks);
		if (sharesPrefix && cut.count === completeBlocks.length) {
			lineAfterCut = cut.lineAfterCut;
			return;
		}
		const text = blocks.slice(0, cut.count).join("");
		if (ENDS_IN_OPEN_LABEL.test(text)) {
			// Keep the previous cut, which is still a prefix of this input.
			if (!sharesPrefix) {
				completeBlocks = [];
				completeText = "";
				lineAfterCut = "";
			}
			return;
		}
		completeBlocks = blocks.slice(0, cut.count);
		completeText = text;
		lineAfterCut = cut.lineAfterCut;
	};

	return (markdown) => {
		// Block raws do not concatenate back to CRLF input or to input with a
		// dropped duplicate link definition, so such input gets a full parse.
		if (markdown.startsWith(completeText)) {
			const rest = markdown.slice(completeText.length);
			if (rest.startsWith(lineAfterCut) && !NEEDS_FULL_PARSE.test(rest)) {
				const blocks = completeBlocks.concat(parse(rest));
				remember(markdown, blocks, true);
				return blocks;
			}
		}

		const blocks = parse(markdown);
		remember(markdown, blocks, false);
		return blocks;
	};
}
