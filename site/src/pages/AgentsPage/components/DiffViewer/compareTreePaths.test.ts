import { describe, expect, it } from "vitest";
import { compareTreePaths } from "./DiffViewer";

describe("compareTreePaths", () => {
	it("orders directories before sibling files and keeps dot-prefixed first", () => {
		const sorted = [".config/a.ts", "b.ts", "a/z.ts", "b/c/d.ts"].sort(
			compareTreePaths,
		);

		expect(sorted).toEqual([".config/a.ts", "a/z.ts", "b/c/d.ts", "b.ts"]);
	});

	it("sorts dot-prefixed names before other names at the same level", () => {
		expect([".env", "app.ts", ".gitignore"].sort(compareTreePaths)).toEqual([
			".env",
			".gitignore",
			"app.ts",
		]);
	});

	it("breaks ties case-insensitively", () => {
		expect(["Beta.ts", "alpha.ts", "Alpha.ts"].sort(compareTreePaths)).toEqual([
			"alpha.ts",
			"Alpha.ts",
			"Beta.ts",
		]);
	});

	it("is a stable total order regardless of input order", () => {
		const files = [
			"src/zeta.ts",
			"src/alpha/index.ts",
			"README.md",
			".github/workflows/ci.yml",
			"src/alpha.ts",
		];
		const forward = [...files].sort(compareTreePaths);
		const reversed = [...files].reverse().sort(compareTreePaths);
		expect(reversed).toEqual(forward);
		expect(forward).toEqual([
			".github/workflows/ci.yml",
			"src/alpha/index.ts",
			"src/alpha.ts",
			"src/zeta.ts",
			"README.md",
		]);
	});
});
