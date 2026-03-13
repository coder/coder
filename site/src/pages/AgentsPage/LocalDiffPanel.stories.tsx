import type { Meta, StoryObj } from "@storybook/react-vite";
import type { WorkspaceAgentRepoChanges } from "api/typesGenerated";
import { LocalDiffPanel } from "./LocalDiffPanel";

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

const meta: Meta<typeof LocalDiffPanel> = {
	title: "pages/AgentsPage/LocalDiffPanel",
	component: LocalDiffPanel,
	args: {
		repo: baseRepo,
		diffStyle: "unified",
	},
};
export default meta;
type Story = StoryObj<typeof LocalDiffPanel>;

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
