import { splitTextForLinks } from "./linkify";

describe("splitTextForLinks", () => {
	it("returns a single text segment when there are no URLs", () => {
		expect(splitTextForLinks("compiled 12 files in 340ms")).toEqual([
			{ kind: "text", value: "compiled 12 files in 340ms" },
		]);
	});

	it("extracts a bare URL surrounded by text", () => {
		expect(splitTextForLinks("Local: http://localhost:3000/ ready")).toEqual([
			{ kind: "text", value: "Local: " },
			{ kind: "url", value: "http://localhost:3000/" },
			{ kind: "text", value: " ready" },
		]);
	});

	it("extracts multiple URLs and preserves whitespace between them", () => {
		expect(
			splitTextForLinks(
				"  ➜  Local:   http://localhost:5173/\n  ➜  Network: http://127.0.0.1:5173/\n",
			),
		).toEqual([
			{ kind: "text", value: "  ➜  Local:   " },
			{ kind: "url", value: "http://localhost:5173/" },
			{ kind: "text", value: "\n  ➜  Network: " },
			{ kind: "url", value: "http://127.0.0.1:5173/" },
			{ kind: "text", value: "\n" },
		]);
	});

	it("keeps ports, paths, query strings, and fragments", () => {
		expect(
			splitTextForLinks("see https://localhost:8080/api/v2?q=1&x=2#frag now"),
		).toEqual([
			{ kind: "text", value: "see " },
			{ kind: "url", value: "https://localhost:8080/api/v2?q=1&x=2#frag" },
			{ kind: "text", value: " now" },
		]);
	});

	it("excludes trailing sentence punctuation from the URL", () => {
		expect(splitTextForLinks("Open http://localhost:3000.")).toEqual([
			{ kind: "text", value: "Open " },
			{ kind: "url", value: "http://localhost:3000" },
			{ kind: "text", value: "." },
		]);
		expect(splitTextForLinks("Ready at http://localhost:3000!?")).toEqual([
			{ kind: "text", value: "Ready at " },
			{ kind: "url", value: "http://localhost:3000" },
			{ kind: "text", value: "!?" },
		]);
	});

	it("excludes trailing Markdown emphasis delimiters from the URL", () => {
		expect(splitTextForLinks("**https://coder.com/docs**")).toEqual([
			{ kind: "text", value: "**" },
			{ kind: "url", value: "https://coder.com/docs" },
			{ kind: "text", value: "**" },
		]);
		expect(
			splitTextForLinks("_https://coder.com/blog_ and ~https://coder.com/x~"),
		).toEqual([
			{ kind: "text", value: "_" },
			{ kind: "url", value: "https://coder.com/blog" },
			{ kind: "text", value: "_ and ~" },
			{ kind: "url", value: "https://coder.com/x" },
			{ kind: "text", value: "~" },
		]);
	});

	it("excludes unmatched closing wrappers from the URL", () => {
		expect(splitTextForLinks("(listening on http://localhost:3000)")).toEqual([
			{ kind: "text", value: "(listening on " },
			{ kind: "url", value: "http://localhost:3000" },
			{ kind: "text", value: ")" },
		]);
		expect(splitTextForLinks("Open [http://localhost:3000] now")).toEqual([
			{ kind: "text", value: "Open [" },
			{ kind: "url", value: "http://localhost:3000" },
			{ kind: "text", value: "] now" },
		]);
	});

	it("keeps balanced closing pairs inside the URL", () => {
		expect(
			splitTextForLinks("docs at https://example.com/wiki/Foo_(bar)"),
		).toEqual([
			{ kind: "text", value: "docs at " },
			{ kind: "url", value: "https://example.com/wiki/Foo_(bar)" },
		]);
		expect(splitTextForLinks("dev http://[::1]:8080/x up")).toEqual([
			{ kind: "text", value: "dev " },
			{ kind: "url", value: "http://[::1]:8080/x" },
			{ kind: "text", value: " up" },
		]);
	});

	it("matches mixed-case schemes and preserves their text", () => {
		expect(
			splitTextForLinks("HTTPS://coder.com/docs and Http://localhost:3000/app"),
		).toEqual([
			{ kind: "url", value: "HTTPS://coder.com/docs" },
			{ kind: "text", value: " and " },
			{ kind: "url", value: "Http://localhost:3000/app" },
		]);
	});

	it("does not linkify non-http schemes", () => {
		expect(splitTextForLinks("ftp://host ws://host file:///tmp/x")).toEqual([
			{ kind: "text", value: "ftp://host ws://host file:///tmp/x" },
		]);
	});

	it("does not linkify bare domains or filenames with TLD-like extensions", () => {
		expect(
			splitTextForLinks("please edit main.ts and README.md then run deploy.sh"),
		).toEqual([
			{
				kind: "text",
				value: "please edit main.ts and README.md then run deploy.sh",
			},
		]);
		expect(splitTextForLinks("see github.com and www.coder.com")).toEqual([
			{ kind: "text", value: "see github.com and www.coder.com" },
		]);
	});

	it("does not linkify email addresses", () => {
		expect(splitTextForLinks("contact admin@coder.com about chat.go")).toEqual([
			{ kind: "text", value: "contact admin@coder.com about chat.go" },
		]);
	});

	it("does not linkify a scheme glued to a preceding word", () => {
		expect(splitTextForLinks("myhttp://thing is custom")).toEqual([
			{ kind: "text", value: "myhttp://thing is custom" },
		]);
	});

	it("linkifies angle-bracket-wrapped URLs and keeps the brackets as text", () => {
		expect(splitTextForLinks("see <https://coder.com/docs> now")).toEqual([
			{ kind: "text", value: "see <" },
			{ kind: "url", value: "https://coder.com/docs" },
			{ kind: "text", value: "> now" },
		]);
	});

	// Prompts are plain text, not markdown: URLs become links uniformly,
	// with no special-casing of markdown syntax around them.

	it("linkifies the destination inside markdown link syntax", () => {
		expect(
			splitTextForLinks("read [policy](http://localhost:3000/policy) first"),
		).toEqual([
			{ kind: "text", value: "read [policy](" },
			{ kind: "url", value: "http://localhost:3000/policy" },
			{ kind: "text", value: ") first" },
		]);
	});

	it("linkifies URLs wrapped in backticks and keeps the backticks as text", () => {
		expect(splitTextForLinks("`http://localhost:3000`")).toEqual([
			{ kind: "text", value: "`" },
			{ kind: "url", value: "http://localhost:3000" },
			{ kind: "text", value: "`" },
		]);
	});

	it("linkifies URLs inside code fences and indented lines", () => {
		expect(splitTextForLinks("```\nhttp://localhost:3000\n```")).toEqual([
			{ kind: "text", value: "```\n" },
			{ kind: "url", value: "http://localhost:3000" },
			{ kind: "text", value: "\n```" },
		]);
		expect(splitTextForLinks("    http://localhost:3000/pasted")).toEqual([
			{ kind: "text", value: "    " },
			{ kind: "url", value: "http://localhost:3000/pasted" },
		]);
	});

	it("linkifies URLs inside blockquoted fenced code", () => {
		expect(splitTextForLinks("> ~~~\n> http://localhost:3000\n> ~~~")).toEqual([
			{ kind: "text", value: "> ~~~\n> " },
			{ kind: "url", value: "http://localhost:3000" },
			{ kind: "text", value: "\n> ~~~" },
		]);
	});

	it("ends URLs at quotes and ASCII control characters", () => {
		expect(splitTextForLinks('served "http://localhost:3000/a" fine')).toEqual([
			{ kind: "text", value: 'served "' },
			{ kind: "url", value: "http://localhost:3000/a" },
			{ kind: "text", value: '" fine' },
		]);
		expect(splitTextForLinks("http://localhost:3000/a\u0007bell")).toEqual([
			{ kind: "url", value: "http://localhost:3000/a" },
			{ kind: "text", value: "\u0007bell" },
		]);
	});

	it("does not linkify a URL with its host trimmed away", () => {
		expect(splitTextForLinks("weird http://. case")).toEqual([
			{ kind: "text", value: "weird http://. case" },
		]);
	});

	it("stays fast on long inputs", () => {
		const urls = Array.from(
			{ length: 10000 },
			(_, i) => `http://localhost:${3000 + (i % 100)}/x`,
		).join(" ");
		const dots = `http://localhost:3000/${".".repeat(10000)}x`;
		const start = performance.now();
		expect(splitTextForLinks(urls)).toHaveLength(19999);
		expect(splitTextForLinks(dots)).toEqual([{ kind: "url", value: dots }]);
		expect(performance.now() - start).toBeLessThan(1000);
	});
});
