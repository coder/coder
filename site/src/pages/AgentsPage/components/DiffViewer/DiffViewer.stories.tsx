import type { DiffLineAnnotation, SelectedLineRange } from "@pierre/diffs";
import { parsePatchFiles } from "@pierre/diffs";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, waitFor } from "storybook/test";
import type { DiffStyle } from "../DiffViewer/DiffViewer";
import { DiffViewer } from "../DiffViewer/DiffViewer";
import { InlinePromptInput } from "../DiffViewer/RemoteDiffPanel";

// biome-ignore format: raw diff string must preserve exact whitespace
const sampleDiff = [
"diff --git a/src/main.ts b/src/main.ts",
"index abc1234..def5678 100644",
"--- a/src/main.ts",
"+++ b/src/main.ts",
"@@ -1,5 +1,7 @@",
" import { start } from \"./server\";",
"+import { logger } from \"./logger\";",
"",
" const port = 3000;",
"+logger.info(\"Starting server...\");",
" start(port);",
"diff --git a/src/server.ts b/src/server.ts",
"index 1111111..2222222 100644",
"--- a/src/server.ts",
"+++ b/src/server.ts",
"@@ -10,3 +10,5 @@",
"   app.listen(port, () => {",
"     console.log(\"Listening on port \" + port);",
"   });",
"+",
"+  return app;",
" }",
].join("\n");
const parsedFiles = parsePatchFiles(sampleDiff).flatMap((p) => p.files);
const firstFileName = parsedFiles[0]?.name ?? "";

const meta: Meta<typeof DiffViewer> = {
	title: "pages/AgentsPage/DiffViewer",
	component: DiffViewer,
	args: {
		parsedFiles,
		diffStyle: "unified" satisfies DiffStyle,
		onLineNumberClick: fn(),
		onLineSelected: fn(),
		onScrollToFileComplete: fn(),
	},
	decorators: [
		(Story) => (
			<div style={{ height: 500, width: 700 }}>
				<Story />
			</div>
		),
	],
};
export default meta;
type Story = StoryObj<typeof DiffViewer>;

export const Default: Story = {};

export const SplitView: Story = {
	args: {
		diffStyle: "split",
	},
};

export const Loading: Story = {
	args: {
		parsedFiles: [],
		isLoading: true,
	},
};

export const ErrorState: Story = {
	name: "Error",
	args: {
		parsedFiles: [],
		error: new Error("Failed to fetch diff"),
	},
};

export const Empty: Story = {
	args: {
		parsedFiles: [],
		emptyMessage: "No file changes to display.",
	},
};

export const WithSelectedLines: Story = {
	args: {
		getSelectedLines: (fileName: string): SelectedLineRange | null => {
			if (fileName === firstFileName) {
				return { start: 2, end: 4, side: "additions" };
			}
			return null;
		},
	},
};

// Diff with two non-adjacent hunks in one file, producing a
// mid-file separator that should remain visible even though
// leading separators are hidden.
// biome-ignore format: raw diff string must preserve exact whitespace
const multiHunkDiff = [
"diff --git a/src/app.ts b/src/app.ts",
"index aaa1111..bbb2222 100644",
"--- a/src/app.ts",
"+++ b/src/app.ts",
"@@ -3,4 +3,5 @@",
" import { db } from \"./db\";",
" import { logger } from \"./logger\";",
"+import { metrics } from \"./metrics\";",
" ",
" const app = express();",
"@@ -20,3 +21,4 @@",
" app.listen(port, () => {",
"   console.log(\"Listening on port \" + port);",
"+  metrics.record(\"server.start\");",
" });",
].join("\n");
const multiHunkFiles = parsePatchFiles(multiHunkDiff).flatMap((p) => p.files);

export const WithMidFileSeparator: Story = {
	args: {
		parsedFiles: multiHunkFiles,
	},
};

