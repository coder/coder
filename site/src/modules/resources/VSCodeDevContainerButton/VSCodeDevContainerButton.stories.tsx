import type { Meta, StoryObj } from "@storybook/react";
import { MockWorkspace, MockWorkspaceAgent } from "testHelpers/entities";
import { VSCodeDevContainerButton } from "./VSCodeDevContainerButton";

const meta: Meta<typeof VSCodeDevContainerButton> = {
	title: "modules/resources/VSCodeDevContainerButton",
	component: VSCodeDevContainerButton,
};

export default meta;
type Story = StoryObj<typeof VSCodeDevContainerButton>;

export const Default: Story = {
	args: {
		userName: MockWorkspace.owner_name,
		workspaceName: MockWorkspace.name,
		agentName: MockWorkspaceAgent.name,
		devContainerName: "musing_ride",
		devContainerFolder: "/workspace/coder",
		displayApps: [
			"vscode",
			"vscode_insiders",
			"port_forwarding_helper",
			"ssh_helper",
			"web_terminal",
		],
	},
};

export const VSCodeOnly: Story = {
	args: {
		userName: MockWorkspace.owner_name,
		workspaceName: MockWorkspace.name,
		agentName: MockWorkspaceAgent.name,
		devContainerName: "nifty_borg",
		devContainerFolder: "/workspace/coder",
		displayApps: [
			"vscode",
			"port_forwarding_helper",
			"ssh_helper",
			"web_terminal",
		],
	},
};

export const InsidersOnly: Story = {
	args: {
		userName: MockWorkspace.owner_name,
		workspaceName: MockWorkspace.name,
		agentName: MockWorkspaceAgent.name,
		devContainerName: "amazing_swartz",
		devContainerFolder: "/workspace/coder",
		displayApps: [
			"vscode_insiders",
			"port_forwarding_helper",
			"ssh_helper",
			"web_terminal",
		],
	},
};
