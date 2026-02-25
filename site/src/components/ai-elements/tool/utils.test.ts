import { describe, expect, it } from "vitest";
import {
	BORDER_BG_STYLE,
	buildEditDiff,
	buildWriteFileDiff,
	COLLAPSED_OUTPUT_HEIGHT,
	COLLAPSED_REPORT_HEIGHT,
	DIFFS_FONT_STYLE,
	diffViewerCSS,
	fileViewerCSS,
	formatResultOutput,
	getDiffViewerOptions,
	getFileContentForViewer,
	getFileViewerOptions,
	getFileViewerOptionsMinimal,
	getFileViewerOptionsNoHeader,
	getWriteFileDiff,
	isSubagentRunningStatus,
	isSubagentSuccessStatus,
	mapSubagentStatusToToolStatus,
	normalizeStatus,
	parseArgs,
	parseEditFilesArgs,
	shortDurationMs,
	toProviderLabel,
} from "./utils";

describe("toProviderLabel", () => {
	it("returns displayName when provided", () => {
		expect(toProviderLabel("GitHub", "gh-id", "oauth")).toBe("GitHub");
	});

	it("falls back to providerID when displayName is empty", () => {
		expect(toProviderLabel("", "gh-id", "oauth")).toBe("gh-id");
	});

	it("falls back to providerType when displayName and ID are empty", () => {
		expect(toProviderLabel("", "", "oauth")).toBe("oauth");
	});

	it("returns default label when all are empty", () => {
		expect(toProviderLabel("", "", "")).toBe("Git provider");
	});
});

describe("shortDurationMs", () => {
	it("returns empty string for undefined", () => {
		expect(shortDurationMs(undefined)).toBe("");
	});

	it("returns empty string for negative values", () => {
		expect(shortDurationMs(-1)).toBe("");
		expect(shortDurationMs(-1000)).toBe("");
	});

	it("returns 0s for zero milliseconds", () => {
		expect(shortDurationMs(0)).toBe("0s");
	});

	it("formats sub-second durations", () => {
		expect(shortDurationMs(500)).toBe("1s");
		expect(shortDurationMs(100)).toBe("0s");
	});

	it("formats seconds", () => {
		expect(shortDurationMs(1000)).toBe("1s");
		expect(shortDurationMs(30_000)).toBe("30s");
		expect(shortDurationMs(59_000)).toBe("59s");
	});

	it("formats minutes", () => {
		expect(shortDurationMs(60_000)).toBe("1m");
		expect(shortDurationMs(300_000)).toBe("5m");
		expect(shortDurationMs(3_540_000)).toBe("59m");
	});

	it("formats hours", () => {
		expect(shortDurationMs(3_600_000)).toBe("1h");
		expect(shortDurationMs(7_200_000)).toBe("2h");
	});
});

describe("normalizeStatus", () => {
	it("lowercases and trims", () => {
		expect(normalizeStatus("  COMPLETED  ")).toBe("completed");
	});

	it("handles already-normalized input", () => {
		expect(normalizeStatus("running")).toBe("running");
	});

	it("handles empty string", () => {
		expect(normalizeStatus("")).toBe("");
	});
});

describe("isSubagentSuccessStatus", () => {
	it("returns true for completed", () => {
		expect(isSubagentSuccessStatus("completed")).toBe(true);
	});

	it("returns true for reported", () => {
		expect(isSubagentSuccessStatus("reported")).toBe(true);
	});

	it("is case-insensitive", () => {
		expect(isSubagentSuccessStatus("COMPLETED")).toBe(true);
		expect(isSubagentSuccessStatus(" Reported ")).toBe(true);
	});

	it("returns false for other statuses", () => {
		expect(isSubagentSuccessStatus("running")).toBe(false);
		expect(isSubagentSuccessStatus("error")).toBe(false);
		expect(isSubagentSuccessStatus("")).toBe(false);
	});
});

