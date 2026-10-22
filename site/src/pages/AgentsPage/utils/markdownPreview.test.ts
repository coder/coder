import { describe, expect, it } from "vitest";
import {
	groupMarkdownBlocks,
	isWorkspaceImageSource,
	resolvePreviewLink,
	resolveWorkspaceUrl,
} from "./markdownPreview";

describe("resolvePreviewLink", () => {
	it.each([
		["https://coder.com", { kind: "external" }],
		["mailto:a@b.c", { kind: "external" }],
		["//cdn.example.com/x", { kind: "external" }],
		["#quickstart", { kind: "anchor", id: "quickstart" }],
		["#why%20coder", { kind: "anchor", id: "why coder" }],
		["./LICENSE", { kind: "file", path: "/repo/docs/LICENSE" }],
		["x.md#L12", { kind: "file", path: "/repo/docs/x.md", line: 12 }],
		["x.md#section", { kind: "file", path: "/repo/docs/x.md" }],
		["../README.md?plain=1", { kind: "file", path: "/repo/README.md" }],
		["/CONTRIBUTING.md", { kind: "file", path: "/repo/CONTRIBUTING.md" }],
		["my%20notes.md", { kind: "file", path: "/repo/docs/my notes.md" }],
	])("classifies %j", (href, expected) => {
		expect(resolvePreviewLink(href, "/repo/docs", "/repo")).toEqual(expected);
	});

	it("resolves root-relative links against the filesystem outside a repository", () => {
		expect(resolveWorkspaceUrl("/etc/hosts", "/home/coder", undefined)).toBe(
			"/etc/hosts",
		);
	});
});

describe("isWorkspaceImageSource", () => {
	it.each([
		["./docs/logo.png", true],
		["logo.png", true],
		["/docs/logo.png", true],
		["https://img.shields.io/x", false],
		["data:image/png;base64,AAAA", false],
		["", false],
	])("classifies %j", (src, expected) => {
		expect(isWorkspaceImageSource(src)).toBe(expected);
	});
});

describe("groupMarkdownBlocks", () => {
	it.each([
		{
			name: "keeps a small file in one chunk",
			blocks: ["# A\n\n", "text\n"],
			size: 100,
			expected: ["# A\n\ntext\n"],
		},
		{
			name: "starts a chunk before a block that would overflow",
			blocks: ["aaaa\n\n", "bbbb\n\n", "cccc\n"],
			size: 12,
			expected: ["aaaa\n\nbbbb\n\n", "cccc\n"],
		},
		{
			name: "keeps a block larger than the chunk size whole",
			blocks: ["a".repeat(30), "b\n"],
			size: 10,
			expected: ["a".repeat(30), "b\n"],
		},
		{
			name: "repeats link definitions in every chunk",
			blocks: ["[ref]: ./x.md\n\n", "See [x][ref].\n"],
			size: 10,
			expected: [
				"[ref]: ./x.md\n\n\n\n[ref]: ./x.md\n",
				"See [x][ref].\n\n\n[ref]: ./x.md\n",
			],
		},
	])("$name", ({ blocks, size, expected }) => {
		expect(groupMarkdownBlocks(blocks.join(""), blocks, size)).toEqual(
			expected,
		);
	});
});
