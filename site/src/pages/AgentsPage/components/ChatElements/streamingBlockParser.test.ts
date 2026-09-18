import { parseMarkdownIntoBlocks } from "streamdown";
import { createStreamingBlockParser } from "./streamingBlockParser";

const prose = [
	"# Build pipeline",
	"",
	"The build pipeline reads every workspace template and resolves provisioner",
	"jobs. Agents report their lifecycle state through the control plane so that",
	"dashboards stay accurate.",
	"",
	"## Steps",
	"",
	"1. Resolve the template version",
	"2. Plan the workspace build",
	"   with a continuation line",
	"",
	"   and a paragraph inside the item after a blank line",
	"3. Apply the plan",
	"",
	"- unordered item",
	"lazy continuation of the item",
	"- second item",
	"",
	"Setext heading",
	"==============",
	"",
	"Another setext",
	"---",
	"",
	"> quoted line one",
	"quoted lazy continuation",
	"",
	"| col a | col b |",
	"| ----- | ----- |",
	"| 1     | 2     |",
	"",
	"```ts",
	"const x = 1;",
	"",
	"console.log(x);",
	"```",
	"",
	"Paragraph with `inline code` and **bold** text, then a [link](https://example.com).",
	"",
	"    indented code block",
	"",
	"    continues after a blank line",
	"",
	"***",
	"",
	"<details>",
	"<summary>Details</summary>",
	"",
	"Hidden paragraph inside html.",
	"",
	"</details>",
	"",
	"$$",
	"a^2 + b^2 = c^2",
	"$$",
	"",
	"- list before blank",
	"",
	"  indented continuation after the blank line",
	"",
	"not indented, ends the list",
	"",
	"[ref]: https://example.com/ref",
	"[ref]: https://example.com/duplicate",
	"[a\\]b]: https://example.com/escaped",
	"[a\\]b]: https://example.com/escaped-duplicate",
	"",
	"Text using [ref] and a trailing paragraph without newline",
].join("\n");

const footnotes = [
	"Intro paragraph.",
	"",
	"A claim with a footnote[^1] in the middle.",
	"",
	"Second paragraph.",
	"",
	"[^1]: The footnote text.",
	"",
	"Closing paragraph.",
].join("\n");

const unclosedFence = [
	"Before the fence.",
	"",
	"```",
	"still open",
	"",
	"more lines that look like",
	"# a heading",
	"- and a list",
].join("\n");

const crlf =
	"First line\r\n\r\nSecond paragraph\r\n\r\n- item\r\n- item two\r\n";

const corpora = { prose, footnotes, unclosedFence, crlf };

// Suffixes that remend appends or that a stream may produce at the end of
// a frame, applied and then removed between renders so the tail is not a
// pure extension of the previous input.
const tailRewrites = [
	"**",
	"\n```",
	"`",
	"]",
	"\n\n",
	"  ",
	"\n",
	"-",
	"\n===",
];

// Line-level constructs whose boundaries depend on neighbouring lines:
// lazy continuations, setext underlines, table delimiter rows, list
// markers that cannot interrupt a paragraph, blank-line continuations.
const fuzzPieces = [
	"para one\n",
	"para two words\n",
	"\n",
	"\n\n",
	"# H1\n",
	"===\n",
	"---\n",
	"***\n",
	"- item\n",
	"1. one\n",
	"2. two\n",
	"  cont after blank\n",
	"    code after blank\n",
	"  - nested\n",
	"> quote\n",
	"| a | b |\n",
	"| - | - |\n",
	"```\n",
	"~~~\n",
	"$$\n",
	"<div>\n",
	"</div>\n",
	"<!-- c -->\n",
	"<script>\n",
	"[ref]: http://x\n",
	"[^1]\n",
	"[^1]: note\n",
	"**bold\n",
	"lazy\n",
	"  \n",
	"-\n",
	"1)\n",
	"\r\n",
];

function seededRandom(seed: number): () => number {
	let state = seed >>> 0;
	return () => {
		state = (state * 1664525 + 1013904223) >>> 0;
		return state / 2 ** 32;
	};
}

describe("createStreamingBlockParser", () => {
	it.each(Object.entries(corpora))(
		"matches streamdown for every prefix of %s",
		(_name, text) => {
			const parse = createStreamingBlockParser();
			for (let end = 0; end <= text.length; end++) {
				const prefix = text.slice(0, end);
				expect(parse(prefix)).toEqual(parseMarkdownIntoBlocks(prefix));
			}
		},
	);

	it.each(tailRewrites)(
		"matches streamdown when the tail is rewritten with %j between renders",
		(suffix) => {
			const parse = createStreamingBlockParser();
			for (let end = 0; end <= prose.length; end++) {
				const prefix = prose.slice(0, end);
				const rewritten = `${prefix}${suffix}`;
				expect(parse(rewritten)).toEqual(parseMarkdownIntoBlocks(rewritten));
				expect(parse(prefix)).toEqual(parseMarkdownIntoBlocks(prefix));
			}
		},
	);

	it("matches streamdown for random construct sequences", () => {
		const random = seededRandom(12345);
		for (let doc = 0; doc < 300; doc++) {
			const pieceCount = 3 + Math.floor(random() * 20);
			let text = "";
			for (let i = 0; i < pieceCount; i++) {
				text += fuzzPieces[Math.floor(random() * fuzzPieces.length)];
			}
			const parse = createStreamingBlockParser();
			for (let end = 0; end <= text.length; end++) {
				const prefix = text.slice(0, end);
				if (random() < 0.3) {
					const rewritten = `${prefix}${tailRewrites[Math.floor(random() * tailRewrites.length)]}`;
					expect(parse(rewritten), rewritten).toEqual(
						parseMarkdownIntoBlocks(rewritten),
					);
				}
				expect(parse(prefix), prefix).toEqual(parseMarkdownIntoBlocks(prefix));
			}
		}
	});

	it("matches streamdown when the message is replaced", () => {
		const parse = createStreamingBlockParser();
		parse(prose);
		const replaced = prose.replace("Build pipeline", "Different title");
		expect(parse(replaced)).toEqual(parseMarkdownIntoBlocks(replaced));
		expect(parse("short")).toEqual(parseMarkdownIntoBlocks("short"));
		expect(parse("")).toEqual(parseMarkdownIntoBlocks(""));
	});

	it("re-lexes only the trailing blocks once leading blocks are complete", () => {
		const lengths: number[] = [];
		const parse = createStreamingBlockParser((markdown) => {
			lengths.push(markdown.length);
			return parseMarkdownIntoBlocks(markdown);
		});

		const streamed = prose.slice(0, prose.indexOf("[ref]:"));
		for (let end = 0; end <= streamed.length; end++) {
			parse(streamed.slice(0, end));
		}

		// After the first blocks complete, each render lexes the previous
		// block plus the open tail. The corpus's longest block is far
		// shorter than the whole text.
		const longestBlock = Math.max(
			...parseMarkdownIntoBlocks(streamed).map((block) => block.length),
		);
		const tailLengths = lengths.slice(Math.floor(lengths.length / 2));
		expect(Math.max(...tailLengths)).toBeLessThanOrEqual(longestBlock * 3);
		expect(Math.max(...tailLengths)).toBeLessThan(streamed.length / 2);
	});
});
