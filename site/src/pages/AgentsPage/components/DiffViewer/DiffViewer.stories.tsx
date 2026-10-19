import type { DiffLineAnnotation, SelectedLineRange } from "@pierre/diffs";
import { FileDiff } from "@pierre/diffs/react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { fn, userEvent, waitFor, within } from "storybook/test";
import type { DiffStyle } from "../DiffViewer/DiffViewer";
import { compareTreePaths, DiffViewer } from "../DiffViewer/DiffViewer";
import { parseDiffString } from "../DiffViewer/parseDiff";
import { InlinePromptInput } from "../DiffViewer/RemoteDiffPanel";
import { generateLargeDiff } from "./testHelpers";

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
const parsedFiles = parseDiffString(sampleDiff);
const firstFileName = parsedFiles[0]?.name ?? "";

const meta: Meta<typeof DiffViewer> = {
	title: "pages/AgentsPage/DiffViewer",
	component: DiffViewer,
	args: {
		parsedFiles,
		diffStyle: "unified" satisfies DiffStyle,
		isExpanded: false,
		onLineNumberClick: fn(),
		onLineSelected: fn(),
		onLineSelectionChange: fn(),
		getLineAnnotations: () => [],
		getSelectedLines: () => null,
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
const multiHunkFiles = parseDiffString(multiHunkDiff);

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
const changeFiles = parseDiffString(changeDiff);
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
};

// Same regression scenario in unified view to ensure the
// annotation also renders when diffStyle is "unified".
export const CrossSideAnnotationUnified: Story = {
	args: {
		...CrossSideAnnotation.args,
		diffStyle: "unified",
	},
};

// -------------------------------------------------------------------
// Edge-case stories
// -------------------------------------------------------------------

// Play function shared by all annotation edge-case stories.
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
const mismatchedFiles = parseDiffString(mismatchedLinesDiff);
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
};

// Same mismatched-line-number scenario in unified view.
export const CrossSideMismatchedLineNumbersUnified: Story = {
	args: {
		...CrossSideMismatchedLineNumbers.args,
		diffStyle: "unified",
	},
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
const backwardFiles = parseDiffString(backwardSelectionDiff);
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
};

// Rename diff with long file paths to verify that:
// 1. The arrow between old and new names is vertically centered
// 2. The +N −N line counts remain visible (not clipped)
// 3. File names truncate with ellipsis
// biome-ignore format: raw diff string must preserve exact whitespace
const renameDiff = [
"diff --git a/site/src/pages/AgentsPage/components/LimitsTab/DefaultLimitSection.tsx b/site/src/pages/AgentsPage/components/SpendingTab/DefaultLimitSection.tsx",
"similarity index 95%",
"rename from site/src/pages/AgentsPage/components/LimitsTab/DefaultLimitSection.tsx",
"rename to site/src/pages/AgentsPage/components/SpendingTab/DefaultLimitSection.tsx",
"index abc1234..def5678 100644",
"--- a/site/src/pages/AgentsPage/components/LimitsTab/DefaultLimitSection.tsx",
"+++ b/site/src/pages/AgentsPage/components/SpendingTab/DefaultLimitSection.tsx",
"@@ -1,3 +1,3 @@",
" export function DefaultLimitSection() {",
"-  return null;",
"+  return <div />;",
" }",
].join("\n");
const renameFiles = parseDiffString(renameDiff);

export const RenameWithLongPaths: Story = {
	args: {
		parsedFiles: renameFiles,
	},
};

export const LargeDiff: Story = {
	args: {
		parsedFiles: parseDiffString(generateLargeDiff(40, 60)),
		isExpanded: true,
	},
	decorators: [
		(Story) => (
			<div style={{ height: 800, width: 900 }}>
				<Story />
			</div>
		),
	],
};

// In production, before content-derived keys, the second render could hit
// the worker-pool AST cached for the first body and throw "deletionLine and
// additionLine are null". The storybook worker timing cannot reproduce that
// collision window, so this story smoke-tests the re-render path instead:
// the second body renders and no error box appears.
const reparseFirstBody = [
	"--- a/src/hot.ts",
	"+++ b/src/hot.ts",
	"@@ -1,2 +1,2 @@",
	" export const keep = true;",
	"-const v = 1;",
	"+const v = 2;",
].join("\n");
const reparseSecondBody = [
	"--- a/src/hot.ts",
	"+++ b/src/hot.ts",
	"@@ -1,2 +1,5 @@",
	" export const keep = true;",
	"-const v = 1;",
	"+const v = 3;",
	"+const a = 1;",
	"+const b = 2;",
	"+const c = 3;",
].join("\n");

