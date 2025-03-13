import type { Meta, StoryObj } from "@storybook/react";
import { AgentDevcontainerCard } from "./AgentDevcontainerCard";
import {
	MockWorkspace,
	MockWorkspaceAgentDevcontainer,
	MockWorkspaceAgentDevcontainerPorts,
} from "testHelpers/entities";

const meta: Meta<typeof AgentDevcontainerCard> = {
	title: "modules/resources/AgentDevcontainerCard",
	component: AgentDevcontainerCard,
	args: {
		container: MockWorkspaceAgentDevcontainer,
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
			...MockWorkspaceAgentDevcontainer,
			ports: MockWorkspaceAgentDevcontainerPorts,
		},
	},
};
