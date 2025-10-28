import { chromatic } from "testHelpers/chromatic";
import {
	MockListeningPortsResponse,
	MockPrimaryWorkspaceProxy,
	MockTask,
	MockTemplate,
	MockWorkspace,
	MockWorkspaceAgent,
	MockWorkspaceAgentContainer,
	MockWorkspaceAgentContainerPorts,
	MockWorkspaceAgentDevcontainer,
	MockWorkspaceApp,
	MockWorkspaceAppStatus,
	MockWorkspaceProxies,
	MockWorkspaceSubAgent,
} from "testHelpers/entities";
import {
	withDashboardProvider,
	withProxyProvider,
} from "testHelpers/storybook";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { getPreferredProxy } from "contexts/ProxyContext";
import { expect, within } from "storybook/test";
import { AgentDevcontainerCard } from "./AgentDevcontainerCard";

const meta: Meta<typeof AgentDevcontainerCard> = {
	title: "modules/resources/AgentDevcontainerCard",
	component: AgentDevcontainerCard,
	args: {
		devcontainer: MockWorkspaceAgentDevcontainer,
		workspace: MockWorkspace,
		wildcardHostname: "*.wildcard.hostname",
		parentAgent: MockWorkspaceAgent,
		template: MockTemplate,
		subAgents: [MockWorkspaceSubAgent],
	},
	decorators: [withProxyProvider(), withDashboardProvider],
	parameters: {
		chromatic,
		queries: [
			{
				key: ["portForward", MockWorkspaceSubAgent.id],
				data: MockListeningPortsResponse,
			},
		],
	},
};

export default meta;
type Story = StoryObj<typeof AgentDevcontainerCard>;

export const HasError: Story = {
	args: {
		devcontainer: {
			...MockWorkspaceAgentDevcontainer,
			error: "unable to inject devcontainer with agent",
			agent: undefined,
		},
	},
};

export const NoPorts: Story = {};

export const WithPorts: Story = {
	args: {
		devcontainer: {
			...MockWorkspaceAgentDevcontainer,
			container: {
				...MockWorkspaceAgentContainer,
				ports: MockWorkspaceAgentContainerPorts,
			},
		},
	},
};

export const Dirty: Story = {
	args: {
		devcontainer: {
			...MockWorkspaceAgentDevcontainer,
			dirty: true,
		},
	},
};

export const Recreating: Story = {
	args: {
		devcontainer: {
			...MockWorkspaceAgentDevcontainer,
			dirty: true,
			status: "starting",
			container: undefined,
		},
		subAgents: [],
	},
};

export const NoContainerOrSubAgent: Story = {
	args: {
		devcontainer: {
			...MockWorkspaceAgentDevcontainer,
			container: undefined,
			agent: undefined,
		},
		subAgents: [],
	},
};

export const NoContainerOrAgentOrName: Story = {
	args: {
		devcontainer: {
			...MockWorkspaceAgentDevcontainer,
			container: undefined,
			agent: undefined,
			name: "",
		},
		subAgents: [],
	},
};

export const NoSubAgent: Story = {
	args: {
		devcontainer: {
			...MockWorkspaceAgentDevcontainer,
			agent: undefined,
		},
		subAgents: [],
	},
};

export const SubAgentConnecting: Story = {
	args: {
		subAgents: [
			{
				...MockWorkspaceSubAgent,
				status: "connecting",
			},
		],
	},
};

export const WithAppsAndPorts: Story = {
	args: {
		devcontainer: {
			...MockWorkspaceAgentDevcontainer,
			container: {
				...MockWorkspaceAgentContainer,
				ports: MockWorkspaceAgentContainerPorts,
			},
		},
		subAgents: [
			{
				...MockWorkspaceSubAgent,
				apps: [MockWorkspaceApp],
			},
		],
	},
};

export const WithPortForwarding: Story = {
	decorators: [
		withProxyProvider({
			proxy: getPreferredProxy(MockWorkspaceProxies, MockPrimaryWorkspaceProxy),
			proxies: MockWorkspaceProxies,
		}),
	],
};

export const WithTask: Story = {
	args: {
		task: MockTask,
		workspace: {
			...MockWorkspace,
			latest_app_status: {
				...MockWorkspaceAppStatus,
				agent_id: MockWorkspaceSubAgent.id,
				message: "Task is running",
				state: "working",
			},
		},
		subAgents: [
			{
				...MockWorkspaceSubAgent,
				apps: [MockWorkspaceApp],
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const taskLink = canvas.getByRole("link", { name: /view task/i });
		await expect(taskLink).toHaveAttribute(
			"href",
			`/tasks/${MockWorkspace.owner_name}/${MockTask.id}`,
		);
	},
};
