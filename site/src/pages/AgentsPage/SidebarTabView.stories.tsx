import type { Meta, StoryObj } from "@storybook/react-vite";
import type { WorkspaceAgentRepoChanges } from "api/typesGenerated";
import { fn } from "storybook/test";
import { SidebarTabView } from "./SidebarTabView";

const sampleDiff = `--- a/src/index.ts
+++ b/src/index.ts
@@ -1,3 +1,5 @@
+import { init } from "./init";
+
 const main = () => {
   console.log("hello");
 };
`;

const makeRepo = (
	name: string,
	overrides?: Partial<WorkspaceAgentRepoChanges>,
): WorkspaceAgentRepoChanges => ({
	repo_root: `/home/coder/${name}`,
	branch: "main",
	remote_origin: `https://github.com/coder/${name}.git`,
	unified_diff: sampleDiff,
	...overrides,
});

const meta: Meta<typeof SidebarTabView> = {
	title: "pages/AgentsPage/SidebarTabView",
	component: SidebarTabView,
	args: {
		workspace: { name: "my-workspace", ownerName: "admin" },
		onRefresh: fn(),
		onCommit: fn(),
		isExpanded: false,
		onToggleExpanded: fn(),
	},
	decorators: [
		(Story) => (
			<div style={{ height: 500, width: 480 }}>
				<Story />
			</div>
		),
	],
};
export default meta;
type Story = StoryObj<typeof SidebarTabView>;

export const PROnly: Story = {
	args: {
		prTab: { prNumber: 42, chatId: "chat-1" },
		repositories: new Map(),
	},
};

export const SingleRepo: Story = {
	args: {
		prTab: undefined,
		repositories: new Map([["/home/coder/project", makeRepo("project")]]),
	},
};

export const PRAndRepos: Story = {
	args: {
		prTab: { prNumber: 123, chatId: "chat-2" },
		repositories: new Map([
			["/home/coder/frontend", makeRepo("frontend")],
			[
				"/home/coder/backend",
				makeRepo("backend", {
					branch: "feat/api",
				}),
			],
		]),
	},
};

export const ManyRepos: Story = {
	args: {
		prTab: undefined,
		repositories: new Map(
			["alpha", "bravo", "charlie", "delta", "echo"].map((name) => [
				`/home/coder/${name}`,
				makeRepo(name),
			]),
		),
	},
};

export const EmptyState: Story = {
	args: {
		prTab: undefined,
		repositories: new Map(),
	},
};

export const SplitDiffMode: Story = {
	args: {
		prTab: undefined,
		repositories: new Map([["/home/coder/project", makeRepo("project")]]),
	},
};

export const ExpandedWithDiffToggle: Story = {
	args: {
		prTab: { prNumber: 42, chatId: "chat-1" },
		repositories: new Map([["/home/coder/project", makeRepo("project")]]),
		isExpanded: true,
		chatTitle: "Fix authentication bug",
	},
	decorators: [
		(Story) => (
			<div style={{ height: 600, width: 900 }}>
				<Story />
			</div>
		),
	],
};

export const WithDiffStats: Story = {
	args: {
		prTab: { prNumber: 99, chatId: "chat-3" },
		repositories: new Map([
			["/home/coder/frontend", makeRepo("frontend")],
			["/home/coder/backend", makeRepo("backend", { branch: "feat/api" })],
		]),
		diffStatus: { additions: 150, deletions: 42 },
	},
};

export const NarrowPanel: Story = {
	args: {
		prTab: { prNumber: 42, chatId: "chat-1" },
		repositories: new Map([["/home/coder/project", makeRepo("project")]]),
	},
	decorators: [
		(Story) => (
			<div style={{ height: 500, width: 360 }}>
				<Story />
			</div>
		),
	],
};
