import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ChatDiffStatusResponse } from "api/api";
import { API } from "api/api";
import type { ChatDiffContents } from "api/typesGenerated";
import { expect, screen, spyOn } from "storybook/test";
import { FilesChangedPanel } from "./FilesChangedPanel";

// ---------------------------------------------------------------------------
// Large-diff generator — produces a realistic unified diff with the
// requested number of additions and deletions spread across multiple
// TypeScript files.  Used by the LargeDiff story to reproduce the
// performance characteristics of a +2000 -1000 agent chat.
// ---------------------------------------------------------------------------

/** Generate a block of realistic-looking TypeScript lines. */
function tsLines(prefix: string, count: number, startIdx = 0): string[] {
	const templates = [
		(i: number) => `  const ${prefix}Val${i} = computeValue(${i}, opts);`,
		(i: number) => `  if (${prefix}Val${i} !== undefined) {`,
		(i: number) =>
			`    logger.info("Processing ${prefix} item", { index: ${i} });`,
		(i: number) => `    results.push(await transform(${prefix}Val${i}));`,
		(_i: number) => "  }",
		(i: number) => `  // Handle edge case for ${prefix} iteration ${i}`,
		(i: number) =>
			`  const ${prefix}Mapped${i} = items.map((x) => x.${prefix}Field${i});`,
		(i: number) =>
			`  await db.query(\`SELECT * FROM ${prefix}_table WHERE id = \${${i}}\`);`,
		(i: number) =>
			`  export function ${prefix}Handler${i}(req: Request): Response {`,
		(i: number) =>
			`    return new Response(JSON.stringify({ ${prefix}: ${i} }));`,
	];
	const lines: string[] = [];
	for (let i = 0; i < count; i++) {
		const tpl = templates[(startIdx + i) % templates.length];
		lines.push(tpl(startIdx + i));
	}
	return lines;
}

/**
 * Build a single file section of a unified diff. `contextLines` lines
 * of shared context surround each hunk.  `additions` new lines and
 * `deletions` removed lines are interleaved across multiple hunks.
 */
function buildFileDiff(opts: {
	oldPath: string;
	newPath: string;
	additions: number;
	deletions: number;
	contextPerHunk?: number;
	hunks?: number;
}): string {
	const {
		oldPath,
		newPath,
		additions,
		deletions,
		contextPerHunk = 5,
		hunks = Math.max(1, Math.ceil((additions + deletions) / 80)),
	} = opts;

	const addPerHunk = Math.ceil(additions / hunks);
	const delPerHunk = Math.ceil(deletions / hunks);
	const prefix = oldPath.replace(/[^a-zA-Z]/g, "").slice(0, 6);

	let output = `diff --git a/${oldPath} b/${newPath}\n`;
	output += "index 1a2b3c4..5d6e7f8 100644\n";
	output += `--- a/${oldPath}\n`;
	output += `+++ b/${newPath}\n`;

	let oldLine = 1;
	let additionsLeft = additions;
	let deletionsLeft = deletions;

	for (let h = 0; h < hunks; h++) {
		const ctxLines = tsLines(prefix, contextPerHunk, oldLine);
		const hunkDel = Math.min(delPerHunk, deletionsLeft);
		const hunkAdd = Math.min(addPerHunk, additionsLeft);
		deletionsLeft -= hunkDel;
		additionsLeft -= hunkAdd;

		const oldCount = contextPerHunk + hunkDel + contextPerHunk;
		const newCount = contextPerHunk + hunkAdd + contextPerHunk;
		output += `@@ -${oldLine},${oldCount} +${oldLine},${newCount} @@\n`;

		// Leading context.
		for (const l of ctxLines) {
			output += ` ${l}\n`;
		}

		// Deletions.
		for (const l of tsLines(
			`old${prefix}`,
			hunkDel,
			oldLine + contextPerHunk,
		)) {
			output += `-${l}\n`;
		}

		// Additions.
		for (const l of tsLines(
			`new${prefix}`,
			hunkAdd,
			oldLine + contextPerHunk,
		)) {
			output += `+${l}\n`;
		}

		// Trailing context.
		for (const l of tsLines(
			prefix,
			contextPerHunk,
			oldLine + contextPerHunk + hunkDel,
		)) {
			output += ` ${l}\n`;
		}

		oldLine += oldCount + 20;
	}

	return output;
}

/**
 * Generate a complete multi-file unified diff targeting roughly
 * `totalAdditions` added lines and `totalDeletions` removed lines.
 */
