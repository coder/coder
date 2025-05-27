import type { Meta, StoryObj } from "@storybook/react";
import { within, userEvent } from "@storybook/test";
import {
	MockWorkspace,
	MockWorkspaceAgent,
	MockWorkspaceAgentContainer,
	MockWorkspaceAgentContainerPorts,
} from "testHelpers/entities";
import { AgentDevcontainerCard } from "./AgentDevcontainerCard";

const meta: Meta<typeof AgentDevcontainerCard> = {
	title: "modules/resources/AgentDevcontainerCard",
	component: AgentDevcontainerCard,
	args: {
		container: MockWorkspaceAgentContainer,
		workspace: MockWorkspace,
		wildcardHostname: "*.wildcard.hostname",
		agent: MockWorkspaceAgent,
	},
};

export default meta;
type Story = StoryObj<typeof AgentDevcontainerCard>;

export const NoPorts: Story = {};

export const WithPorts: Story = {
	args: {
		container: {
			...MockWorkspaceAgentContainer,
			ports: MockWorkspaceAgentContainerPorts,
		},
	},
};

export const Dirty: Story = {
	args: {
		container: {
			...MockWorkspaceAgentContainer,
			devcontainer_dirty: true,
			ports: MockWorkspaceAgentContainerPorts,
		},
	},
};

export const Recreating: Story = {
	args: {
		container: {
			...MockWorkspaceAgentContainer,
			devcontainer_dirty: true,
			ports: MockWorkspaceAgentContainerPorts,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const recreateButton = await canvas.findByRole("button", {
			name: /recreate/i,
		});
		await userEvent.click(recreateButton);
	},
};
