import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, spyOn, userEvent, waitFor, within } from "storybook/test";
import { API } from "#/api/api";
import type {
	ChatDiffContents,
	ChatDiffStatus,
	WorkspaceAgentRepoChanges,
} from "#/api/typesGenerated";
import { MockChatDiffStatus } from "#/testHelpers/chatEntities";
import { generateLargeDiff } from "../DiffViewer/testHelpers";
import { GitPanel } from "./GitPanel";

// ---------------------------------------------------------------------------
// Shared fixtures
// ---------------------------------------------------------------------------

const sampleDiff = `diff --git a/src/main.ts b/src/main.ts
index abc1234..def5678 100644
--- a/src/main.ts
+++ b/src/main.ts
@@ -1,5 +1,7 @@
 import { start } from "./server";
+import { logger } from "./logger";

 const port = 3000;
+logger.info("Starting server...");
 start(port);
diff --git a/src/server.ts b/src/server.ts
index 1111111..2222222 100644
--- a/src/server.ts
+++ b/src/server.ts
@@ -10,3 +10,5 @@
   app.listen(port, () => {
     console.log("Listening on port " + port);
   });
+
+  return app;
 }
`;

const secondRepoDiff = `diff --git a/README.md b/README.md
index aaa1111..bbb2222 100644
--- a/README.md
+++ b/README.md
@@ -1,3 +1,5 @@
 # My Project
+
+This project does things.

 ## Getting Started
-Follow the steps below.
+Follow the steps below to get started.
`;

const makeRepo = (
	overrides: Partial<WorkspaceAgentRepoChanges> = {},
): WorkspaceAgentRepoChanges => ({
	repo_root: "/home/coder/coder",
	branch: "feat/add-logging",
	remote_origin: "https://github.com/coder/coder.git",
	unified_diff: sampleDiff,
	...overrides,
});

const mockDiffContents: ChatDiffContents = {
	chat_id: "test-chat",
};

// The default PR shown across GitPanel stories.
const makePrStatus = (overrides: Partial<ChatDiffStatus> = {}) => [
	{
		...MockChatDiffStatus,
		chat_id: "test-chat",
		url: "https://github.com/coder/coder/pull/23020",
		pr_number: 23020,
		pull_request_title: "feat(agents): add MCP server configuration to agents",
		pull_request_state: "open",
		base_branch: "main",
		head_branch: "feat/add-mcp-config",
		git_branch: "feat/add-mcp-config",
		additions: 4037,
		deletions: 7,
		changed_files: 12,
		...overrides,
	},
];

// ---------------------------------------------------------------------------
// Meta
// ---------------------------------------------------------------------------

const meta: Meta<typeof GitPanel> = {
	title: "pages/AgentsPage/GitPanel",
	component: GitPanel,
	args: {
		onRefresh: fn().mockReturnValue(true),
		onCommit: fn(),
		repositories: new Map(),
	},
	decorators: [
		(Story) => (
			<div style={{ height: 600, width: 480 }}>
				<Story />
			</div>
		),
	],
	beforeEach: () => {
		spyOn(API.experimental, "getChatDiffContents").mockResolvedValue(
			mockDiffContents,
		);
	},
};

export default meta;
type Story = StoryObj<typeof GitPanel>;

// ---------------------------------------------------------------------------
// Stories
// ---------------------------------------------------------------------------

/** PR is open with a title, head/base branches, and working changes. */
export const PullRequestAndWorkingChanges: Story = {
	args: {
		chatId: "test-chat",
		remoteDiffStats: makePrStatus(),
		repositories: new Map([["/home/coder/coder", makeRepo()]]),
	},
	beforeEach: () => {
		spyOn(API.experimental, "getChatDiffContents").mockResolvedValue({
			...mockDiffContents,
			diff: sampleDiff,
		});
	},
};

/**
 * Two tracked PRs on different branches, switching between them via
 * the view switcher.
 */
export const MultiplePullRequests: Story = {
	args: {
		chatId: "test-chat",
		remoteDiffStats: [
			...makePrStatus({
				pull_request_title: "feat: first change",
				head_branch: "feat/first",
				git_branch: "feat/first",
				pr_number: 23020,
			}),
			...makePrStatus({
				pull_request_title: "fix: second change",
				head_branch: "fix/second",
				git_branch: "fix/second",
				pr_number: 23021,
				url: "https://github.com/coder/coder/pull/23021",
				pull_request_state: "merged",
				additions: 12,
				deletions: 3,
				changed_files: 2,
			}),
		],
	},
	beforeEach: () => {
		spyOn(API.experimental, "getChatDiffContents").mockResolvedValue({
			...mockDiffContents,
			diff: sampleDiff,
		});
	},
	play: async ({ canvasElement }) => {
		// Open the switcher and select the second PR so the screenshot
		// shows the merged state. Behavior is asserted in Vitest.
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByTestId("git-panel-view-switcher"));
		const menu = await within(document.body).findByRole("menu");
		await userEvent.click(within(menu).getByText("PR #23021"));
	},
};

