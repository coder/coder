import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ChatDiffStatusResponse } from "api/api";
import { API } from "api/api";
import type { ChatDiffContents } from "api/typesGenerated";
import { expect, screen, spyOn } from "storybook/test";
import { FilesChangedPanel } from "./FilesChangedPanel";

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
