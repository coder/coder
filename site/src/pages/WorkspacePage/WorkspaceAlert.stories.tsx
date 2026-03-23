import {
	MockWorkspace,
	MockWorkspaceAgent,
	MockWorkspaceResource,
} from "testHelpers/entities";
import type { Meta, StoryObj } from "@storybook/react-vite";
import type { Workspace, WorkspaceAgent } from "api/typesGenerated";
import { WorkspaceAlert } from "./WorkspaceAlert";

const createUnhealthyWorkspace = (
	agentOverrides: Partial<WorkspaceAgent>,
	agentCount = 1,
): Workspace => {
	const agents = Array.from({ length: agentCount }, (_, i) => ({
		...MockWorkspaceAgent,
		id: `test-agent-${i}`,
		name: `agent-${i}`,
		health: { healthy: false },
		...agentOverrides,
	}));
	return {
		...MockWorkspace,
		health: {
			healthy: false,
			failing_agents: agents.map((a) => a.id),
		},
		latest_build: {
			...MockWorkspace.latest_build,
			resources: [{ ...MockWorkspaceResource, agents }],
		},
	};
};

const meta: Meta<typeof WorkspaceAlert> = {
	title: "pages/WorkspacePage/WorkspaceAlert",
	component: WorkspaceAlert,
};

export default meta;
type Story = StoryObj<typeof WorkspaceAlert>;

export const Disconnected: Story = {
	args: {
		workspace: createUnhealthyWorkspace({ status: "disconnected" }),
		troubleshootingURL: "https://coder.com/docs/troubleshoot",
	},
};

export const DisconnectedMultipleAgents: Story = {
	args: {
		workspace: createUnhealthyWorkspace({ status: "disconnected" }, 3),
		troubleshootingURL: "https://coder.com/docs/troubleshoot",
	},
};

export const TimeoutWarning: Story = {
	args: {
		workspace: createUnhealthyWorkspace({ status: "timeout" }),
		troubleshootingURL: "https://coder.com/docs/troubleshoot",
	},
};

export const StartupScriptFailed: Story = {
	args: {
		workspace: createUnhealthyWorkspace({
			status: "connected",
			lifecycle_state: "start_error",
		}),
		troubleshootingURL: "https://coder.com/docs/troubleshoot",
	},
};

export const StartupScriptFailedMultipleAgents: Story = {
	args: {
		workspace: createUnhealthyWorkspace(
			{
				status: "connected",
				lifecycle_state: "start_error",
			},
			2,
		),
		troubleshootingURL: "https://coder.com/docs/troubleshoot",
	},
};

export const ShuttingDownInformational: Story = {
	args: {
		workspace: createUnhealthyWorkspace({
			status: "connected",
			lifecycle_state: "shutting_down",
		}),
	},
};

export const NotConnected: Story = {
	args: {
		workspace: createUnhealthyWorkspace({ status: "connecting" }),
	},
};

export const WithoutTroubleshootingURL: Story = {
	args: {
		workspace: createUnhealthyWorkspace({ status: "disconnected" }),
		troubleshootingURL: undefined,
	},
};
