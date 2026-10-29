import type { Meta, StoryObj } from "@storybook/react-vite";
import { spyOn, userEvent, within } from "storybook/test";
import { API } from "#/api/api";
import {
	MockDeploymentSSH,
	MockWorkspace,
	MockWorkspaceAgent,
	mockApiError,
} from "#/testHelpers/entities";
import { withDesktopViewport } from "#/testHelpers/storybook";
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
		workspaceOwnerUsername: MockWorkspace.owner_name,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const button = canvas.getByRole("button");
		await userEvent.click(button);
	},
	decorators: [withDesktopViewport],
};

export const OpenedLoading: Story = {
	...Opened,
	beforeEach: () => {
		spyOn(API, "getDeploymentSSHConfig").mockReturnValue(new Promise(() => {}));
	},
};

export const OpenedWithError: Story = {
	...Opened,
	beforeEach: () => {
		spyOn(API, "getDeploymentSSHConfig").mockRejectedValue(
			mockApiError({ message: "Unable to load deployment SSH configuration." }),
		);
	},
};

export const OpenedWithoutHostnameSuffix: Story = {
	...Opened,
	beforeEach: () => {
		const mockDeploymentSSH = { ...MockDeploymentSSH, hostname_suffix: "" };
		spyOn(API, "getDeploymentSSHConfig").mockResolvedValue(mockDeploymentSSH);
	},
};
