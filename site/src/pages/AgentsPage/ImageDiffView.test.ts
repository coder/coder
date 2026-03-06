import { describe, expect, it } from "vitest";
import { parseBinaryImageDiffs } from "./ImageDiffView";

describe("parseBinaryImageDiffs", () => {
	it("returns empty array for text-only diffs", () => {
		const textDiff = [
			"diff --git a/src/main.ts b/src/main.ts",
			"index abc1234..def5678 100644",
			"--- a/src/main.ts",
			"+++ b/src/main.ts",
			"@@ -1,3 +1,4 @@",
			" import { foo } from './foo';",
			"+import { bar } from './bar';",
			" ",
			" console.log(foo);",
		].join("\n");

		expect(parseBinaryImageDiffs(textDiff)).toEqual([]);
	});

	it("detects added image", () => {
		const diff = [
			"diff --git a/images/logo.png b/images/logo.png",
			"new file mode 100644",
			"index 0000000..abcdef1",
			"Binary files /dev/null and b/images/logo.png differ",
		].join("\n");

		expect(parseBinaryImageDiffs(diff)).toEqual([
			{ name: "images/logo.png", changeType: "new", isBinaryImage: true },
		]);
	});

	it("detects deleted image", () => {
		const diff = [
			"diff --git a/old/banner.jpg b/old/banner.jpg",
			"deleted file mode 100644",
			"index abcdef1..0000000",
			"Binary files a/old/banner.jpg and /dev/null differ",
		].join("\n");

		expect(parseBinaryImageDiffs(diff)).toEqual([
			{ name: "old/banner.jpg", changeType: "deleted", isBinaryImage: true },
		]);
	});

	it("detects modified image", () => {
		const diff = [
			"diff --git a/assets/icon.svg b/assets/icon.svg",
			"index abc1234..def5678 100644",
			"Binary files a/assets/icon.svg and b/assets/icon.svg differ",
		].join("\n");

		expect(parseBinaryImageDiffs(diff)).toEqual([
			{ name: "assets/icon.svg", changeType: "change", isBinaryImage: true },
		]);
	});

	it("ignores non-image binary files", () => {
		const diff = [
			"diff --git a/lib/module.wasm b/lib/module.wasm",
			"new file mode 100644",
			"index 0000000..abcdef1",
			"Binary files /dev/null and b/lib/module.wasm differ",
		].join("\n");

		expect(parseBinaryImageDiffs(diff)).toEqual([]);
	});

	it("handles multiple images mixed with text diffs", () => {
		const diff = [
			"diff --git a/README.md b/README.md",
			"index aaa1111..bbb2222 100644",
			"--- a/README.md",
			"+++ b/README.md",
			"@@ -1,2 +1,3 @@",
			" # Project",
			"+Some new docs.",
			" ",
			"diff --git a/assets/screenshot.png b/assets/screenshot.png",
			"new file mode 100644",
			"index 0000000..ccc3333",
			"Binary files /dev/null and b/assets/screenshot.png differ",
			"diff --git a/src/index.ts b/src/index.ts",
			"index ddd4444..eee5555 100644",
			"--- a/src/index.ts",
			"+++ b/src/index.ts",
			"@@ -1 +1,2 @@",
			" export {};",
			"+export { App } from './App';",
		].join("\n");

		expect(parseBinaryImageDiffs(diff)).toEqual([
			{
				name: "assets/screenshot.png",
				changeType: "new",
				isBinaryImage: true,
			},
		]);
	});

	it("handles various image extensions", () => {
		const extensions = [
			"png",
			"jpg",
			"jpeg",
			"gif",
			"webp",
			"svg",
			"avif",
			"bmp",
			"ico",
		];
		const diff = extensions
			.map((ext) =>
				[
					`diff --git a/img/photo.${ext} b/img/photo.${ext}`,
					"new file mode 100644",
					"index 0000000..abcdef1",
					`Binary files /dev/null and b/img/photo.${ext} differ`,
				].join("\n"),
			)
			.join("\n");

		const result = parseBinaryImageDiffs(diff);

		expect(result).toHaveLength(extensions.length);
		for (const ext of extensions) {
			expect(result).toContainEqual({
				name: `img/photo.${ext}`,
				changeType: "new",
				isBinaryImage: true,
			});
		}
	});

	it("returns empty array for empty string input", () => {
		expect(parseBinaryImageDiffs("")).toEqual([]);
	});

	it("matches extensions case-insensitively", () => {
		const diff = [
			"diff --git a/img/photo.PNG b/img/photo.PNG",
			"new file mode 100644",
			"index 0000000..abcdef1",
			"Binary files /dev/null and b/img/photo.PNG differ",
		].join("\n");

		expect(parseBinaryImageDiffs(diff)).toEqual([
			{ name: "img/photo.PNG", changeType: "new", isBinaryImage: true },
		]);
	});
});