export const WithAnnotation: Story = {
	args: {
		getSelectedLines: (fileName: string): SelectedLineRange | null => {
			if (fileName === firstFileName) {
				return { start: 2, end: 4, side: "additions" };
			}
			return null;
		},
		getLineAnnotations: (fileName: string): DiffLineAnnotation<string>[] => {
			if (fileName === firstFileName) {
				return [
					{
						lineNumber: 4,
						side: "additions",
						metadata: "active-input",
					},
				];
			}
			return [];
		},
		renderAnnotation: () => (
			<InlinePromptInput onSubmit={fn()} onCancel={fn()} />
		),
	},
};

// Diff with a change block (both deletions and additions) for
// testing cross-side selection in split view.
// biome-ignore format: raw diff string must preserve exact whitespace
const changeDiff = [
"diff --git a/src/config.ts b/src/config.ts",
"index abc1234..def5678 100644",
"--- a/src/config.ts",
"+++ b/src/config.ts",
"@@ -1,5 +1,5 @@",
" const config = {",
"-  port: 3000,",
"-  host: \"localhost\",",
"+  port: 8080,",
"+  host: \"0.0.0.0\",",
"   debug: false,",
" };",
].join("\n");
const changeFiles = parsePatchFiles(changeDiff).flatMap((p) => p.files);
const changeFileName = changeFiles[0]?.name ?? "";

// Regression test: in split view, selecting from one side to the
// other can produce a range where start === end numerically but
// the sides differ (e.g. deletions line 2 → additions line 2).
// Previously this was incorrectly treated as a single-line click
// and the annotation was never shown.
export const CrossSideAnnotation: Story = {
	args: {
		parsedFiles: changeFiles,
		diffStyle: "split",
		getSelectedLines: (fileName: string): SelectedLineRange | null => {
			if (fileName === changeFileName) {
				return {
					start: 2,
					end: 2,
					side: "deletions",
					endSide: "additions",
				};
			}
			return null;
		},
		getLineAnnotations: (fileName: string): DiffLineAnnotation<string>[] => {
			if (fileName === changeFileName) {
				return [
					{
						lineNumber: 2,
						side: "additions",
						metadata: "active-input",
					},
				];
			}
			return [];
		},
		renderAnnotation: () => (
			<InlinePromptInput onSubmit={fn()} onCancel={fn()} />
		),
	},
	play: async ({ canvasElement }) => {
		// The annotation renders via a slot in the light DOM of the
		// web component, so we can find the textarea directly.
		await waitFor(() => {
			const textarea = canvasElement.querySelector("textarea");
			expect(textarea).not.toBeNull();
		});
	},
};

// Same regression scenario in unified view to ensure the
// annotation also renders when diffStyle is "unified".
export const CrossSideAnnotationUnified: Story = {
	args: {
		...CrossSideAnnotation.args,
		diffStyle: "unified",
	},
	play: CrossSideAnnotation.play,
};

// -------------------------------------------------------------------
// Edge-case stories
// -------------------------------------------------------------------

// Play function shared by all annotation edge-case stories.
const expectAnnotationTextarea = async ({
	canvasElement,
}: {
	canvasElement: HTMLElement;
}) => {
	await waitFor(() => {
		const textarea = canvasElement.querySelector("textarea");
		expect(textarea).not.toBeNull();
	});
};

// Diff where deletion and addition line numbers are wildly
// different (hunk header: @@ -508,4 +218,4 @@). Deletion
// lines are 509-510, addition lines are 219-220.
// biome-ignore format: raw diff string must preserve exact whitespace
const mismatchedLinesDiff = [
"diff --git a/src/big.ts b/src/big.ts",
"index abc1234..def5678 100644",
"--- a/src/big.ts",
"+++ b/src/big.ts",
"@@ -508,6 +218,6 @@ function process() {",
"   return result;",
"-  const old1 = true;",
"-  const old2 = false;",
"+  const new1 = true;",
"+  const new2 = false;",
"   cleanup();",
" }",
].join("\n");
const mismatchedFiles = parsePatchFiles(mismatchedLinesDiff).flatMap(
	(p) => p.files,
);
const mismatchedFileName = mismatchedFiles[0]?.name ?? "";