// FileDiff renders hunks synchronously enough for play tests.
const ReparseSamePath: React.FC = () => {
	const [body, setBody] = useState(reparseFirstBody);
	const file = parseDiffString(body)[0];
	return (
		<div style={{ height: 400, width: 600 }}>
			<button type="button" onClick={() => setBody(reparseSecondBody)}>
				next body
			</button>
			{file && (
				<FileDiff
					fileDiff={file}
					options={{
						diffStyle: "unified",
						theme: "github-dark-high-contrast",
						themeType: "dark",
					}}
				/>
			)}
		</div>
	);
};

/**
 * Resolve once a given diff body has rendered inside the shadow root that
 * Pixel's DOM-idle check cannot observe.
 */
const waitForDiffBody = (canvasElement: HTMLElement, text: string) =>
	waitFor(() => {
		const rendered = Array.from(
			canvasElement.querySelectorAll("diffs-container"),
		).some((host) => host.shadowRoot?.textContent?.includes(text));
		if (!rendered) {
			throw new Error(`Diff body has not rendered yet: ${text}`);
		}
	});

export const ReparseSamePathAfterEdit: StoryObj = {
	render: () => <ReparseSamePath />,
	play: async ({ canvasElement }) => {
		// The regression is a reparse: the first parse must commit before the
		// swap, or body 2 renders as a fresh parse and the bug is not exercised.
		await waitForDiffBody(canvasElement, "const v = 2");

		await userEvent.click(
			within(canvasElement).getByRole("button", { name: "next body" }),
		);

		// Shadow-root renders are invisible to the stability wait, so wait for
		// the reparsed body.
		await waitForDiffBody(canvasElement, "const v = 3");
	},
};

const ReparseInDiffViewer: React.FC<React.ComponentProps<typeof DiffViewer>> = (
	props,
) => {
	const [body, setBody] = useState(reparseFirstBody);
	return (
		<div className="flex h-full flex-col">
			<button type="button" onClick={() => setBody(reparseSecondBody)}>
				next body
			</button>
			<DiffViewer {...props} parsedFiles={parseDiffString(body)} />
		</div>
	);
};

// The same path with new content must re-render, so the CodeView item
// version has to change with the file content.
export const ReparseSamePathInDiffViewer: Story = {
	render: (args) => <ReparseInDiffViewer {...args} />,
	play: async ({ canvasElement }) => {
		await waitForDiffBody(canvasElement, "const v = 2");
		await userEvent.click(
			within(canvasElement).getByRole("button", { name: "next body" }),
		);
		await waitForDiffBody(canvasElement, "const v = 3");
	},
};

const waitForNoDiffBody = (canvasElement: HTMLElement, text: string) =>
	waitFor(() => {
		const rendered = Array.from(
			canvasElement.querySelectorAll("diffs-container"),
		).some((host) => host.shadowRoot?.textContent?.includes(text));
		if (rendered) {
			throw new Error(`Diff body is still rendered: ${text}`);
		}
	});

// A line from src/main.ts in sampleDiff.
const firstFileLine = "Starting server...";

export const CollapseAndExpandFile: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await waitForDiffBody(canvasElement, firstFileLine);

		await userEvent.click(
			await canvas.findByRole("button", { name: `Collapse ${firstFileName}` }),
		);
		await waitForNoDiffBody(canvasElement, firstFileLine);

		await userEvent.click(
			await canvas.findByRole("button", { name: `Expand ${firstFileName}` }),
		);
		await waitForDiffBody(canvasElement, firstFileLine);
	},
};

const ScrollToCollapsedFile: React.FC<
	React.ComponentProps<typeof DiffViewer>
> = (props) => {
	const [target, setTarget] = useState<string | null>(null);
	return (
		<div className="flex h-full flex-col">
			<button type="button" onClick={() => setTarget(firstFileName)}>
				jump to file
			</button>
			<DiffViewer
				{...props}
				scrollToFile={target}
				onScrollToFileComplete={() => setTarget(null)}
			/>
		</div>
	);
};

export const ScrollToFileExpandsCollapsedFile: Story = {
	render: (args) => <ScrollToCollapsedFile {...args} />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await waitForDiffBody(canvasElement, firstFileLine);
		await userEvent.click(
			await canvas.findByRole("button", { name: `Collapse ${firstFileName}` }),
		);
		await waitForNoDiffBody(canvasElement, firstFileLine);

		await userEvent.click(canvas.getByRole("button", { name: "jump to file" }));
		await waitForDiffBody(canvasElement, firstFileLine);
	},
};

