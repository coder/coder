import {
	expandFileDiff,
	isExpandable,
	MAX_EXPANDABLE_FILE_SIZE,
} from "./expandFileDiff";

const simplePatch = `diff --git a/test.ts b/test.ts
index abc1234..def5678 100644
--- a/test.ts
+++ b/test.ts
@@ -1,5 +1,5 @@
 line 1
 line 2
-old line 3
+new line 3
 line 4
 line 5
`;

const oldContents = "line 1\nline 2\nold line 3\nline 4\nline 5\n";
const newContents = "line 1\nline 2\nnew line 3\nline 4\nline 5\n";

const newFilePatch = `diff --git a/newfile.ts b/newfile.ts
new file mode 100644
index 0000000..abc1234
--- /dev/null
+++ b/newfile.ts
@@ -0,0 +1,3 @@
+line 1
+line 2
+line 3
`;

const deletedFilePatch = `diff --git a/deleted.ts b/deleted.ts
deleted file mode 100644
index abc1234..0000000
--- a/deleted.ts
+++ /dev/null
@@ -1,3 +0,0 @@
-line 1
-line 2
-line 3
`;

describe("expandFileDiff", () => {
	it("returns FileDiffMetadata with isPartial false", () => {
		const result = expandFileDiff(
			"test.ts",
			simplePatch,
			oldContents,
			newContents,
		);
		expect(result).not.toBeNull();
		expect(result!.isPartial).toBe(false);
	});

	it("handles new file (null old contents)", () => {
		const result = expandFileDiff(
			"newfile.ts",
			newFilePatch,
			null,
			"line 1\nline 2\nline 3\n",
		);
		expect(result).not.toBeNull();
		expect(result!.isPartial).toBe(false);
	});

	it("handles deleted file (null new contents)", () => {
		const result = expandFileDiff(
			"deleted.ts",
			deletedFilePatch,
			"line 1\nline 2\nline 3\n",
			null,
		);
		expect(result).not.toBeNull();
	});

	it("returns empty diff on invalid patch (processFile still succeeds)", () => {
		// processFile always succeeds when file contents are provided,
		// even if the patch string is garbage — it diffs the files directly.
		const result = expandFileDiff(
			"test.ts",
			"this is not a valid patch at all",
			null,
			null,
		);
		expect(result).not.toBeNull();
		expect(result!.hunks).toHaveLength(0);
		expect(result!.isPartial).toBe(false);
	});

	it("preserves file name", () => {
		const result = expandFileDiff(
			"test.ts",
			simplePatch,
			oldContents,
			newContents,
		);
		expect(result).not.toBeNull();
		expect(result!.name).toContain("test.ts");
	});
});

describe("isExpandable", () => {
	it("returns true for small files", () => {
		expect(isExpandable("small content", "small content")).toBe(true);
	});

	it("returns false when old file exceeds limit", () => {
		const huge = "x".repeat(MAX_EXPANDABLE_FILE_SIZE + 1);
		expect(isExpandable(huge, "small")).toBe(false);
	});

	it("returns false when new file exceeds limit", () => {
		const huge = "x".repeat(MAX_EXPANDABLE_FILE_SIZE + 1);
		expect(isExpandable("small", huge)).toBe(false);
	});

	it("returns true when both are null", () => {
		expect(isExpandable(null, null)).toBe(true);
	});
});
