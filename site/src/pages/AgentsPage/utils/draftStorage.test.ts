import { describe, expect, it } from "vitest";
import { parseStoredDraft } from "./draftStorage";

describe("parseStoredDraft", () => {
	it("returns empty text and no editorState for null input", () => {
		const result = parseStoredDraft(null);
		expect(result.text).toBe("");
		expect(result.editorState).toBeUndefined();
	});

	it("returns empty text and no editorState for empty string", () => {
		const result = parseStoredDraft("");
		expect(result.text).toBe("");
		expect(result.editorState).toBeUndefined();
	});

	it("treats a plain text string as a legacy draft", () => {
		const result = parseStoredDraft("hello world");
		expect(result.text).toBe("hello world");
		expect(result.editorState).toBeUndefined();
	});

	it("treats valid JSON without a root key as a legacy draft", () => {
		const raw = JSON.stringify({ foo: "bar" });
		const result = parseStoredDraft(raw);
		expect(result.text).toBe(raw);
		expect(result.editorState).toBeUndefined();
	});

	it("treats JSON with a root key but no type as a legacy draft", () => {
		const raw = JSON.stringify({ root: true });
		const result = parseStoredDraft(raw);
		expect(result.text).toBe(raw);
		expect(result.editorState).toBeUndefined();
	});

	it("detects Lexical editor state JSON (has root key)", () => {
		const state = JSON.stringify({
			root: {
				children: [
					{
						children: [{ text: "hello", type: "text" }],
						type: "paragraph",
					},
				],
				type: "root",
			},
		});
		const result = parseStoredDraft(state);
		expect(result.editorState).toBe(state);
		expect(result.text).toBe("hello");
	});

	it("extracts text from multiple paragraphs joined by newlines", () => {
		const state = JSON.stringify({
			root: {
				children: [
					{
						children: [{ text: "line one", type: "text" }],
						type: "paragraph",
					},
					{
						children: [{ text: "line two", type: "text" }],
						type: "paragraph",
					},
				],
				type: "root",
			},
		});
		const result = parseStoredDraft(state);
		expect(result.text).toBe("line one\n\nline two");
	});

	it("skips non-text nodes (file-reference chips)", () => {
		const state = JSON.stringify({
			root: {
				children: [
					{
						children: [
							{ text: "review ", type: "text" },
							{
								type: "file-reference",
								fileName: "main.go",
								startLine: 1,
								endLine: 10,
								content: "code",
							},
							{ text: " please", type: "text" },
						],
						type: "paragraph",
					},
				],
				type: "root",
			},
		});
		const result = parseStoredDraft(state);
		expect(result.text).toBe("review  please");
	});

	it("handles empty root children", () => {
		const state = JSON.stringify({
			root: { children: [], type: "root" },
		});
		const result = parseStoredDraft(state);
		expect(result.text).toBe("");
		expect(result.editorState).toBe(state);
	});

	it("handles paragraphs with no children", () => {
		const state = JSON.stringify({
			root: {
				children: [{ type: "paragraph" }],
				type: "root",
			},
		});
		const result = parseStoredDraft(state);
		expect(result.text).toBe("");
		expect(result.editorState).toBe(state);
	});

	it("extracts text from deeply nested structures", () => {
		const state = JSON.stringify({
			root: {
				children: [
					{
						children: [
							{
								children: [{ text: "nested", type: "text" }],
								type: "listitem",
							},
						],
						type: "list",
					},
				],
				type: "root",
			},
		});
		const result = parseStoredDraft(state);
		expect(result.text).toBe("nested");
	});

	it("extracts linebreak nodes as newline characters", () => {
		const state = JSON.stringify({
			root: {
				children: [
					{
						children: [
							{ text: "before", type: "text" },
							{ type: "linebreak", version: 1 },
							{ text: "after", type: "text" },
						],
						type: "paragraph",
					},
				],
				type: "root",
			},
		});
		const result = parseStoredDraft(state);
		expect(result.text).toBe("before\nafter");
	});

	it("handles malformed JSON gracefully", () => {
		const result = parseStoredDraft("{not valid json");
		expect(result.text).toBe("{not valid json");
		expect(result.editorState).toBeUndefined();
	});
});
