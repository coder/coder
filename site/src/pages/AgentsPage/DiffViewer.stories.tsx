import type { DiffLineAnnotation, SelectedLineRange } from "@pierre/diffs";
import { parsePatchFiles } from "@pierre/diffs";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import type { DiffStyle } from "./DiffViewer";
import { DiffViewer } from "./DiffViewer";
import { InlinePromptInput } from "./RemoteDiffPanel";

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