/**
 * Opens the dropdown, asserts the PR + working repos appear, then
 * clicks a working entry to verify the view swap.
 */
export const ViewSwitcherOpen: Story = {
	args: {
		chatId: "test-chat",
		remoteDiffStats: makePrStatus({
			pull_request_title: "feat: multi-repo workspace support",
			head_branch: "feat/multi-repo",
		}),
		repositories: new Map([
			["/home/coder/coder", makeRepo()],
			[
				"/home/coder/other-project",
				makeRepo({
					repo_root: "/home/coder/other-project",
					branch: "main",
					remote_origin: "https://github.com/coder/other-project.git",
					unified_diff: secondRepoDiff,
				}),
			],
		]),
	},
	beforeEach: () => {
		spyOn(API.experimental, "getChatDiffContents").mockResolvedValue({
			...mockDiffContents,
			diff: sampleDiff,
		});
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const switcher = canvas.getByTestId("git-panel-view-switcher");
		await userEvent.click(switcher);

		const menu = await within(document.body).findByRole("menu");

		// Selecting a menu item swaps the active view and the trigger
		// identifier reflects the new selection.
		const otherProjectItem = within(menu).getByText("other-project");
		await userEvent.click(otherProjectItem);
	},
};

/** Draft PR with head/base branches. */
export const DraftPullRequest: Story = {
	args: {
		chatId: "test-chat",
		remoteDiffStats: makePrStatus({
			url: "https://github.com/coder/coder/pull/22950",
			pull_request_title: "fix: resolve race condition in workspace builds",
			pull_request_draft: true,
			head_branch: "fix/race-condition",
			additions: 142,
			deletions: 38,
			changed_files: 5,
		}),
		repositories: new Map([
			["/home/coder/coder", makeRepo({ branch: "fix/race-condition" })],
		]),
	},
	beforeEach: () => {
		spyOn(API.experimental, "getChatDiffContents").mockResolvedValue({
			...mockDiffContents,
			diff: sampleDiff,
		});
	},
};

/** Merged PR. */
export const MergedPullRequest: Story = {
	args: {
		chatId: "test-chat",
		remoteDiffStats: makePrStatus({
			url: "https://github.com/coder/coder/pull/23000",
			pull_request_title: "chore: update dependencies to latest",
			pull_request_state: "merged",
			head_branch: "chore/update-deps",
			additions: 89,
			deletions: 45,
			changed_files: 3,
		}),
	},
	beforeEach: () => {
		spyOn(API.experimental, "getChatDiffContents").mockResolvedValue({
			...mockDiffContents,
			diff: sampleDiff,
		});
	},
};

/** Closed PR. */
export const ClosedPullRequest: Story = {
	args: {
		chatId: "test-chat",
		remoteDiffStats: makePrStatus({
			url: "https://github.com/coder/coder/pull/22800",
			pull_request_title: "feat: experimental websocket transport",
			pull_request_state: "closed",
			head_branch: "feat/websocket-transport",
			additions: 200,
			deletions: 10,
			changed_files: 4,
		}),
	},
	beforeEach: () => {
		spyOn(API.experimental, "getChatDiffContents").mockResolvedValue({
			...mockDiffContents,
			diff: sampleDiff,
		});
	},
};

/** Branch pushed but no PR opened yet. */
export const BranchOnly: Story = {
	args: {
		chatId: "test-chat",
		remoteDiffStats: [
			{
				...MockChatDiffStatus,
				chat_id: "test-chat",
				git_branch: "feat/branch-only",
				head_branch: "feat/branch-only",
				url: "https://github.com/coder/coder/tree/feat/branch-only",
				pr_number: undefined,
				pull_request_state: undefined,
				pull_request_title: "",
				additions: 42,
				deletions: 7,
				changed_files: 3,
			},
		],
		repositories: new Map([["/home/coder/coder", makeRepo()]]),
	},
};

/**
 * A branch-only primary with an older PR: selecting the PR makes the
 * title row follow the selection instead of the branch-only primary.
 */
