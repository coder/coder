import { describe, expect, it } from "vitest";
import { getHeadLine, isAddedInDiff } from "./useFileTabRefresh";

describe("getHeadLine", () => {
	// `git diff HEAD` and `git diff -U0 HEAD` after inserting two lines
	// after line 2, deleting line 12 and changing line 25 of 30.
	const header =
		"diff --git a/f.txt b/f.txt\nindex ac9837c..f2e5cda 100644\n--- a/f.txt\n+++ b/f.txt\n";
	const diffs = {
		"3 context lines": `${header}@@ -1,5 +1,7 @@
 line 1
 line 2
+new A
+new B
 line 3
 line 4
 line 5
@@ -9,7 +11,6 @@ line 8
 line 9
 line 10
 line 11
-line 12
 line 13
 line 14
 line 15
@@ -22,7 +23,7 @@ line 21
 line 22
 line 23
 line 24
-line 25
+changed 25
 line 26
 line 27
 line 28
`,
		"no context": `${header}@@ -2,0 +3,2 @@ line 2
+new A
+new B
@@ -12 +13,0 @@ line 11
-line 12
@@ -25 +26 @@ line 24
-line 25
+changed 25
`,
	};

	describe.each(Object.entries(diffs))("with %s", (_, diff) => {
		it.each([
			[1, 1],
			[2, 2],
			[3, undefined],
			[4, undefined],
			[5, 3],
			[13, 11],
			[14, 13],
			[20, 19],
			[25, 24],
			[26, undefined],
			[27, 26],
			[31, 30],
		])("maps line %i on disk to line %s in HEAD", (line, headLine) => {
			expect(getHeadLine(diff, "f.txt", line)).toBe(headLine);
		});
	});

	it("keeps the line of a file the diff does not change", () => {
		expect(getHeadLine(diffs["3 context lines"], "g.txt", 7)).toBe(7);
		expect(getHeadLine("", "f.txt", 7)).toBe(7);
	});

	it("maps no line of a diff it cannot read", () => {
		expect(
			getHeadLine(
				"Total diff too large to show. Size: 4 MiB. Showing branch and remote only.",
				"f.txt",
				7,
			),
		).toBeUndefined();
	});

	it("finds a file whose path git quotes", () => {
		const diff =
			'diff --git "a/caf\\303\\251.txt" "b/caf\\303\\251.txt"\nindex de98044..c5924bc 100644\n--- "a/caf\\303\\251.txt"\n+++ "b/caf\\303\\251.txt"\n@@ -1,3 +1,4 @@\n a\n+new\n b\n c\n';
		expect(getHeadLine(diff, "café.txt", 3)).toBe(2);
		expect(
			isAddedInDiff(
				'diff --git "a/na\\303\\257ve \\"q\\".txt" "b/na\\303\\257ve \\"q\\".txt"\nnew file mode 100644\nindex 0000000..1e2f3c4\n--- /dev/null\n+++ "b/na\\303\\257ve \\"q\\".txt"\t\n@@ -0,0 +1 @@\n+n\n',
				'naïve "q".txt',
			),
		).toBe(true);
	});

	it("maps no line of a file whose change has no hunks to read", () => {
		expect(
			getHeadLine(
				"diff --git a/img.bin b/img.bin\nindex bdc955b..8835708 100644\nBinary files a/img.bin and b/img.bin differ\n",
				"img.bin",
				1,
			),
		).toBeUndefined();
		expect(
			getHeadLine(
				"diff --git a/run.txt b/run.txt\nold mode 100644\nnew mode 100755\n",
				"run.txt",
				4,
			),
		).toBe(4);
	});
});
