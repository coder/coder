import { chromatic } from "testHelpers/chromatic";
import { MockWorkspace, MockWorkspaceAgent } from "testHelpers/entities";
import { withDashboardProvider } from "testHelpers/storybook";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { AgentExternal } from "./AgentExternal";

const meta: Meta<typeof AgentExternal> = {
	title: "modules/resources/AgentExternal",
	component: AgentExternal,
	args: {
		agent: {
			...MockWorkspaceAgent,
			status: "connecting",
			operating_system: "linux",
			architecture: "amd64",
		},
		workspace: MockWorkspace,
	},
	decorators: [withDashboardProvider],
	parameters: {
		chromatic,
	},
};

export default meta;
type Story = StoryObj<typeof AgentExternal>;

export const Connecting: Story = {
	args: {
		agent: {
			...MockWorkspaceAgent,
			status: "connecting",
			operating_system: "linux",
			architecture: "amd64",
		},
	},
};

export const Timeout: Story = {
	args: {
		agent: {
			...MockWorkspaceAgent,
			status: "timeout",
			operating_system: "linux",
			architecture: "amd64",
		},
	},
};

export const DifferentOS: Story = {
	args: {
		agent: {
			...MockWorkspaceAgent,
			status: "connecting",
			operating_system: "darwin",
			architecture: "arm64",
		},
	},
};

export const NotExternalAgent: Story = {
	args: {
		agent: {
			...MockWorkspaceAgent,
			status: "connecting",
			operating_system: "linux",
			architecture: "amd64",
		},
	},
};