export const BranchPrimarySelectedPr: Story = {
	args: {
		chatId: "test-chat",
		remoteDiffStats: [
			{
				...MockChatDiffStatus,
				chat_id: "test-chat",
				git_branch: "feat/branch-only",
				head_branch: "feat/branch-only",
				url: "https://github.com/coder/coder/tree/feat/branch-only",
				pr_number: undefined,
				pull_request_state: undefined,
				pull_request_title: "",
				additions: 42,
				deletions: 7,
				changed_files: 3,
			},
			...makePrStatus({
				pull_request_title: "fix: second change",
				head_branch: "fix/second",
				git_branch: "fix/second",
				pr_number: 23021,
				url: "https://github.com/coder/coder/pull/23021",
			}),
		],
	},
	beforeEach: () => {
		spyOn(API.experimental, "getChatDiffContents").mockResolvedValue({
			...mockDiffContents,
			diff: sampleDiff,
		});
	},
	play: async ({ canvasElement }) => {
		// Open the switcher and select the older PR so the screenshot
		// shows its title row. Behavior is asserted in Vitest.
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByTestId("git-panel-view-switcher"));
		const menu = await within(document.body).findByRole("menu");
		await userEvent.click(within(menu).getByText("PR #23021"));
	},
};

/** Only local working changes, no remote/PR. */
export const WorkingChangesOnly: Story = {
	args: {
		chatId: "test-chat",
		repositories: new Map([["/home/coder/coder", makeRepo()]]),
	},
};

/** Multiple repos with working changes. */
export const MultipleRepos: Story = {
	args: {
		chatId: "test-chat",
		remoteDiffStats: makePrStatus({
			pull_request_title: "feat: multi-repo workspace support",
			head_branch: "feat/multi-repo",
			additions: 500,
			deletions: 120,
			changed_files: 8,
		}),
		repositories: new Map([
			["/home/coder/coder", makeRepo()],
			[
				"/home/coder/other-project",
				makeRepo({
					repo_root: "/home/coder/other-project",
					branch: "main",
					remote_origin: "https://github.com/coder/other-project.git",
					unified_diff: secondRepoDiff,
				}),
			],
		]),
	},
	beforeEach: () => {
		spyOn(API.experimental, "getChatDiffContents").mockResolvedValue({
			...mockDiffContents,
			diff: sampleDiff,
		});
	},
};

/** No remote changes and no working changes. */
export const EmptyState: Story = {
	args: {
		chatId: "test-chat",
	},
};

/** No repositories and no remote tab; Git controls should be disabled. */
export const GitNotActive: Story = {
	args: {
		chatId: "test-chat",
		repositories: new Map(),
	},
};

/** Git watcher is loading its first repository update. */
export const GitStatusLoading: Story = {
	args: {
		chatId: "test-chat",
		repositories: new Map(),
		isGitStatusLoading: true,
	},
};

/**
 * PR diff with the inline comment input visible. The play function
 * waits for the diff to render, then clicks a line number gutter
 * to trigger the annotation input.
 */
export const InlineCommentInput: Story = {
	args: {
		chatId: "test-chat",
		remoteDiffStats: makePrStatus(),
	},
	decorators: [
		(Story) => (
			<div style={{ height: 700, width: 600 }}>
				<Story />
			</div>
		),
	],
	beforeEach: () => {
		spyOn(API.experimental, "getChatDiffContents").mockResolvedValue({
			...mockDiffContents,
			diff: sampleDiff,
		});
	},
	play: async ({ canvasElement }) => {
		const lineNumber = await waitFor(() => {
			for (const host of canvasElement.querySelectorAll("diffs-container")) {
				const target = host.shadowRoot?.querySelector(
					"[data-column-number]",
				) as HTMLElement | null;
				if (target) return target;
			}
			throw new Error("No rendered diff line number found");
		});

		await userEvent.click(lineNumber);
	},
};

export const LargeDiff: Story = {
	args: {
		chatId: "test-chat",
		repositories: new Map([
			[
				"/home/coder/large-project",
				makeRepo({
					repo_root: "/home/coder/large-project",
					branch: "feat/large-refactor",
					remote_origin: "https://github.com/coder/large-project.git",
					unified_diff: generateLargeDiff(40, 60),
				}),
			],
		]),
	},
};

/**
 * Regression: a repo that was dirty earlier in the session must
 * keep its switcher entry even after its unified_diff empties.
 */
export const EverDirtyRepoGoneClean: Story = {
	args: {
		chatId: "test-chat",
		repositories: new Map([
			["/home/coder/coder", makeRepo({ unified_diff: "" })],
		]),
		everDirty: new Set(["/home/coder/coder"]),
	},
};

/**
 * Baseline: a repo reported clean from the start (never dirty in
 * this session) has no switcher entry. Ensures the ever-dirty fix
 * did not regress the "nothing to show" case.
 */
export const CleanRepoFromStart: Story = {
	args: {
		chatId: "test-chat",
		repositories: new Map([
			["/home/coder/coder", makeRepo({ unified_diff: "" })],
		]),
		everDirty: new Set(),
	},
};
