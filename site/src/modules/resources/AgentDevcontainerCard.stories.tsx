import type { Meta, StoryObj } from "@storybook/react";
import { getPreferredProxy } from "contexts/ProxyContext";
import { chromatic } from "testHelpers/chromatic";
import {
	MockListeningPortsResponse,
	MockPrimaryWorkspaceProxy,
	MockTemplate,
	MockWorkspace,
	MockWorkspaceAgent,
	MockWorkspaceAgentContainer,
	MockWorkspaceAgentContainerPorts,
	MockWorkspaceAgentDevcontainer,
	MockWorkspaceApp,
	MockWorkspaceProxies,
	MockWorkspaceSubAgent,
} from "testHelpers/entities";
import {
	withDashboardProvider,
	withProxyProvider,
} from "testHelpers/storybook";
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
