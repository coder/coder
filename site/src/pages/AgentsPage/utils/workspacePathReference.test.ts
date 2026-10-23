import { describe, expect, it } from "vitest";
import {
	formatWorkspacePathReference,
	splitWorkspacePathReferences,
} from "./workspacePathReference";

describe("splitWorkspacePathReferences", () => {
	it("finds file and folder references in the form chips send", () => {
		const text = `see ${formatWorkspacePathReference("/repo/main.go", false)} and ${formatWorkspacePathReference("~/repo/src", true)}`;
		expect(splitWorkspacePathReferences(text)).toEqual([
			{ type: "text", text: "see " },
			{ type: "path", path: "/repo/main.go", isDirectory: false },
			{ type: "text", text: " and " },
			{ type: "path", path: "~/repo/src", isDirectory: true },
		]);
	});

	it("finds a reference that is the whole line", () => {
		expect(splitWorkspacePathReferences("`/repo/a.ts`\nnext")).toEqual([
			{ type: "path", path: "/repo/a.ts", isDirectory: false },
			{ type: "text", text: "\nnext" },
		]);
	});

	it.each([
		["/repo/My File.ts", false],
		["/repo/two  spaces /dir", true],
		["/repo/a`b.ts", false],
		["/repo/ends-with`", false],
		["/repo/``double``/x", true],
		["~/notes/line\nbreak.md", false],
	])("round-trips %j", (path, isDirectory) => {
		const reference = formatWorkspacePathReference(path, isDirectory);
		expect(splitWorkspacePathReferences(`open ${reference} now`)).toEqual([
			{ type: "text", text: "open " },
			{ type: "path", path, isDirectory },
			{ type: "text", text: " now" },
		]);
	});

	it("finds a folder reference that ends a sentence", () => {
		expect(
			splitWorkspacePathReferences(
				"look in `/home/coder/websocket/internal/`.",
			),
		).toEqual([
			{ type: "text", text: "look in " },
			{
				type: "path",
				path: "/home/coder/websocket/internal",
				isDirectory: true,
			},
			{ type: "text", text: "." },
		]);
	});

	it.each(
		[
			".",
			",",
			";",
			":",
			"!",
			"?",
			")",
			"]",
			"}",
			'"',
			"'",
			"”",
			"’",
			").",
			"?!",
		].flatMap((punctuation): [string, string, boolean][] => [
			[punctuation, "/repo/main.go", false],
			[punctuation, "/repo/src", true],
		]),
	)(
		"finds a reference followed by %j: %s",
		(punctuation, path, isDirectory) => {
			const reference = formatWorkspacePathReference(path, isDirectory);
			expect(
				splitWorkspacePathReferences(`see ${reference}${punctuation} next`),
			).toEqual([
				{ type: "text", text: "see " },
				{ type: "path", path, isDirectory },
				{ type: "text", text: `${punctuation} next` },
			]);
		},
	);

	it.each([
		"run `make gen` first",
		"edit `src/main.go`",
		"(`/repo/a.ts`)",
		"x`/repo/a.ts` y",
		"`/repo/a`b` is not one span",
		"`/repo/a`b",
		"`/repo/a.ts`.md",
		"`/repo/a.ts`x.",
		"`/repo/a.ts`-",
		"` /repo/a.ts `",
		"`/` and `~/`",
	])("leaves other inline code alone: %s", (text) => {
		expect(splitWorkspacePathReferences(text)).toEqual([
			{ type: "text", text },
		]);
	});
});