// Cross-side selection where deletion line 509 maps to addition
// line 219. The old code would Math.min/max these into a
// nonsensical 290-line range.
export const CrossSideMismatchedLineNumbers: Story = {
	args: {
		parsedFiles: mismatchedFiles,
		diffStyle: "split",
		getSelectedLines: (fileName: string): SelectedLineRange | null => {
			if (fileName === mismatchedFileName) {
				return {
					start: 509,
					end: 219,
					side: "deletions",
					endSide: "additions",
				};
			}
			return null;
		},
		getLineAnnotations: (fileName: string): DiffLineAnnotation<string>[] => {
			if (fileName === mismatchedFileName) {
				return [
					{
						lineNumber: 219,
						side: "additions",
						metadata: "active-input",
					},
				];
			}
			return [];
		},
		renderAnnotation: () => (
			<InlinePromptInput onSubmit={fn()} onCancel={fn()} />
		),
	},
	play: expectAnnotationTextarea,
};

// Same mismatched-line-number scenario in unified view.
export const CrossSideMismatchedLineNumbersUnified: Story = {
	args: {
		...CrossSideMismatchedLineNumbers.args,
		diffStyle: "unified",
	},
	play: expectAnnotationTextarea,
};

// Backward same-side selection (start > end). The user clicks
// line 9 then shift-clicks line 5 on the additions side.
// biome-ignore format: raw diff string must preserve exact whitespace
const backwardSelectionDiff = [
"diff --git a/src/utils.ts b/src/utils.ts",
"index abc1234..def5678 100644",
"--- a/src/utils.ts",
"+++ b/src/utils.ts",
"@@ -3,4 +3,9 @@",
" import { foo } from \"./foo\";",
" import { bar } from \"./bar\";",
"+import { baz } from \"./baz\";",
"+import { qux } from \"./qux\";",
"+import { quux } from \"./quux\";",
"+import { corge } from \"./corge\";",
"+import { grault } from \"./grault\";",
" ",
" export function main() {",
].join("\n");
const backwardFiles = parsePatchFiles(backwardSelectionDiff).flatMap(
	(p) => p.files,
);
const backwardFileName = backwardFiles[0]?.name ?? "";

// Backward selection: start=9 > end=5 on the same side.
// The annotation should appear at line 5 (the end point).
export const BackwardSameSideSelection: Story = {
	args: {
		parsedFiles: backwardFiles,
		diffStyle: "unified",
		getSelectedLines: (fileName: string): SelectedLineRange | null => {
			if (fileName === backwardFileName) {
				return { start: 9, end: 5, side: "additions" };
			}
			return null;
		},
		getLineAnnotations: (fileName: string): DiffLineAnnotation<string>[] => {
			if (fileName === backwardFileName) {
				return [
					{
						lineNumber: 5,
						side: "additions",
						metadata: "active-input",
					},
				];
			}
			return [];
		},
		renderAnnotation: () => (
			<InlinePromptInput onSubmit={fn()} onCancel={fn()} />
		),
	},
	play: expectAnnotationTextarea,
};

// Cross-side selection going additions -> deletions (the
// reverse of the typical del -> add direction).
export const CrossSideAdditionsToDeletions: Story = {
	args: {
		parsedFiles: changeFiles,
		diffStyle: "split",
		getSelectedLines: (fileName: string): SelectedLineRange | null => {
			if (fileName === changeFileName) {
				return {
					start: 2,
					end: 3,
					side: "additions",
					endSide: "deletions",
				};
			}
			return null;
		},
		getLineAnnotations: (fileName: string): DiffLineAnnotation<string>[] => {
			if (fileName === changeFileName) {
				return [
					{
						lineNumber: 3,
						side: "deletions",
						metadata: "active-input",
					},
				];
			}
			return [];
		},
		renderAnnotation: () => (
			<InlinePromptInput onSubmit={fn()} onCancel={fn()} />
		),
	},
	play: expectAnnotationTextarea,
};