describe("isSubagentRunningStatus", () => {
	it("returns true for running statuses", () => {
		expect(isSubagentRunningStatus("pending")).toBe(true);
		expect(isSubagentRunningStatus("running")).toBe(true);
		expect(isSubagentRunningStatus("awaiting")).toBe(true);
	});

	it("is case-insensitive", () => {
		expect(isSubagentRunningStatus("RUNNING")).toBe(true);
		expect(isSubagentRunningStatus(" Pending ")).toBe(true);
	});

	it("returns false for non-running statuses", () => {
		expect(isSubagentRunningStatus("completed")).toBe(false);
		expect(isSubagentRunningStatus("error")).toBe(false);
		expect(isSubagentRunningStatus("")).toBe(false);
	});
});

describe("mapSubagentStatusToToolStatus", () => {
	it("returns fallback for empty status", () => {
		expect(mapSubagentStatusToToolStatus("", "running")).toBe("running");
		expect(mapSubagentStatusToToolStatus("  ", "error")).toBe("error");
	});

	it("maps success statuses to completed", () => {
		expect(mapSubagentStatusToToolStatus("completed", "running")).toBe(
			"completed",
		);
		expect(mapSubagentStatusToToolStatus("reported", "running")).toBe(
			"completed",
		);
	});

	it("maps running statuses to running when fallback is not completed", () => {
		expect(mapSubagentStatusToToolStatus("pending", "running")).toBe("running");
		expect(mapSubagentStatusToToolStatus("running", "error")).toBe("running");
		expect(mapSubagentStatusToToolStatus("awaiting", "running")).toBe(
			"running",
		);
	});

	it("preserves completed fallback even with running subagent status", () => {
		expect(mapSubagentStatusToToolStatus("pending", "completed")).toBe(
			"completed",
		);
		expect(mapSubagentStatusToToolStatus("running", "completed")).toBe(
			"completed",
		);
	});

	it("maps waiting to completed", () => {
		expect(mapSubagentStatusToToolStatus("waiting", "running")).toBe(
			"completed",
		);
	});

	it("maps terminated to completed", () => {
		expect(mapSubagentStatusToToolStatus("terminated", "running")).toBe(
			"completed",
		);
	});

	it("maps error to error", () => {
		expect(mapSubagentStatusToToolStatus("error", "running")).toBe("error");
	});

	it("returns fallback for unknown statuses", () => {
		expect(mapSubagentStatusToToolStatus("unknown-status", "running")).toBe(
			"running",
		);
		expect(mapSubagentStatusToToolStatus("banana", "error")).toBe("error");
	});
});

describe("parseArgs", () => {
	it("returns null for falsy values", () => {
		expect(parseArgs(null)).toBeNull();
		expect(parseArgs(undefined)).toBeNull();
		expect(parseArgs("")).toBeNull();
		expect(parseArgs(0)).toBeNull();
	});

	it("parses a JSON string into a record", () => {
		expect(parseArgs('{"key": "value"}')).toEqual({ key: "value" });
	});

	it("returns null for invalid JSON strings", () => {
		expect(parseArgs("not json")).toBeNull();
	});

	it("returns null for JSON strings that parse to non-objects", () => {
		expect(parseArgs('"just a string"')).toBeNull();
		expect(parseArgs("42")).toBeNull();
		expect(parseArgs("[1, 2, 3]")).toBeNull();
	});

	it("returns object args directly", () => {
		const obj = { path: "/foo.ts", content: "hello" };
		expect(parseArgs(obj)).toEqual(obj);
	});

	it("returns null for arrays", () => {
		expect(parseArgs([1, 2, 3])).toBeNull();
	});
});

