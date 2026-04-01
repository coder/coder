import { MockWorkspace } from "testHelpers/entities";
import type { Meta, StoryObj } from "@storybook/react-vite";
import type { Workspace, WorkspaceStatus } from "api/typesGenerated";
import { WorkspaceStatusIndicator } from "./WorkspaceStatusIndicator";

const meta: Meta<typeof WorkspaceStatusIndicator> = {
	title: "modules/workspaces/WorkspaceStatusIndicator",
	component: WorkspaceStatusIndicator,
};

export default meta;
type Story = StoryObj<typeof WorkspaceStatusIndicator>;

const createWorkspaceWithStatus = (status: WorkspaceStatus): Workspace => {
	return {
		...MockWorkspace,
		latest_build: {
			...MockWorkspace.latest_build,
			status,
		},
	} as Workspace;
};

export const Running: Story = {
	args: {
		workspace: createWorkspaceWithStatus("running"),
	},
};

export const Unhealthy: Story = {
	args: {
		workspace: {
			...createWorkspaceWithStatus("running"),
			health: {
				healthy: false,
				failing_agents: [],
			},
		},
	},
};

export const Stopped: Story = {
	args: {
		workspace: createWorkspaceWithStatus("stopped"),
	},
};

export const Starting: Story = {
	args: {
		workspace: createWorkspaceWithStatus("starting"),
	},
};

export const Stopping: Story = {
	args: {
		workspace: createWorkspaceWithStatus("stopping"),
	},
};

export const Failed: Story = {
	args: {
		workspace: createWorkspaceWithStatus("failed"),
	},
};

export const Canceling: Story = {
	args: {
		workspace: createWorkspaceWithStatus("canceling"),
	},
};

export const Canceled: Story = {
	args: {
		workspace: createWorkspaceWithStatus("canceled"),
	},
};

export const Deleting: Story = {
	args: {
		workspace: createWorkspaceWithStatus("deleting"),
	},
};

export const Deleted: Story = {
	args: {
		workspace: createWorkspaceWithStatus("deleted"),
	},
};

export const Pending: Story = {
	args: {
		workspace: createWorkspaceWithStatus("pending"),
	},
};