function generateLargeDiff(
	totalAdditions: number,
	totalDeletions: number,
): string {
	const files = [
		{
			path: "site/src/pages/AgentsPage/AgentDetail.tsx",
			addPct: 0.25,
			delPct: 0.2,
		},
		{
			path: "site/src/components/ai-elements/tool/utils.ts",
			addPct: 0.15,
			delPct: 0.15,
		},
		{
			path: "site/src/pages/AgentsPage/FilesChangedPanel.tsx",
			addPct: 0.15,
			delPct: 0.1,
		},
		{ path: "site/src/api/queries/chats.ts", addPct: 0.1, delPct: 0.15 },
		{
			path: "site/src/modules/resources/AgentLogs/AgentLogs.tsx",
			addPct: 0.1,
			delPct: 0.1,
		},
		{ path: "coderd/database/queries/chats.sql", addPct: 0.05, delPct: 0.1 },
		{
			path: "site/src/components/SyntaxHighlighter/SyntaxHighlighter.tsx",
			addPct: 0.1,
			delPct: 0.05,
		},
		{
			path: "site/src/pages/AgentsPage/RightPanel.tsx",
			addPct: 0.05,
			delPct: 0.1,
		},
		{ path: "site/src/hooks/useDiffViewer.ts", addPct: 0.05, delPct: 0.05 },
	];
	const patches: string[] = [];
	for (const f of files) {
		const add = Math.round(totalAdditions * f.addPct);
		const del = Math.round(totalDeletions * f.delPct);
		if (add === 0 && del === 0) continue;
		patches.push(
			buildFileDiff({
				oldPath: f.path,
				newPath: f.path,
				additions: add,
				deletions: del,
			}),
		);
	}
	return patches.join("");
}

const sampleUnifiedDiff = `diff --git a/site/src/pages/AgentsPage/FilesChangedPanel.tsx b/site/src/pages/AgentsPage/FilesChangedPanel.tsx
index abc1234..def5678 100644
--- a/site/src/pages/AgentsPage/FilesChangedPanel.tsx
+++ b/site/src/pages/AgentsPage/FilesChangedPanel.tsx
@@ -1,10 +1,15 @@
+import { useTheme } from "@emotion/react";
 import { parsePatchFiles } from "@pierre/diffs";
 import { FileDiff } from "@pierre/diffs/react";
+import { createContext, useCallback, useContext, useEffect, useLayoutEffect, useMemo, useReducer, useRef, useState, useSyncExternalStore } from "react"; // deliberately long import to verify horizontal overflow handling in narrow panels
+import {
+  DIFFS_FONT_STYLE,
+  getDiffViewerOptions,
+} from "components/ai-elements/tool/utils";
 import { chatDiffContents, chatDiffStatus } from "api/queries/chats";
 import { ErrorAlert } from "components/Alert/ErrorAlert";
 import { ScrollArea } from "components/ScrollArea/ScrollArea";
-import { Skeleton } from "components/Skeleton/Skeleton";
 import { type FC, useMemo } from "react";
 import { useQuery } from "react-query";
diff --git a/site/src/components/ai-elements/tool/utils.ts b/site/src/components/ai-elements/tool/utils.ts
index 1234567..abcdef0 100644
--- a/site/src/components/ai-elements/tool/utils.ts
+++ b/site/src/components/ai-elements/tool/utils.ts
@@ -10,6 +10,18 @@ export const diffViewerCSS =
 export function getDiffViewerOptions(isDark: boolean) {
   return {
     diffStyle: "unified" as const,
+    diffIndicators: "bars" as const,
+    overflow: "scroll" as const,
     themeType: (isDark ? "dark" : "light") as "dark" | "light",
+    theme: isDark ? "github-dark-high-contrast" : "github-light",
+    unsafeCSS: diffViewerCSS,
   };
 }
+
+export const DIFFS_FONT_STYLE = {
+  "--diffs-font-size": "11px",
+  "--diffs-line-height": "1.5",
+} as React.CSSProperties;
`;

const defaultDiffStatus: ChatDiffStatusResponse = {
	chat_id: "test-chat",
	changes_requested: false,
	additions: 0,
	deletions: 0,
	changed_files: 0,
};

const defaultDiffContents: ChatDiffContents = {
	chat_id: "test-chat",
};

const meta: Meta<typeof FilesChangedPanel> = {
	title: "pages/AgentsPage/FilesChangedPanel",
	component: FilesChangedPanel,
	args: {
		chatId: "test-chat",
	},
	decorators: [
		(Story) => (
			<div style={{ height: 600, width: 500 }}>
				<Story />
			</div>
		),
	],
	beforeEach: () => {
		spyOn(API, "getChatDiffStatus").mockResolvedValue(defaultDiffStatus);
		spyOn(API, "getChatDiffContents").mockResolvedValue(defaultDiffContents);
	},
};

export default meta;
type Story = StoryObj<typeof FilesChangedPanel>;

