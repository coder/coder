import { MockWorkspaceAppStatus } from "testHelpers/entities";
import { withProxyProvider } from "testHelpers/storybook";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { WorkspaceAppStatus } from "./WorkspaceAppStatus";

const meta: Meta<typeof WorkspaceAppStatus> = {
	title: "modules/workspaces/WorkspaceAppStatus",
	component: WorkspaceAppStatus,
	decorators: [withProxyProvider()],
};

export default meta;
type Story = StoryObj<typeof WorkspaceAppStatus>;

export const Complete: Story = {
	args: {
		status: MockWorkspaceAppStatus,
	},
};

export const Failure: Story = {
	args: {
		status: {
			...MockWorkspaceAppStatus,
			state: "failure",
			message: "Couldn't figure out how to start the dev server",
		},
	},
};

export const Working: Story = {
	args: {
		status: {
			...MockWorkspaceAppStatus,
			state: "working",
			message: "Starting dev server...",
			uri: "",
		},
	},
};

export const Idle: Story = {
	args: {
		status: {
			...MockWorkspaceAppStatus,
			state: "idle",
			message: "Done for now",
		},
	},
};

export const NoMessage: Story = {
	args: {
		status: {
			...MockWorkspaceAppStatus,
			state: "idle",
			message: "",
		},
	},
};

export const LongMessage: Story = {
	args: {
		status: {
			...MockWorkspaceAppStatus,
			message:
				"This is a long message that will wrap around the component. It should wrap many times because this is very very very very very long.",
		},
	},
};

export const DisabledComplete: Story = {
	args: {
		status: MockWorkspaceAppStatus,
		disabled: true,
	},
};

export const DisabledFailure: Story = {
	args: {
		status: {
			...MockWorkspaceAppStatus,
			state: "failure",
			message: "Couldn't figure out how to start the dev server",
		},
		disabled: true,
	},
};

export const DisabledWorking: Story = {
	args: {
		status: {
			...MockWorkspaceAppStatus,
			state: "working",
			message: "Starting dev server...",
			uri: "",
		},
		disabled: true,
	},
};

export const DisabledIdle: Story = {
	args: {
		status: {
			...MockWorkspaceAppStatus,
			state: "idle",
			message: "Done for now",
		},
		disabled: true,
	},
};
