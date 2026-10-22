import { describe, expect, it } from "vitest";
import { getTabHints } from "./tabHints";

describe("getTabHints", () => {
	it("gives no hint to a unique basename", () => {
		expect(getTabHints(["/repo/main.go", "/repo/README.md"])).toEqual(
			new Map(),
		);
	});

	it("uses the differing parent directory", () => {
		expect(getTabHints(["/repo/a/index.ts", "/repo/b/index.ts"])).toEqual(
			new Map([
				["/repo/a/index.ts", "a"],
				["/repo/b/index.ts", "b"],
			]),
		);
	});

	it("skips directories the paths share", () => {
		expect(
			getTabHints([
				"/home/coder/coder/coderd/x/chatd/chatd.go",
				"/home/coder/coder/enterprise/coderd/x/chatd/chatd.go",
			]),
		).toEqual(
			new Map([
				["/home/coder/coder/coderd/x/chatd/chatd.go", "coderd/…"],
				[
					"/home/coder/coder/enterprise/coderd/x/chatd/chatd.go",
					"enterprise/…",
				],
			]),
		);
	});

	it("prefers the directory nearest the file", () => {
		expect(
			getTabHints([
				"/r/web/src/app.ts",
				"/r/web/test/app.ts",
				"/r/api/src/app.ts",
			]),
		).toEqual(
			new Map([
				["/r/web/src/app.ts", "web/src"],
				["/r/web/test/app.ts", "…/test"],
				["/r/api/src/app.ts", "api/…"],
			]),
		);
	});

	it("names the common ancestor for a file directly in it", () => {
		expect(getTabHints(["/repo/README.md", "/repo/docs/README.md"])).toEqual(
			new Map([
				["/repo/README.md", "repo"],
				["/repo/docs/README.md", "docs"],
			]),
		);
	});
});
