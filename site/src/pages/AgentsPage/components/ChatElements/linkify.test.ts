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

	it("keeps ports, paths, and query strings", () => {
		expect(
			splitTextForLinks("see https://localhost:8080/api/v2?q=1&x=2 now"),
		).toEqual([
			{ kind: "text", value: "see " },
			{ kind: "url", value: "https://localhost:8080/api/v2?q=1&x=2" },
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

	it("excludes a closing parenthesis that is not part of the URL", () => {
		expect(splitTextForLinks("(listening on http://localhost:3000)")).toEqual([
			{ kind: "text", value: "(listening on " },
			{ kind: "url", value: "http://localhost:3000" },
			{ kind: "text", value: ")" },
		]);
	});

	it("keeps a balanced closing parenthesis inside the URL", () => {
		expect(
			splitTextForLinks("docs at https://example.com/wiki/Foo_(bar)"),
		).toEqual([
			{ kind: "text", value: "docs at " },
			{ kind: "url", value: "https://example.com/wiki/Foo_(bar)" },
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

	it("linkifies angle-bracket autolinks and keeps the brackets as text", () => {
		expect(splitTextForLinks("see <https://coder.com/docs> now")).toEqual([
			{ kind: "text", value: "see <" },
			{ kind: "url", value: "https://coder.com/docs" },
			{ kind: "text", value: "> now" },
		]);
	});

	it("keeps non-http angle-bracket autolinks as text", () => {
		expect(splitTextForLinks("<mailto:x@y.com> and <ftp://host>")).toEqual([
			{ kind: "text", value: "<mailto:x@y.com> and <ftp://host>" },
		]);
	});

	it("keeps markdown link syntax as literal text", () => {
		expect(
			splitTextForLinks("read [policy](http://localhost:3000/policy) first"),
		).toEqual([
			{
				kind: "text",
				value: "read [policy](http://localhost:3000/policy) first",
			},
		]);
	});

	it("does not apply markdown block formatting to the surrounding text", () => {
		expect(splitTextForLinks("# fix this http://localhost:3000/x")).toEqual([
			{ kind: "text", value: "# fix this " },
			{ kind: "url", value: "http://localhost:3000/x" },
		]);
	});

	// These cases intentionally follow GFM autolink behavior without
	// custom recovery.

	it("does not linkify a bracket-wrapped URL", () => {
		expect(splitTextForLinks("Open [http://localhost:3000] now")).toEqual([
			{ kind: "text", value: "Open [http://localhost:3000] now" },
		]);
	});

	it("does not linkify inside a four-space-indented line", () => {
		expect(splitTextForLinks("    http://localhost:3000/pasted")).toEqual([
			{ kind: "text", value: "    http://localhost:3000/pasted" },
		]);
	});

	it("does not detect URLs with IPv6 literal hosts", () => {
		expect(splitTextForLinks("[http://[::1]:8080/]")).toEqual([
			{ kind: "text", value: "[http://[::1]:8080/]" },
		]);
	});

	it("does not linkify URLs adjacent to ANSI escape sequences", () => {
		expect(
			splitTextForLinks("\u001b[32mhttp://localhost:3000/\u001b[39m done"),
		).toEqual([
			{
				kind: "text",
				value: "\u001b[32mhttp://localhost:3000/\u001b[39m done",
			},
		]);
	});

	it("keeps ASCII control characters inside the URL", () => {
		expect(splitTextForLinks("http://localhost:3000/a\u0007bell")).toEqual([
			{ kind: "url", value: "http://localhost:3000/a\u0007bell" },
		]);
	});
});
