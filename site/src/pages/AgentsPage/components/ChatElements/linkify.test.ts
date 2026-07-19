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
});