// CodeView's scroll root is the nearest scrolling ancestor of a file card.
const findScroller = (canvasElement: HTMLElement): HTMLElement => {
	let el = canvasElement.querySelector("diffs-container")?.parentElement;
	while (el && getComputedStyle(el).overflowY !== "auto") {
		el = el.parentElement;
	}
	if (!el) {
		throw new Error("CodeView scroll root not found");
	}
	return el;
};

// The file tree renders its rows in a shadow root.
const treeRows = (canvasElement: HTMLElement): HTMLElement[] =>
	Array.from(canvasElement.querySelectorAll("*")).flatMap((el) =>
		el.shadowRoot
			? Array.from(
					el.shadowRoot.querySelectorAll<HTMLElement>("[data-item-path]"),
				)
			: [],
	);

const selectedTreePaths = (canvasElement: HTMLElement): string[] =>
	treeRows(canvasElement)
		.filter((row) => row.getAttribute("aria-selected") === "true")
		.map((row) => row.dataset.itemPath ?? "");

// CodeView disables pointer events while a scroll is in progress.
const clickAfterScroll = async (element: HTMLElement) => {
	await waitFor(() => {
		if (getComputedStyle(element).pointerEvents === "none") {
			throw new Error("Scroll still in progress");
		}
	});
	await userEvent.click(element);
};

const longFiles = parseDiffString(generateLargeDiff(4, 300));
const firstLongFile =
	[...longFiles].sort((a, b) => compareTreePaths(a.name, b.name))[0]?.name ??
	"";

// Collapsing from a sticky header scrolls the file to its start, and the
// resulting scroll event must not leave the collapsed file selected in the
// tree, or clicking its row would not expand it.
export const TreeClickExpandsFileCollapsedFromStickyHeader: Story = {
	args: {
		parsedFiles: longFiles,
		isExpanded: true,
	},
	decorators: [
		(Story) => (
			<div style={{ height: 600, width: 1000 }}>
				<Story />
			</div>
		),
	],
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const bodyText = `in ${firstLongFile}`;
		await waitForDiffBody(canvasElement, bodyText);
		const scroller = findScroller(canvasElement);
		scroller.scrollTop = 2000;

		await clickAfterScroll(
			await canvas.findByRole("button", { name: `Collapse ${firstLongFile}` }),
		);
		await waitForNoDiffBody(canvasElement, bodyText);
		await waitFor(() => {
			if (selectedTreePaths(canvasElement).includes(firstLongFile)) {
				throw new Error("Collapsed file is still selected in the tree");
			}
		});

		const row = treeRows(canvasElement).find(
			(r) => r.dataset.itemPath === firstLongFile,
		);
		if (!row) {
			throw new Error(`Tree row not found: ${firstLongFile}`);
		}
		await userEvent.click(row);
		await waitForDiffBody(canvasElement, bodyText);
	},
};

const shortFiles = parseDiffString(generateLargeDiff(20, 3));
const sortedShortFiles = [...shortFiles]
	.sort((a, b) => compareTreePaths(a.name, b.name))
	.map((file) => file.name);

// Collapsing a file other than the active one keeps the tree highlight.
export const CollapsingAnotherFileKeepsTreeSelection: Story = {
	args: {
		parsedFiles: shortFiles,
		isExpanded: true,
	},
	decorators: [
		(Story) => (
			<div style={{ height: 600, width: 1000 }}>
				<Story />
			</div>
		),
	],
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const [activeName, otherName] = sortedShortFiles;
		await waitForDiffBody(canvasElement, `in ${otherName}`);
		findScroller(canvasElement).scrollTop = 5;
		await waitFor(() => {
			if (!selectedTreePaths(canvasElement).includes(activeName)) {
				throw new Error(`${activeName} is not selected yet`);
			}
		});

		await clickAfterScroll(
			await canvas.findByRole("button", { name: `Collapse ${otherName}` }),
		);
		await waitForNoDiffBody(canvasElement, `in ${otherName}`);
		await waitFor(() => {
			const selected = selectedTreePaths(canvasElement);
			if (selected.length !== 1 || selected[0] !== activeName) {
				throw new Error(`Tree selection is ${JSON.stringify(selected)}`);
			}
		});
	},
};

// Matches FILE_LIST_PADDING in DiffViewer.tsx.
const listPadding = 8;
const overflowLinesPerFile = 5;
const overflowFiles = parseDiffString(
	generateLargeDiff(30, overflowLinesPerFile),
);
const sortedOverflowFiles = [...overflowFiles]
	.sort((a, b) => compareTreePaths(a.name, b.name))
	.map((file) => file.name);
