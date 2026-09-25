import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { MockChatProject } from "#/testHelpers/entities";
import { ProjectFolders } from "./ProjectFolders";

const meta: Meta<typeof ProjectFolders> = {
	title: "pages/AgentsPage/ProjectFolders",
	component: ProjectFolders,
	args: {
		projects: [MockChatProject],
		chatsByProjectId: new Map(),
		expandedProjectIds: { [MockChatProject.id]: true },
		onToggle: fn(),
		onCreate: fn(),
		onEdit: fn(),
		onDelete: fn(),
		onRetry: fn(),
		isFiltered: false,
	},
};

export default meta;
type Story = StoryObj<typeof ProjectFolders>;

export const EmptyProject: Story = {};

export const EmptyProjectWithFilters: Story = {
	args: {
		isFiltered: true,
	},
};
