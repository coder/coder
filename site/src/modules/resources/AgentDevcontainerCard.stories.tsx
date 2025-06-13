import type { Meta, StoryObj } from "@storybook/react";
import type { WorkspaceAgentDevcontainer } from "api/typesGenerated";
import {
	MockWorkspace,
	MockWorkspaceAgent,
	MockWorkspaceAgentContainer,
	MockWorkspaceAgentContainerPorts,
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
		id: "test-agent-id",
		name: "test-devcontainer-agent",
		directory: "/workspace/test",
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
	},
};
