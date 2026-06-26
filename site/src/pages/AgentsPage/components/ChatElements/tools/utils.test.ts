import { describe, expect, it, vi } from "vitest";
import {
	buildEditDiff,
	buildWriteFileDiff,
	COLLAPSED_OUTPUT_HEIGHT,
	COLLAPSED_REPORT_HEIGHT,
	DIFFS_FONT_STYLE,
	diffViewerCSS,
	fileViewerCSS,
	formatModelIntentLabel,
	formatResultOutput,
	formatShellDurationMs,
	formatToolInput,
	getDiffViewerOptions,
	getFileContentForViewer,
	getFileViewerOptions,
	getFileViewerOptionsMinimal,
	getFileViewerOptionsNoHeader,
	getWriteFileDiff,
	humanizeMCPToolName,
	isSubagentRunningStatus,
	isSubagentSuccessStatus,
	mapSubagentStatusToToolStatus,
	normalizeStatus,
	parseArgs,
	parseEditFilesArgs,
	parseServerEditDiffText,
	parseServerEditResults,
	sanitizeExecuteModelIntent,
	stripSvnIndexHeaders,
	summarizeParsedCommands,
	toProviderLabel,
} from "./utils";

describe("formatModelIntentLabel", () => {
	it("returns empty string for empty values", () => {
		expect(formatModelIntentLabel(undefined)).toBe("");
		expect(formatModelIntentLabel("")).toBe("");
		expect(formatModelIntentLabel("   ")).toBe("");
	});

	it("trims and capitalizes labels", () => {
		expect(formatModelIntentLabel("checking repository state")).toBe(
			"Checking repository state",
		);
		expect(formatModelIntentLabel(" a")).toBe("A");
		expect(formatModelIntentLabel("Running tests")).toBe("Running tests");
	});
});

describe("sanitizeExecuteModelIntent", () => {
	it("strips redundant command and duration suffixes", () => {
		expect(
			sanitizeExecuteModelIntent("Running tests using npm for 5s", "npm test"),
		).toBe("Running tests");
		expect(
			sanitizeExecuteModelIntent(
				"checking status using git fetch origin",
				"git fetch origin",
			),
		).toBe("Checking status");
	});

	it("strips trailing durations without command suffixes", () => {
		expect(
			sanitizeExecuteModelIntent("Running tests for 2.5s", "npm test"),
		).toBe("Running tests");
		expect(sanitizeExecuteModelIntent("for 5s", "npm test")).toBe("");
	});

	it("strips leading using only when it references the command", () => {
		expect(
			sanitizeExecuteModelIntent("using git fetch origin", "git fetch origin"),
		).toBe("");
		expect(
			sanitizeExecuteModelIntent("Using environment variables", "npm test"),
		).toBe("Using environment variables");
	});

	it("preserves using when it is not followed by a command reference", () => {
		expect(
			sanitizeExecuteModelIntent("Testing using mock data", "npm test"),
		).toBe("Testing using mock data");
	});
});

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

