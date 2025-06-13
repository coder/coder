import type { Meta, StoryObj } from "@storybook/react";
import type { WorkspaceAgentDevcontainer } from "api/typesGenerated";
import {
	MockTemplate,
	MockWorkspace,
	MockWorkspaceAgent,
	MockWorkspaceAgentContainer,
	MockWorkspaceAgentContainerPorts,
	MockWorkspaceApp,
	MockWorkspaceSubAgent,
} from "testHelpers/entities";
import { AgentDevcontainerCard } from "./AgentDevcontainerCard";

const MockWorkspaceAgentDevcontainer: WorkspaceAgentDevcontainer = {
	id: "test-devcontainer-id",
	name: "test-devcontainer",
	workspace_folder: "/workspace/test",
	config_path: "/workspace/test/.devcontainer/devcontainer.json",
	status: "running",
	dirty: false,
	container: MockWorkspaceAgentContainer,
	agent: {
		id: MockWorkspaceSubAgent.id,
		name: MockWorkspaceSubAgent.name,
		directory: MockWorkspaceSubAgent?.directory ?? "/workspace/test",
	},
};

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
};

export default meta;
type Story = StoryObj<typeof AgentDevcontainerCard>;

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
		subAgents: [MockWorkspaceSubAgent],
	},
};

export const Dirty: Story = {
	args: {
		devcontainer: {
			...MockWorkspaceAgentDevcontainer,
			dirty: true,
			container: {
				...MockWorkspaceAgentContainer,
				ports: MockWorkspaceAgentContainerPorts,
			},
		},
		subAgents: [MockWorkspaceSubAgent],
	},
};

export const Recreating: Story = {
	args: {
		devcontainer: {
			...MockWorkspaceAgentDevcontainer,
			dirty: true,
			status: "starting",
			container: {
				...MockWorkspaceAgentContainer,
				ports: MockWorkspaceAgentContainerPorts,
			},
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
		devcontainer: {
			...MockWorkspaceAgentDevcontainer,
			container: {
				...MockWorkspaceAgentContainer,
			},
		},
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
