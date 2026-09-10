import { describe, expect, it } from "vitest";
import { isMarkdownFileName } from "./markdownFile";

describe("isMarkdownFileName", () => {
	it.each([
		["README.md", true],
		["docs/guide.MD", true],
		["notes.markdown", true],
		["a/b/c/CHANGELOG.Markdown", true],
		["README", false],
		["component.mdx", false],
		["main.go", false],
		["md", false],
		["", false],
		["archive.md.bak", false],
	])("returns %s for %s", (fileName, expected) => {
		expect(isMarkdownFileName(fileName)).toBe(expected);
	});
});
