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

	it("does not linkify non-http schemes", () => {
		expect(splitTextForLinks("ftp://host ws://host file:///tmp/x")).toEqual([
			{ kind: "text", value: "ftp://host ws://host file:///tmp/x" },
		]);
	});

	it("trims an unmatched closing bracket wrapping the URL", () => {
		expect(splitTextForLinks("Open [http://localhost:3000] now")).toEqual([
			{ kind: "text", value: "Open [" },
			{ kind: "url", value: "http://localhost:3000" },
			{ kind: "text", value: "] now" },
		]);
	});

	it("trims an unmatched closing bracket after a path", () => {
		expect(splitTextForLinks("[http://localhost:3000/app]")).toEqual([
			{ kind: "text", value: "[" },
			{ kind: "url", value: "http://localhost:3000/app" },
			{ kind: "text", value: "]" },
		]);
	});

	it("keeps IPv6 host brackets while trimming a wrapper bracket", () => {
		expect(splitTextForLinks("[http://[::1]:8080/]")).toEqual([
			{ kind: "text", value: "[" },
			{ kind: "url", value: "http://[::1]:8080/" },
			{ kind: "text", value: "]" },
		]);
	});

	it("stops the URL at ANSI escape sequences", () => {
		expect(
			splitTextForLinks("\u001b[32mhttp://localhost:3000/\u001b[39m done"),
		).toEqual([
			{ kind: "text", value: "\u001b[32m" },
			{ kind: "url", value: "http://localhost:3000/" },
			{ kind: "text", value: "\u001b[39m done" },
		]);
	});

	it("stops the URL at other ASCII control characters", () => {
		expect(splitTextForLinks("http://localhost:3000/a\u0007bell")).toEqual([
			{ kind: "url", value: "http://localhost:3000/a" },
			{ kind: "text", value: "\u0007bell" },
		]);
	});
});
