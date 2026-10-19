import type { DiffLineAnnotation } from "@pierre/diffs/react";
import { describe, expect, it } from "vitest";
import { diffItemVersion } from "./DiffViewer";
import { parseDiffString } from "./parseDiff";

const diffWith = (line: string) =>
	parseDiffString(
		[
			"--- a/src/a.ts",
			"+++ b/src/a.ts",
			"@@ -1,1 +1,1 @@",
			"-const v = 0;",
			`+${line}`,
		].join("\n"),
	)[0];

const annotations = (count: number): DiffLineAnnotation<string>[] =>
	Array.from({ length: count }, (_, i) => ({
		side: "additions",
		lineNumber: i + 1,
		metadata: "active-input",
	}));

describe("diffItemVersion", () => {
	it("changes when the file content changes", () => {
		expect(diffItemVersion(diffWith("const v = 1;"), [], false)).not.toBe(
			diffItemVersion(diffWith("const v = 2;"), [], false),
		);
	});

	it("changes when the collapsed flag flips, even with many annotations", () => {
		// A floating-point version loses its low bit after about 10
		// annotations, which would make the toggle a no-op.
		const file = diffWith("const v = 1;");
		for (const count of [0, 1, 12, 40]) {
			expect(diffItemVersion(file, annotations(count), false)).not.toBe(
				diffItemVersion(file, annotations(count), true),
			);
		}
	});

	it("is stable for identical inputs", () => {
		expect(
			diffItemVersion(diffWith("const v = 1;"), annotations(2), true),
		).toBe(diffItemVersion(diffWith("const v = 1;"), annotations(2), true));
	});
});