describe("formatResultOutput", () => {
	it("returns null for null and undefined", () => {
		expect(formatResultOutput(null)).toBeNull();
		expect(formatResultOutput(undefined)).toBeNull();
	});

	it("returns trimmed string or null for empty", () => {
		expect(formatResultOutput("  hello  ")).toBe("hello");
		expect(formatResultOutput("")).toBeNull();
		expect(formatResultOutput("   ")).toBeNull();
	});

	it("extracts output field from record", () => {
		expect(formatResultOutput({ output: "  some output  " })).toBe(
			"some output",
		);
	});

	it("extracts content field from record when output is empty", () => {
		expect(formatResultOutput({ content: "file content" })).toBe(
			"file content",
		);
	});

	it("prefers output over content", () => {
		expect(
			formatResultOutput({ output: "cmd output", content: "file content" }),
		).toBe("cmd output");
	});

	it("falls back to JSON.stringify when output and content are empty", () => {
		// Both output and content are empty strings after trim, so
		// the function falls through to JSON.stringify the record.
		const result = formatResultOutput({ output: "", content: "" });
		expect(result).toBe(JSON.stringify({ output: "", content: "" }, null, 2));
	});

	it("falls back to JSON.stringify for objects without output/content", () => {
		const result = formatResultOutput({ status: "ok", code: 0 });
		expect(result).toBe(JSON.stringify({ status: "ok", code: 0 }, null, 2));
	});

	it("returns String representation for non-object/non-string primitives", () => {
		expect(formatResultOutput(42)).toBe("42");
		expect(formatResultOutput(true)).toBe("true");
	});
});

describe("getDiffViewerOptions", () => {
	it("returns dark theme options", () => {
		const opts = getDiffViewerOptions(true);
		expect(opts.themeType).toBe("dark");
		expect(opts.theme).toBe("github-dark-high-contrast");
		expect(opts.diffStyle).toBe("unified");
		expect(opts.diffIndicators).toBe("bars");
		expect(opts.overflow).toBe("scroll");
		expect(opts.unsafeCSS).toBe(diffViewerCSS);
	});

	it("returns light theme options", () => {
		const opts = getDiffViewerOptions(false);
		expect(opts.themeType).toBe("light");
		expect(opts.theme).toBe("github-light");
	});
});

describe("getFileViewerOptions", () => {
	it("returns dark theme options", () => {
		const opts = getFileViewerOptions(true);
		expect(opts.themeType).toBe("dark");
		expect(opts.theme).toBe("github-dark-high-contrast");
		expect(opts.overflow).toBe("scroll");
		expect(opts.unsafeCSS).toBe(fileViewerCSS);
	});

	it("returns light theme options", () => {
		const opts = getFileViewerOptions(false);
		expect(opts.themeType).toBe("light");
		expect(opts.theme).toBe("github-light");
	});
});

describe("getFileViewerOptionsNoHeader", () => {
	it("extends base options with disableFileHeader", () => {
		const opts = getFileViewerOptionsNoHeader(true);
		expect(opts.disableFileHeader).toBe(true);
		expect(opts.themeType).toBe("dark");
	});
});

describe("getFileViewerOptionsMinimal", () => {
	it("extends base options with disableFileHeader and disableLineNumbers", () => {
		const opts = getFileViewerOptionsMinimal(false);
		expect(opts.disableFileHeader).toBe(true);
		expect(opts.disableLineNumbers).toBe(true);
		expect(opts.themeType).toBe("light");
	});
});

