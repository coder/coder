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
		[
			"(`/repo/a.ts`)",
			[
				{ type: "text", text: "(" },
				{ type: "path", path: "/repo/a.ts", isDirectory: false },
				{ type: "text", text: ")" },
			],
		],
		[
			'"`/repo/src/`"',
			[
				{ type: "text", text: '"' },
				{ type: "path", path: "/repo/src", isDirectory: true },
				{ type: "text", text: '"' },
			],
		],
	])("finds a reference inside brackets or quotes: %s", (text, segments) => {
		expect(splitWorkspacePathReferences(text)).toEqual(segments);
	});

	it.each(
		[
			["(", ")"],
			["[", "]"],
			["{", "}"],
			['"', '"'],
			["'", "'"],
			["“", "”"],
			["‘", "’"],
			["(", " and"],
			['("', '")'],
		].flatMap(([opening, closing]): [string, string, string, boolean][] => [
			[opening, closing, "/repo/main.go", false],
			[opening, closing, "/repo/src", true],
		]),
	)(
		"finds a reference after %j and before %j: %s",
		(opening, closing, path, isDirectory) => {
			const reference = formatWorkspacePathReference(path, isDirectory);
			expect(
				splitWorkspacePathReferences(
					`see ${opening}${reference}${closing} next`,
				),
			).toEqual([
				{ type: "text", text: `see ${opening}` },
				{ type: "path", path, isDirectory },
				{ type: "text", text: `${closing} next` },
			]);
		},
	);

	it("finds a reference after a span its boundaries reject", () => {
		expect(
			splitWorkspacePathReferences(
				"Use [`/repo/a.ts`](./a) and `/repo/b.ts` next.",
			),
		).toEqual([
			{ type: "text", text: "Use [`/repo/a.ts`](./a) and " },
			{ type: "path", path: "/repo/b.ts", isDirectory: false },
			{ type: "text", text: " next." },
		]);
	});

	it.each(["(`/repo/a`x ", "`/repo/a`x "])(
		"scans many rejected spans in linear time: %j",
		(unit) => {
			// Each span's closing backtick is glued to a word. A search that runs
			// past a rejected closing backtick takes seconds here.
			const text = unit.repeat(20_000);
			const start = performance.now();
			const segments = splitWorkspacePathReferences(text);
			expect(performance.now() - start).toBeLessThan(1000);
			expect(segments).toEqual([{ type: "text", text }]);
		},
	);

	it.each([
		"run `make gen` first",
		"edit `src/main.go`",
		"x`/repo/a.ts` y",
		"x(`/repo/a.ts`)",
		'x"`/repo/a.ts`"',
		"don't`/repo/a.ts` y",
		"<`/repo/a.ts`>",
		"-`/repo/a.ts`",
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
