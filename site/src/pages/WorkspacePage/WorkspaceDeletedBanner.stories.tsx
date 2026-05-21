import type { Meta, StoryObj } from "@storybook/react-vite";
import { WorkspaceDeletedBanner } from "./WorkspaceDeletedBanner";

const meta: Meta<typeof WorkspaceDeletedBanner> = {
	title: "pages/WorkspacePage/WorkspaceDeletedBanner",
	component: WorkspaceDeletedBanner,
	args: {
		createWorkspaceLink: "/templates/test-template/workspace",
		templateName: "Test Template",
	},
};

export default meta;
type Story = StoryObj<typeof WorkspaceDeletedBanner>;

const Example: Story = {};

export { Example as WorkspaceDeletedBanner };