describe("getFileContentForViewer", () => {
	it("returns null for unsupported tool names", () => {
		expect(getFileContentForViewer("write_file", {}, {})).toBeNull();
		expect(getFileContentForViewer("search", {}, {})).toBeNull();
	});

	describe("execute tool", () => {
		it("returns output with shell path and disabled header/line numbers", () => {
			const result = getFileContentForViewer(
				"execute",
				{},
				{ output: "ls -la" },
			);
			expect(result).toEqual({
				path: "output.sh",
				content: "ls -la",
				disableHeader: true,
				disableLineNumbers: true,
			});
		});

		it("returns null when result is not a record", () => {
			expect(getFileContentForViewer("execute", {}, "string")).toBeNull();
		});

		it("returns null when output is empty", () => {
			expect(
				getFileContentForViewer("execute", {}, { output: "  " }),
			).toBeNull();
		});
	});

	describe("read_file tool", () => {
		it("returns path and content from args and result", () => {
			const args = { path: "/src/main.ts" };
			const result = { content: "const x = 1;" };
			const out = getFileContentForViewer("read_file", args, result);
			expect(out).toEqual({
				path: "/src/main.ts",
				content: "const x = 1;",
			});
		});

		it("parses JSON string args", () => {
			const args = JSON.stringify({ path: "/foo.ts" });
			const result = { content: "hello" };
			expect(getFileContentForViewer("read_file", args, result)).toEqual({
				path: "/foo.ts",
				content: "hello",
			});
		});

		it("returns null when path is missing", () => {
			expect(
				getFileContentForViewer("read_file", {}, { content: "hello" }),
			).toBeNull();
		});

		it("returns null when content is empty", () => {
			expect(
				getFileContentForViewer("read_file", { path: "/x" }, { content: "" }),
			).toBeNull();
		});

		it("returns null when result is not a record", () => {
			expect(
				getFileContentForViewer("read_file", { path: "/x" }, "not record"),
			).toBeNull();
		});
	});
});

describe("buildWriteFileDiff", () => {
	it("returns a FileDiffMetadata for new file content", () => {
		const diff = buildWriteFileDiff(
			"src/hello.ts",
			"const x = 1;\nconst y = 2;\n",
		);
		expect(diff).not.toBeNull();
		expect(diff!.name).toContain("hello.ts");
	});

	it("returns null for empty content", () => {
		expect(buildWriteFileDiff("foo.ts", "")).toBeNull();
	});

	it("handles content without trailing newline", () => {
		const diff = buildWriteFileDiff("foo.ts", "single line");
		expect(diff).not.toBeNull();
	});

	it("handles content that is only a newline", () => {
		// A single newline splits into ["", ""], trailing empty is popped,
		// leaving [""] which is one line.
		const diff = buildWriteFileDiff("foo.ts", "\n");
		// After split: ["", ""], pop trailing empty -> [""]
		// That's 1 empty-string line, which is still a valid line.
		expect(diff).not.toBeNull();
	});
});

describe("getWriteFileDiff", () => {
	it("returns null for non-write_file tools", () => {
		expect(
			getWriteFileDiff("read_file", { path: "x", content: "y" }),
		).toBeNull();
		expect(getWriteFileDiff("execute", { path: "x", content: "y" })).toBeNull();
	});

	it("returns null when args cannot be parsed", () => {
		expect(getWriteFileDiff("write_file", null)).toBeNull();
	});

	it("returns null when path is missing", () => {
		expect(getWriteFileDiff("write_file", { content: "hello" })).toBeNull();
	});

	it("returns null when content is missing", () => {
		expect(getWriteFileDiff("write_file", { path: "/foo.ts" })).toBeNull();
	});

	it("returns a diff for valid write_file args", () => {
		const diff = getWriteFileDiff("write_file", {
			path: "src/main.ts",
			content: "export default 42;\n",
		});
		expect(diff).not.toBeNull();
		expect(diff!.name).toContain("main.ts");
	});

	it("parses JSON string args", () => {
		const diff = getWriteFileDiff(
			"write_file",
			JSON.stringify({ path: "x.ts", content: "y" }),
		);
		expect(diff).not.toBeNull();
	});
});