export const EmptyDiff: Story = {
	beforeEach: () => {
		spyOn(API, "getChatDiffStatus").mockResolvedValue({
			...defaultDiffStatus,
			url: undefined,
		});
		spyOn(API, "getChatDiffContents").mockResolvedValue({
			...defaultDiffContents,
			diff: "",
		});
	},
	play: async () => {
		await screen.findByText("No file changes to display.");
		expect(screen.getByText("No file changes to display.")).toBeInTheDocument();
	},
};

export const ParseError: Story = {
	beforeEach: () => {
		spyOn(API, "getChatDiffStatus").mockResolvedValue({
			...defaultDiffStatus,
			url: "https://github.com/coder/coder/pull/123",
		});
		spyOn(API, "getChatDiffContents").mockResolvedValue({
			...defaultDiffContents,
			diff: "not-a-valid-unified-diff",
		});
	},
	play: async () => {
		await screen.findByText("No file changes to display.");
		expect(screen.getByText("No file changes to display.")).toBeInTheDocument();
	},
};

export const WithDiffDark: Story = {
	beforeEach: () => {
		spyOn(API, "getChatDiffStatus").mockResolvedValue({
			...defaultDiffStatus,
			url: "https://github.com/coder/coder/pull/456",
			additions: 14,
			deletions: 2,
			changed_files: 2,
		});
		spyOn(API, "getChatDiffContents").mockResolvedValue({
			...defaultDiffContents,
			diff: sampleUnifiedDiff,
		});
	},
};

export const WithDiffLight: Story = {
	globals: {
		theme: "light",
	},
	beforeEach: () => {
		spyOn(API, "getChatDiffStatus").mockResolvedValue({
			...defaultDiffStatus,
			url: "https://github.com/coder/coder/pull/456",
			additions: 14,
			deletions: 2,
			changed_files: 2,
		});
		spyOn(API, "getChatDiffContents").mockResolvedValue({
			...defaultDiffContents,
			diff: sampleUnifiedDiff,
		});
	},
};

export const NoPullRequestDark: Story = {
	beforeEach: () => {
		spyOn(API, "getChatDiffStatus").mockResolvedValue({
			...defaultDiffStatus,
			url: "https://github.com/coder/coder/pull/456",
			additions: 14,
			deletions: 2,
			changed_files: 2,
		});
		spyOn(API, "getChatDiffContents").mockResolvedValue({
			...defaultDiffContents,
			diff: sampleUnifiedDiff,
		});
	},
};

export const NoPullRequestLight: Story = {
	globals: {
		theme: "light",
	},
	beforeEach: () => {
		spyOn(API, "getChatDiffStatus").mockResolvedValue({
			...defaultDiffStatus,
			url: "https://github.com/coder/coder/pull/456",
			additions: 14,
			deletions: 2,
			changed_files: 2,
		});
		spyOn(API, "getChatDiffContents").mockResolvedValue({
			...defaultDiffContents,
			diff: sampleUnifiedDiff,
		});
	},
};

/**
 * Stress-test story: renders a diff with approximately +2000 -1000
 * lines spread across 9 TypeScript files. Use this to reproduce
 * and profile the performance issues reported for large agent chats.
 *
 * Open the browser DevTools Performance tab and record while this
 * story loads to measure:
 *  - Initial render blocking time (shiki tokenization)
 *  - Total DOM node count
 *  - Scroll jank / frame drops
 */
export const LargeDiff: Story = {
	decorators: [
		(Story) => (
			<div style={{ height: 800, width: 600 }}>
				<Story />
			</div>
		),
	],
	beforeEach: () => {
		const largeDiff = generateLargeDiff(2000, 1000);
		spyOn(API, "getChatDiffStatus").mockResolvedValue({
			...defaultDiffStatus,
			url: "https://github.com/coder/coder/pull/789",
			additions: 2000,
			deletions: 1000,
			changed_files: 9,
		});
		spyOn(API, "getChatDiffContents").mockResolvedValue({
			...defaultDiffContents,
			diff: largeDiff,
		});
	},
};

/**
 * Same as LargeDiff but in light mode for visual comparison.
 */
export const LargeDiffLight: Story = {
	globals: {
		theme: "light",
	},
	decorators: [
		(Story) => (
			<div style={{ height: 800, width: 600 }}>
				<Story />
			</div>
		),
	],
	beforeEach: () => {
		const largeDiff = generateLargeDiff(2000, 1000);
		spyOn(API, "getChatDiffStatus").mockResolvedValue({
			...defaultDiffStatus,
			url: "https://github.com/coder/coder/pull/789",
			additions: 2000,
			deletions: 1000,
			changed_files: 9,
		});
		spyOn(API, "getChatDiffContents").mockResolvedValue({
			...defaultDiffContents,
			diff: largeDiff,
		});
	},
};