describe("formatShellDurationMs", () => {
	it("returns empty string for invalid values", () => {
		expect(formatShellDurationMs(undefined)).toBe("");
		expect(formatShellDurationMs(-1)).toBe("");
		expect(formatShellDurationMs(Number.NaN)).toBe("");
		expect(formatShellDurationMs(Number.POSITIVE_INFINITY)).toBe("");
	});

	it("formats milliseconds and rounded seconds", () => {
		expect(formatShellDurationMs(100)).toBe("100ms");
		expect(formatShellDurationMs(47_200)).toBe("47.2s");
		expect(formatShellDurationMs(59_949)).toBe("59.9s");
		expect(formatShellDurationMs(59_950)).toBe("1m");
	});

	it("formats rounded minutes and hours", () => {
		expect(formatShellDurationMs(3_596_999)).toBe("59.9m");
		expect(formatShellDurationMs(3_597_000)).toBe("1h");
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

	it("treats interrupted as an unknown status, not a chat status", () => {
		// The interrupt_agent rename returns an `interrupted: true` response
		// boolean, which is not a chat status. Status mapping only handles
		// chat status strings, so "interrupted" falls back like any unknown
		// value and "terminated" keeps mapping to completed.
		expect(mapSubagentStatusToToolStatus("interrupted", "running")).toBe(
			"running",
		);
		expect(mapSubagentStatusToToolStatus("interrupted", "error")).toBe("error");
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

describe("formatToolInput", () => {
	it("returns null for null, undefined, and empty inputs", () => {
		expect(formatToolInput(null)).toBeNull();
		expect(formatToolInput(undefined)).toBeNull();
		expect(formatToolInput("")).toBeNull();
		expect(formatToolInput({})).toBeNull();
		expect(formatToolInput([])).toBeNull();
		expect(formatToolInput("{}")).toBeNull();
		expect(formatToolInput("[]")).toBeNull();
		expect(formatToolInput("null")).toBeNull();
		expect(
			formatToolInput(
				JSON.stringify({
					model_intent: "Reading backend issues",
					properties: {},
				}),
			),
		).toBeNull();
	});

	it("formats object input as pretty JSON", () => {
		expect(formatToolInput({ project: "backend", limit: 2 })).toBe(
			JSON.stringify({ project: "backend", limit: 2 }, null, 2),
		);
	});

	it("formats JSON string input as pretty JSON", () => {
		expect(formatToolInput('{"project":"backend","limit":2}')).toBe(
			JSON.stringify({ project: "backend", limit: 2 }, null, 2),
		);
	});

	it("unwraps model intent input wrappers", () => {
		expect(
			formatToolInput({
				model_intent: "Reading backend issues",
				properties: { project: "backend" },
			}),
		).toBe(JSON.stringify({ project: "backend" }, null, 2));
		expect(
			formatToolInput({
				model_intent: "Reading backend issues",
				project: "backend",
			}),
		).toBe(JSON.stringify({ project: "backend" }, null, 2));
		expect(
			formatToolInput(
				JSON.stringify({
					model_intent: "Reading backend issues",
					properties: { project: "backend" },
				}),
			),
		).toBe(JSON.stringify({ project: "backend" }, null, 2));
	});

	it("preserves non-JSON string input", () => {
		expect(formatToolInput("search text")).toBe("search text");
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
		expect(opts.overflow).toBe("wrap");
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

	it("does not emit console errors from SVN-style Index headers", () => {
		const spy = vi.spyOn(console, "error").mockImplementation(() => {});
		try {
			const diff = buildWriteFileDiff(
				"src/components/Example.tsx",
				"export default function Example() {\n  return <div />;\n}\n",
			);
			expect(diff).not.toBeNull();
			expect(spy).not.toHaveBeenCalled();
		} finally {
			spy.mockRestore();
		}
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
				undefined, // invalid: undefined
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

	it("filters out edits with non-string search or replace", () => {
		const args = {
			files: [
				{
					path: "a.ts",
					edits: [
						{ search: "x", replace: "y" },
						{ search: "a" }, // missing replace
						{ replace: "b" }, // missing search
						{ search: 42, replace: "c" }, // non-string search
						{ search: "d", replace: null }, // non-string replace
						null, // null edit
					],
				},
			],
		};
		const result = parseEditFilesArgs(args);
		expect(result).toHaveLength(1);
		expect(result[0].edits).toHaveLength(1);
		expect(result[0].edits[0]).toEqual({ search: "x", replace: "y" });
	});

	// Yup.object() is optional by default, so undefined passes
	// isValidSync in strict mode. Without .required() on the
	// schemas, undefined entries survive the filter and crash
	// the subsequent .map() accessing f.path.
	it("rejects undefined file entries and edits", () => {
		const args = {
			files: [
				undefined,
				{
					path: "a.ts",
					edits: [undefined, { search: "x", replace: "y" }],
				},
			],
		};
		const result = parseEditFilesArgs(args);
		expect(result).toHaveLength(1);
		expect(result[0].path).toBe("a.ts");
		expect(result[0].edits).toHaveLength(1);
		expect(result[0].edits[0]).toEqual({ search: "x", replace: "y" });
	});

	// Regression: a partial edit with a missing replace field caused
	// Diff.createPatch to crash inside its tokenize method with
	// "Cannot read properties of undefined (reading 'split')".
	// This reproduces the exact call path from Tool.tsx:
	// parseEditFilesArgs(args) -> buildEditDiff(file.path, file.edits).
	it("does not crash buildEditDiff when edits have missing replace", () => {
		const args = {
			files: [
				{
					path: "src/app.ts",
					edits: [
						{ search: "const x = 1;", replace: "const x = 2;" },
						{ search: "const y = 3;" }, // streamed edit, replace not yet present
					],
				},
			],
		};
		const parsed = parseEditFilesArgs(args);
		expect(parsed).toHaveLength(1);
		// The incomplete edit should be filtered out, leaving only
		// the valid one so buildEditDiff never sees undefined.
		expect(parsed[0].edits).toHaveLength(1);
		const diff = buildEditDiff(parsed[0].path, parsed[0].edits);
		expect(diff).not.toBeNull();
	});

	// search uses required() (rejects "") while replace uses
	// defined() (allows ""). This asymmetry is intentional:
	// empty search is meaningless, empty replace is a deletion.
	it("rejects edits with empty-string search", () => {
		const args = {
			files: [
				{
					path: "a.ts",
					edits: [
						{ search: "", replace: "new" },
						{ search: "valid", replace: "also valid" },
					],
				},
			],
		};
		const result = parseEditFilesArgs(args);
		expect(result).toHaveLength(1);
		expect(result[0].edits).toHaveLength(1);
		expect(result[0].edits[0].search).toBe("valid");
	});

	it("preserves edits with empty-string replace (deletion)", () => {
		const args = {
			files: [
				{
					path: "src/app.ts",
					edits: [{ search: "const old = 1;", replace: "" }],
				},
			],
		};
		const parsed = parseEditFilesArgs(args);
		expect(parsed).toHaveLength(1);
		expect(parsed[0].edits).toHaveLength(1);
		expect(parsed[0].edits[0].replace).toBe("");
	});

	it("accepts old_text/new_text field names", () => {
		const args = {
			files: [
				{
					path: "a.ts",
					edits: [{ old_text: "before", new_text: "after" }],
				},
			],
		};
		const result = parseEditFilesArgs(args);
		expect(result).toHaveLength(1);
		expect(result[0].edits).toHaveLength(1);
		expect(result[0].edits[0]).toEqual({ search: "before", replace: "after" });
	});

	it("prefers old_text/new_text over search/replace when both present", () => {
		const args = {
			files: [
				{
					path: "a.ts",
					edits: [
						{
							old_text: "from-old-text",
							new_text: "from-new-text",
							search: "from-search",
							replace: "from-replace",
						},
					],
				},
			],
		};
		const result = parseEditFilesArgs(args);
		expect(result[0].edits[0]).toEqual({
			search: "from-old-text",
			replace: "from-new-text",
		});
	});

	it("preserves deletion via old_text/new_text (empty new_text)", () => {
		const args = {
			files: [
				{
					path: "a.ts",
					edits: [{ old_text: "remove me", new_text: "" }],
				},
			],
		};
		const result = parseEditFilesArgs(args);
		expect(result[0].edits[0]).toEqual({ search: "remove me", replace: "" });
	});

	// During streaming the model may emit a file entry before any
	// edit is complete. Every edit has a missing replace, so all are
	// filtered out. The file entry survives with an empty edits
	// array and buildEditDiff returns null.
	it("returns file entry with empty edits when all edits are invalid", () => {
		const args = {
			files: [
				{
					path: "src/app.ts",
					edits: [{ search: "const x = 1;" }, { search: "const y = 2;" }],
				},
			],
		};
		const parsed = parseEditFilesArgs(args);
		expect(parsed).toHaveLength(1);
		expect(parsed[0].path).toBe("src/app.ts");
		expect(parsed[0].edits).toHaveLength(0);
		expect(buildEditDiff(parsed[0].path, parsed[0].edits)).toBeNull();
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

	it("does not emit console errors for multi-edit diffs", () => {
		const spy = vi.spyOn(console, "error").mockImplementation(() => {});
		try {
			const diff = buildEditDiff(
				"/home/coder/coder/site/src/pages/AgentsPage/components/ChatSidebar/ChatsSidebar.tsx",
				[
					{ search: "const a = 1;", replace: "const a = 2;" },
					{ search: "const b = 3;", replace: "const b = 4;" },
				],
			);
			expect(diff).not.toBeNull();
			// Before the fix, @pierre/diffs logged:
			//   parseLineType: Invalid firstChar: "I"
			//   processFile: invalid rawLine: Index: ...
			expect(spy).not.toHaveBeenCalled();
		} finally {
			spy.mockRestore();
		}
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

	it("preserves unchanged lines as context in hunks", () => {
		const diff = buildEditDiff("file.ts", [
			{
				search: "const x = 1;\nconst y = 2;\nconst z = 3;",
				replace: "const x = 10;\nconst y = 2;\nconst z = 30;",
			},
		]);
		expect(diff).not.toBeNull();
		const hunk = diff!.hunks[0];
		// The hunk should contain context blocks for the unchanged
		// middle line rather than removing and re-adding everything.
		const hasContext = hunk.hunkContent.some((c) => c.type === "context");
		expect(hasContext).toBe(true);
	});
});

describe("stripSvnIndexHeaders", () => {
	it("removes Index: headers from SVN-style patches", () => {
		const input = [
			"Index: src/file.ts",
			"===================================================================",
			"--- src/file.ts",
			"+++ src/file.ts",
			"@@ -1,1 +1,1 @@",
			"-old",
			"+new",
			"",
		].join("\n");
		const result = stripSvnIndexHeaders(input);
		expect(result).not.toContain("Index:");
		expect(result).not.toContain("===");
		expect(result).toContain("--- src/file.ts");
	});

	it("handles multiple Index: headers in concatenated patches", () => {
		const input = [
			"Index: a.ts",
			"===================================================================",
			"--- a.ts",
			"+++ a.ts",
			"@@ -1,1 +1,1 @@",
			"-old1",
			"+new1",
			"Index: b.ts",
			"===================================================================",
			"--- b.ts",
			"+++ b.ts",
			"@@ -1,1 +1,1 @@",
			"-old2",
			"+new2",
			"",
		].join("\n");
		const result = stripSvnIndexHeaders(input);
		// Both Index headers removed.
		expect(result.match(/Index:/g)).toBeNull();
		// Diff content preserved.
		expect(result).toContain("-old1");
		expect(result).toContain("+new2");
	});

	it("is a no-op for git-style diffs", () => {
		const gitDiff = [
			"diff --git a/file.ts b/file.ts",
			"--- a/file.ts",
			"+++ b/file.ts",
			"@@ -1,1 +1,1 @@",
			"-old",
			"+new",
			"",
		].join("\n");
		expect(stripSvnIndexHeaders(gitDiff)).toBe(gitDiff);
	});

	it("is a no-op for empty strings", () => {
		expect(stripSvnIndexHeaders("")).toBe("");
	});
});

describe("humanizeMCPToolName", () => {
	it("strips slug prefix and humanizes", () => {
		expect(humanizeMCPToolName("linear", "linear__list_issues")).toBe(
			"List issues",
		);
	});

	it("handles single-word tool name after prefix", () => {
		expect(humanizeMCPToolName("github", "github__search")).toBe("Search");
	});

	it("humanizes entire name when prefix does not match", () => {
		expect(humanizeMCPToolName("linear", "github__list_repos")).toBe(
			"Github list repos",
		);
	});

	it("falls back to prefixedName when stripping prefix leaves empty string", () => {
		expect(humanizeMCPToolName("linear", "linear__")).toBe("linear__");
	});

	it("collapses consecutive underscores into a single space", () => {
		expect(humanizeMCPToolName("srv", "srv__get__data")).toBe("Get data");
	});

	it("humanizes tool name without slug prefix", () => {
		expect(humanizeMCPToolName("linear", "list_issues")).toBe("List issues");
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

	it("DIFFS_FONT_STYLE uses theme-aware diff variables", () => {
		expect(DIFFS_FONT_STYLE).toHaveProperty(
			"--diffs-addition-color-override",
			"hsl(var(--git-added))",
		);
		expect(DIFFS_FONT_STYLE).toHaveProperty(
			"--diffs-deletion-color-override",
			"hsl(var(--git-deleted))",
		);
		expect(DIFFS_FONT_STYLE).toHaveProperty(
			"--diffs-bg-addition-override",
			"hsl(var(--surface-git-added))",
		);
		expect(DIFFS_FONT_STYLE).toHaveProperty(
			"--diffs-bg-deletion-override",
			"hsl(var(--surface-git-deleted))",
		);
	});

	it("fileViewerCSS keeps file viewer backgrounds transparent", () => {
		expect(fileViewerCSS).toContain("background-color: transparent");
		expect(fileViewerCSS).toContain("[data-diffs-header]");
		expect(fileViewerCSS).not.toContain("[data-code]");
	});

	it("diffViewerCSS keeps hunk separator styling scoped", () => {
		expect(diffViewerCSS).toContain("[data-separator='line-info']");
		expect(diffViewerCSS).toContain("[data-separator-content]");
		expect(diffViewerCSS).not.toContain("[data-diffs-header]");
	});
});

describe("parseServerEditResults", () => {
	it("returns null when result is not a record", () => {
		expect(parseServerEditResults(null)).toBeNull();
		expect(parseServerEditResults(undefined)).toBeNull();
		expect(parseServerEditResults("foo")).toBeNull();
	});

	it("returns null when files field is absent", () => {
		expect(parseServerEditResults({ ok: true })).toBeNull();
	});

	it("returns null when files is explicitly null (older agents)", () => {
		expect(parseServerEditResults({ files: null })).toBeNull();
	});

	it("returns an empty array when files is an empty array", () => {
		expect(parseServerEditResults({ files: [] })).toEqual([]);
	});

	it("parses well-formed entries", () => {
		const result = parseServerEditResults({
			files: [
				{
					path: "/abs/a.txt",
					diff: "--- /abs/a.txt\n+++ /abs/a.txt\n@@ -1 +1 @@\n-a\n+A\n",
				},
				{ path: "/abs/b.txt", diff: "" },
			],
		});
		expect(result).toEqual([
			{
				path: "/abs/a.txt",
				diff: "--- /abs/a.txt\n+++ /abs/a.txt\n@@ -1 +1 @@\n-a\n+A\n",
			},
			{ path: "/abs/b.txt", diff: "" },
		]);
	});

	it("skips malformed entries without dropping the rest", () => {
		const result = parseServerEditResults({
			files: [
				null,
				{ diff: "orphan" },
				{ path: "", diff: "empty-path" },
				{ path: "/ok", diff: "--- /ok\n+++ /ok\n" },
			],
		});
		expect(result).toEqual([{ path: "/ok", diff: "--- /ok\n+++ /ok\n" }]);
	});
});

describe("parseServerEditDiffText", () => {
	const changedLineContents = (
		diff: NonNullable<ReturnType<typeof parseServerEditDiffText>>,
	) =>
		diff.hunks.flatMap((hunk) =>
			hunk.hunkContent.flatMap((content) => {
				if (content.type !== "change") {
					return [];
				}
				return [
					...diff.deletionLines.slice(
						content.deletionLineIndex,
						content.deletionLineIndex + content.deletions,
					),
					...diff.additionLines.slice(
						content.additionLineIndex,
						content.additionLineIndex + content.additions,
					),
				].map((line) => line.trimEnd());
			}),
		);

	it("returns null for an empty string (no-op edit)", () => {
		expect(parseServerEditDiffText("")).toBeNull();
	});

	it("parses a unified diff into a FileDiffMetadata", () => {
		const diff = parseServerEditDiffText(
			"--- /abs/a.txt\n+++ /abs/a.txt\n@@ -1 +1 @@\n-hello\n+HELLO\n",
		);
		expect(diff).not.toBeNull();
		expect(diff?.name).toBe("/abs/a.txt");
	});

	it("parses quoted git diff headers", () => {
		const diff = parseServerEditDiffText(
			[
				'diff --git "a/path with spaces.ts" "b/path with spaces.ts"',
				"index 1111111..2222222 100644",
				'--- "a/path with spaces.ts"',
				'+++ "b/path with spaces.ts"',
				"@@ -1 +1 @@",
				"-old value",
				"+new value",
				"",
			].join("\n"),
		);

		expect(diff).not.toBeNull();
		expect(diff?.name).toBe("path with spaces.ts");
		expect(changedLineContents(diff!)).toEqual(["old value", "new value"]);
	});

	it("parses diffs that include git patch footer metadata", () => {
		const diff = parseServerEditDiffText(
			[
				"diff --git a/example.ts b/example.ts",
				"index 1111111..2222222 100644",
				"--- a/example.ts",
				"+++ b/example.ts",
				"@@ -1 +1 @@",
				"-old value",
				"+new value",
				"-- ",
				"2.45.0",
				"",
			].join("\n"),
		);

		expect(diff).not.toBeNull();
		expect(diff?.name).toBe("example.ts");
		expect(changedLineContents(diff!)).toEqual(["old value", "new value"]);
	});
});

describe("summarizeParsedCommands", () => {
	it("renders <prog> <verb> for multi-verb tools", () => {
		expect(
			summarizeParsedCommands([
				["git", "pull"],
				["git", "add"],
				["git", "commit"],
			]),
		).toBe("git pull, git add, git commit");
	});

	it("renders just <prog> for non-multi-verb tools", () => {
		expect(
			summarizeParsedCommands([
				["cd", "/repo"],
				["ls", "/tmp"],
			]),
		).toBe("cd, ls");
	});

	it("renders single-arg entries as just the program", () => {
		expect(summarizeParsedCommands([["pwd"]])).toBe("pwd");
	});

	it("dedupes consecutive duplicates", () => {
		expect(
			summarizeParsedCommands([
				["git", "pull"],
				["git", "pull"],
			]),
		).toBe("git pull");
	});

	it("keeps non-consecutive duplicates", () => {
		expect(
			summarizeParsedCommands([["git", "pull"], ["ls"], ["git", "pull"]]),
		).toBe("git pull, ls, git pull");
	});

	it("returns empty string for empty input", () => {
		expect(summarizeParsedCommands([])).toBe("");
	});

	it("skips entries with no program", () => {
		expect(summarizeParsedCommands([[""], ["git", "pull"]])).toBe("git pull");
	});
});
