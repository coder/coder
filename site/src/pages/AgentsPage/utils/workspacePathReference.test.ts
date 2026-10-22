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
		"run `make gen` first",
		"edit `src/main.go`",
		"the `/usr/bin/env python` line",
		"see `/repo/a.ts`.",
		"(`/repo/a.ts`)",
		"x`/repo/a.ts` y",
		"`/` and `~/`",
	])("leaves other inline code alone: %s", (text) => {
		expect(splitWorkspacePathReferences(text)).toEqual([
			{ type: "text", text },
		]);
	});
});
