import type { Meta, StoryObj } from "@storybook/react-vite";
import { API } from "api/api";
import type {
	ChatDiffContents,
	ChatDiffStatus,
	WorkspaceAgentRepoChanges,
} from "api/typesGenerated";
import { fn, spyOn } from "storybook/test";
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

const defaultDiffStatus: ChatDiffStatus = {
	chat_id: "test-chat",
	pull_request_title: "",
	pull_request_draft: false,
	changes_requested: false,
	additions: 0,
	deletions: 0,
	changed_files: 0,
};

const defaultDiffContents: ChatDiffContents = {
	chat_id: "test-chat",
};

// ---------------------------------------------------------------------------
// Meta
// ---------------------------------------------------------------------------

const meta: Meta<typeof GitPanel> = {
	title: "pages/AgentsPage/GitPanel",
	component: GitPanel,
	args: {
		onRefresh: fn(),
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
		spyOn(API, "getChatDiffStatus").mockResolvedValue(defaultDiffStatus);
		spyOn(API, "getChatDiffContents").mockResolvedValue(defaultDiffContents);
	},
};

export default meta;
type Story = StoryObj<typeof GitPanel>;

// ---------------------------------------------------------------------------
// Stories
// ---------------------------------------------------------------------------

/** PR is open with a title, working changes in one repo. */
export const PullRequestAndWorkingChanges: Story = {
	args: {
		prTab: { prNumber: 23020, chatId: "test-chat" },
		remoteDiffStats: {
			...defaultDiffStatus,
			url: "https://github.com/coder/coder/pull/23020",
			pull_request_title:
				"feat(agents): add MCP server configuration to agents",
			pull_request_state: "open",
			pull_request_draft: false,
			base_branch: "main",
			additions: 4037,
			deletions: 7,
			changed_files: 12,
		},
		repositories: new Map([["/home/coder/coder", makeRepo()]]),
	},
	beforeEach: () => {
		spyOn(API, "getChatDiffStatus").mockResolvedValue({
			...defaultDiffStatus,
			url: "https://github.com/coder/coder/pull/23020",
			pull_request_title:
				"feat(agents): add MCP server configuration to agents",
			pull_request_state: "open",
			pull_request_draft: false,
			base_branch: "main",
			additions: 4037,
			deletions: 7,
			changed_files: 12,
		});
		spyOn(API, "getChatDiffContents").mockResolvedValue({
			...defaultDiffContents,
			diff: sampleDiff,
		});
	},
};

/** Draft PR with working changes. */
export const DraftPullRequest: Story = {
	args: {
		prTab: { prNumber: 22950, chatId: "test-chat" },
		remoteDiffStats: {
			...defaultDiffStatus,
			url: "https://github.com/coder/coder/pull/22950",
			pull_request_title: "fix: resolve race condition in workspace builds",
			pull_request_state: "open",
			pull_request_draft: true,
			base_branch: "main",
			additions: 142,
			deletions: 38,
			changed_files: 5,
		},
		repositories: new Map([
			["/home/coder/coder", makeRepo({ branch: "fix/race-condition" })],
		]),
	},
	beforeEach: () => {
		spyOn(API, "getChatDiffStatus").mockResolvedValue({
			...defaultDiffStatus,
			url: "https://github.com/coder/coder/pull/22950",
			pull_request_title: "fix: resolve race condition in workspace builds",
			pull_request_state: "open",
			pull_request_draft: true,
			base_branch: "main",
			additions: 142,
			deletions: 38,
			changed_files: 5,
		});
		spyOn(API, "getChatDiffContents").mockResolvedValue({
			...defaultDiffContents,
			diff: sampleDiff,
		});
	},
};

/** Merged PR, no working changes. */
export const MergedPullRequest: Story = {
	args: {
		prTab: { prNumber: 23000, chatId: "test-chat" },
		remoteDiffStats: {
			...defaultDiffStatus,
			url: "https://github.com/coder/coder/pull/23000",
			pull_request_title: "chore: update dependencies to latest",
			pull_request_state: "merged",
			base_branch: "main",
			additions: 89,
			deletions: 45,
			changed_files: 3,
		},
	},
	beforeEach: () => {
		spyOn(API, "getChatDiffStatus").mockResolvedValue({
			...defaultDiffStatus,
			url: "https://github.com/coder/coder/pull/23000",
			pull_request_title: "chore: update dependencies to latest",
			pull_request_state: "merged",
			base_branch: "main",
			additions: 89,
			deletions: 45,
			changed_files: 3,
		});
		spyOn(API, "getChatDiffContents").mockResolvedValue({
			...defaultDiffContents,
			diff: sampleDiff,
		});
	},
};

/** Closed PR. */
export const ClosedPullRequest: Story = {
	args: {
		prTab: { prNumber: 22800, chatId: "test-chat" },
		remoteDiffStats: {
			...defaultDiffStatus,
			url: "https://github.com/coder/coder/pull/22800",
			pull_request_title: "feat: experimental websocket transport",
			pull_request_state: "closed",
			base_branch: "main",
			additions: 200,
			deletions: 10,
			changed_files: 4,
		},
	},
	beforeEach: () => {
		spyOn(API, "getChatDiffStatus").mockResolvedValue({
			...defaultDiffStatus,
			url: "https://github.com/coder/coder/pull/22800",
			pull_request_title: "feat: experimental websocket transport",
			pull_request_state: "closed",
			base_branch: "main",
			additions: 200,
			deletions: 10,
			changed_files: 4,
		});
		spyOn(API, "getChatDiffContents").mockResolvedValue({
			...defaultDiffContents,
			diff: sampleDiff,
		});
	},
};

/** Branch pushed but no PR opened yet. */
export const BranchOnly: Story = {
	args: {
		remoteDiffStats: {
			...defaultDiffStatus,
			additions: 42,
			deletions: 7,
			changed_files: 3,
		},
		repositories: new Map([["/home/coder/coder", makeRepo()]]),
	},
};

/** Only local working changes, no remote/PR. */
export const WorkingChangesOnly: Story = {
	args: {
		repositories: new Map([["/home/coder/coder", makeRepo()]]),
	},
};

/** Multiple repos with working changes. */
export const MultipleRepos: Story = {
	args: {
		prTab: { prNumber: 23020, chatId: "test-chat" },
		remoteDiffStats: {
			...defaultDiffStatus,
			url: "https://github.com/coder/coder/pull/23020",
			pull_request_title: "feat: multi-repo workspace support",
			pull_request_state: "open",
			base_branch: "main",
			additions: 500,
			deletions: 120,
			changed_files: 8,
		},
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
		spyOn(API, "getChatDiffStatus").mockResolvedValue({
			...defaultDiffStatus,
			url: "https://github.com/coder/coder/pull/23020",
			pull_request_title: "feat: multi-repo workspace support",
			pull_request_state: "open",
			base_branch: "main",
			additions: 500,
			deletions: 120,
			changed_files: 8,
		});
		spyOn(API, "getChatDiffContents").mockResolvedValue({
			...defaultDiffContents,
			diff: sampleDiff,
		});
	},
};

/** No remote changes, no working changes — empty state. */
export const EmptyState: Story = {
	args: {
		prTab: { prNumber: 23020, chatId: "test-chat" },
	},
};
