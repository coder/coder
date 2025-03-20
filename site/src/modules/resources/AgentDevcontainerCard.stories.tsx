import type { Meta, StoryObj } from "@storybook/react";
import {
	MockWorkspace,
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
		agentName: "dev",
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
