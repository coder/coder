import { describe, expect, it } from "vitest";
import { getHeadLine } from "./useFileTabRefresh";

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
});