describe("parseEditFilesArgs", () => {
	it("returns empty array for null args", () => {
		expect(parseEditFilesArgs(null)).toEqual([]);
	});

	it("returns empty array when files is not an array", () => {
		expect(parseEditFilesArgs({ files: "not array" })).toEqual([]);
	});

	it("returns empty array when files key is missing", () => {
		expect(parseEditFilesArgs({ other: "value" })).toEqual([]);
	});

	it("filters out invalid entries", () => {
		const args = {
			files: [
				{ path: "a.ts", edits: [{ search: "x", replace: "y" }] },
				{ path: 42, edits: [] }, // invalid: path not string
				null, // invalid: null
				{ path: "b.ts" }, // invalid: no edits array
				{ path: "c.ts", edits: [{ search: "a", replace: "b" }] },
			],
		};
		const result = parseEditFilesArgs(args);
		expect(result).toHaveLength(2);
		expect(result[0].path).toBe("a.ts");
		expect(result[1].path).toBe("c.ts");
	});

	it("parses JSON string args", () => {
		const args = JSON.stringify({
			files: [{ path: "test.ts", edits: [{ search: "old", replace: "new" }] }],
		});
		const result = parseEditFilesArgs(args);
		expect(result).toHaveLength(1);
		expect(result[0].path).toBe("test.ts");
	});
});

describe("buildEditDiff", () => {
	it("returns null for empty edits array", () => {
		expect(buildEditDiff("file.ts", [])).toBeNull();
	});

	it("builds a diff from a single search/replace pair", () => {
		const diff = buildEditDiff("src/index.ts", [
			{ search: "const x = 1;", replace: "const x = 2;" },
		]);
		expect(diff).not.toBeNull();
		expect(diff!.name).toContain("index.ts");
	});

	it("builds a diff from multiple search/replace pairs", () => {
		const diff = buildEditDiff("src/index.ts", [
			{ search: "const x = 1;", replace: "const x = 2;" },
			{ search: "const y = 3;", replace: "const y = 4;" },
		]);
		expect(diff).not.toBeNull();
	});

	it("strips leading slash from path", () => {
		const diff = buildEditDiff("/src/index.ts", [
			{ search: "old", replace: "new" },
		]);
		expect(diff).not.toBeNull();
		// The diff path should not have a double slash.
		expect(diff!.name).not.toContain("//");
	});

	it("skips edits with empty search", () => {
		const diff = buildEditDiff("file.ts", [{ search: "", replace: "new" }]);
		// All edits skipped → only header lines remain. The parser
		// still returns a file entry but with no hunks.
		expect(diff).not.toBeNull();
		expect(diff!.hunks).toHaveLength(0);
	});

	it("handles multi-line search and replace", () => {
		const diff = buildEditDiff("file.ts", [
			{
				search: "line1\nline2\nline3",
				replace: "newLine1\nnewLine2",
			},
		]);
		expect(diff).not.toBeNull();
	});

	it("handles replace with trailing newline (trailing empty popped)", () => {
		const diff = buildEditDiff("file.ts", [
			{ search: "old\n", replace: "new\n" },
		]);
		expect(diff).not.toBeNull();
	});
});

describe("constants", () => {
	it("COLLAPSED_OUTPUT_HEIGHT is 54", () => {
		expect(COLLAPSED_OUTPUT_HEIGHT).toBe(54);
	});

	it("COLLAPSED_REPORT_HEIGHT is 72", () => {
		expect(COLLAPSED_REPORT_HEIGHT).toBe(72);
	});

	it("DIFFS_FONT_STYLE has expected CSS properties", () => {
		expect(DIFFS_FONT_STYLE).toHaveProperty("--diffs-font-size", "11px");
		expect(DIFFS_FONT_STYLE).toHaveProperty("--diffs-line-height", "1.5");
	});

	it("BORDER_BG_STYLE has expected background", () => {
		expect(BORDER_BG_STYLE).toHaveProperty(
			"background",
			"hsl(var(--border-default))",
		);
	});

	it("fileViewerCSS is a non-empty string", () => {
		expect(typeof fileViewerCSS).toBe("string");
		expect(fileViewerCSS.length).toBeGreaterThan(0);
	});

	it("diffViewerCSS includes border-left style", () => {
		expect(diffViewerCSS).toContain("border-left");
	});
});
