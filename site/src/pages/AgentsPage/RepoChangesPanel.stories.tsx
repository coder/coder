import type { Meta, StoryObj } from "@storybook/react-vite";
import type { WorkspaceAgentRepoChanges } from "api/typesGenerated";
import { fn } from "storybook/test";
import { RepoChangesPanel } from "./RepoChangesPanel";

const sampleDiff = `--- a/src/main.ts
+++ b/src/main.ts
@@ -1,5 +1,7 @@
 import { start } from "./server";
+import { logger } from "./logger";

 const port = 3000;
+logger.info("Starting server...");
 start(port);
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

const baseRepo: WorkspaceAgentRepoChanges = {
	repo_root: "/home/coder/project",
	branch: "feat/add-logging",
	remote_origin: "https://github.com/coder/project.git",
	unified_diff: sampleDiff,
};

const meta: Meta<typeof RepoChangesPanel> = {
	title: "pages/AgentsPage/RepoChangesPanel",
	component: RepoChangesPanel,
	args: {
		repo: baseRepo,
		onRefresh: fn(),
		onCommit: fn(),
		diffStyle: "unified",
	},
};
export default meta;
type Story = StoryObj<typeof RepoChangesPanel>;

export const WithChanges: Story = {};

export const NoChanges: Story = {
	args: {
		repo: {
			...baseRepo,
			unified_diff: undefined,
		},
	},
};

export const SplitDiffStyle: Story = {
	args: {
		repo: baseRepo,
		diffStyle: "split",
	},
};

export const LongBranchName: Story = {
	args: {
		repo: {
			...baseRepo,
			branch:
				"feature/TICKET-12345-implement-very-long-branch-name-for-testing-truncation-behavior",
		},
	},
	decorators: [
		(Story) => (
			<div style={{ width: 400 }}>
				<Story />
			</div>
		),
	],
};

export const DeepRepoPath: Story = {
	args: {
		repo: {
			...baseRepo,
			repo_root: "/home/coder/workspaces/my-org/services/project",
		},
	},
};

export const EmptyBranchName: Story = {
	args: {
		repo: {
			...baseRepo,
			branch: "",
		},
	},
};

export const ManyFiles: Story = {};

export const UntrackedFiles: Story = {};