const lastOverflowLine = `context line ${overflowLinesPerFile} in ${sortedOverflowFiles.at(-1)}`;

// File heights come from itemMetrics, so they must match the rendered card,
// code inset included. At maximum scroll the last card then ends exactly the
// list padding above the bottom of the list.
export const OverflowScrollsToLastCard: Story = {
	args: {
		parsedFiles: overflowFiles,
	},
	play: async ({ canvasElement }) => {
		await waitFor(() => findScroller(canvasElement));
		const scroller = findScroller(canvasElement);

		await waitFor(() => {
			scroller.scrollTop = scroller.scrollHeight;
			const cards = canvasElement.querySelectorAll("diffs-container");
			const lastCard = cards[cards.length - 1];
			if (!lastCard?.shadowRoot?.textContent?.includes(lastOverflowLine)) {
				throw new Error("Last file has not rendered yet");
			}
			const gap =
				scroller.getBoundingClientRect().bottom -
				lastCard.getBoundingClientRect().bottom;
			if (Math.abs(gap - listPadding) > 0.5) {
				throw new Error(
					`Last card ends ${gap}px above the list bottom, want ${listPadding}px`,
				);
			}
		});
	},
};

// Headless Chromium has no classic scrollbars, so the story fakes one by
// making the scroll root report a wider offsetWidth, then collapses a file so
// the content resize re-runs the scrollbar check.
export const ClassicScrollbarDropsRightPadding: Story = {
	args: {
		parsedFiles: overflowFiles,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const firstName = sortedOverflowFiles[0] ?? "";
		await waitForDiffBody(canvasElement, `in ${firstName}`);
		const scroller = findScroller(canvasElement);
		const paddingRight = () => getComputedStyle(scroller).paddingRight;
		if (paddingRight() !== `${listPadding}px`) {
			throw new Error(
				`padding-right is ${paddingRight()} without a scrollbar, want ${listPadding}px`,
			);
		}

		Object.defineProperty(scroller, "offsetWidth", {
			configurable: true,
			get: () => scroller.clientWidth + 12,
		});
		try {
			await userEvent.click(
				await canvas.findByRole("button", { name: `Collapse ${firstName}` }),
			);
			await waitFor(() => {
				if (paddingRight() !== "0px") {
					throw new Error(
						`padding-right is ${paddingRight()} with a classic scrollbar, want 0px`,
					);
				}
			});
		} finally {
			Reflect.deleteProperty(scroller, "offsetWidth");
		}
	},
};

const headerRadii = (canvasElement: HTMLElement): string => {
	const header = canvasElement
		.querySelector("diffs-container")
		?.shadowRoot?.querySelector<HTMLElement>("[data-diffs-header]");
	if (!header) {
		throw new Error("Header not rendered");
	}
	const style = getComputedStyle(header);
	return [
		style.borderTopLeftRadius,
		style.borderTopRightRadius,
		style.borderBottomRightRadius,
		style.borderBottomLeftRadius,
	].join(" ");
};

// An expanded header rounds only its top corners, above the code. A collapsed
// file has no code block, so its header rounds every corner.
export const CollapsedHeaderIsFullyRounded: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await waitForDiffBody(canvasElement, firstFileLine);
		await waitFor(() => {
			const radii = headerRadii(canvasElement);
			if (radii !== "6px 6px 0px 0px") {
				throw new Error(
					`Expanded header radii are ${radii}, want 6px 6px 0px 0px`,
				);
			}
		});

		await userEvent.click(
			await canvas.findByRole("button", { name: `Collapse ${firstFileName}` }),
		);
		await waitForNoDiffBody(canvasElement, firstFileLine);
		await waitFor(() => {
			const radii = headerRadii(canvasElement);
			if (radii !== "6px 6px 6px 6px") {
				throw new Error(
					`Collapsed header radii are ${radii}, want 6px 6px 6px 6px`,
				);
			}
		});
	},
};

// Scrolls partway into a long file so its header is stuck, which renders the
// rounded-corner mask for the visual snapshot.
export const StickyHeaderCorners: Story = {
	args: {
		parsedFiles: longFiles,
	},
	play: async ({ canvasElement }) => {
		await waitForDiffBody(canvasElement, `in ${firstLongFile}`);
		findScroller(canvasElement).scrollTop = 600;
		await waitForDiffBody(canvasElement, `context line 60 in ${firstLongFile}`);
	},
};
