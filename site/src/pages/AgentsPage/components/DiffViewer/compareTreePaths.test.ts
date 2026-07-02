import { describe, expect, it } from "vitest";
import { compareTreePaths, treeSortComparator } from "./DiffViewer";

// Mirrors how @pierre/trees feeds its sort comparator: it sorts one flat list
// of every path entry (files and the intermediate directories), then builds
// the tree. Expands file paths into that entry set so a test can sort it with
// treeSortComparator and read back the resulting file (leaf) order.
function treeFileOrder(files: readonly string[]): string[] {
	const entries = new Map<
		string,
		{
			basename: string;
			depth: number;
			isDirectory: boolean;
			path: string;
			segments: string[];
		}
	>();
	for (const file of files) {
		const segments = file.split("/");
		for (let i = 0; i < segments.length; i++) {
			const slice = segments.slice(0, i + 1);
			const path = slice.join("/");
			entries.set(path, {
				basename: slice[slice.length - 1],
				depth: slice.length,
				isDirectory: i < segments.length - 1,
				path,
				segments: slice,
			});
		}
	}
	return [...entries.values()]
		.sort(treeSortComparator)
		.filter((entry) => !entry.isDirectory)
		.map((entry) => entry.path);
}

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

	it("matches the tree's leaf order so the diff and sidebar stay in sync", () => {
		const files = [
			"src/zeta.ts",
			"lib/a.ts",
			"src/alpha/index.ts",
			"README.md",
			".github/workflows/ci.yml",
			"src/alpha.ts",
			"lib/b/c.ts",
		];
		// The sidebar tree (treeSortComparator over the full entry set) and the
		// flat diff list (compareTreePaths) must produce the same file order.
		expect([...files].sort(compareTreePaths)).toEqual(treeFileOrder(files));
	});
});
