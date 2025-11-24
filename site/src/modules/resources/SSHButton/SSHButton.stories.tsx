import {
	MockDeploymentSSH,
	MockWorkspace,
	MockWorkspaceAgent,
} from "testHelpers/entities";
import { withDesktopViewport } from "testHelpers/storybook";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { API } from "api/api";
import { spyOn, userEvent, within } from "storybook/test";
import { AgentSSHButton } from "./SSHButton";

const meta: Meta<typeof AgentSSHButton> = {
	title: "modules/resources/AgentSSHButton",
	component: AgentSSHButton,
};

export default meta;
type Story = StoryObj<typeof AgentSSHButton>;

export const Closed: Story = {
	args: {
		workspaceName: MockWorkspace.name,
		agentName: MockWorkspaceAgent.name,
	},
};

export const Opened: Story = {
	beforeEach: () => {
		spyOn(API, "getDeploymentSSHConfig").mockResolvedValue(MockDeploymentSSH);
	},
	args: {
		workspaceName: MockWorkspace.name,
		agentName: MockWorkspaceAgent.name,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const button = canvas.getByRole("button");
		await userEvent.click(button);
	},
	decorators: [withDesktopViewport],
};
