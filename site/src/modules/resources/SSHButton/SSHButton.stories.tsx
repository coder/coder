import type { Meta, StoryObj } from "@storybook/react-vite";
import { useQuery } from "react-query";
import { spyOn, userEvent, within } from "storybook/test";
import { API } from "#/api/api";
import {
	deploymentSSHConfig,
	deploymentSSHConfigQueryKey,
} from "#/api/queries/deployment";
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

/** No connection command is offered before the first request settles. */
export const OpenedLoading: Story = {
	...Opened,
	beforeEach: () => {
		spyOn(API, "getDeploymentSSHConfig").mockReturnValue(new Promise(() => {}));
	},
};

/** Initial request failures leave the CLI fallback usable beside the error. */
export const OpenedWithError: Story = {
	...Opened,
	beforeEach: () => {
		spyOn(API, "getDeploymentSSHConfig").mockRejectedValue(
			mockApiError({ message: "Unable to load deployment SSH configuration." }),
		);
	},
};

/** A cached suffix stays usable when refreshing the deployment settings fails. */
export const OpenedWithRefetchError: Story = {
	...OpenedWithError,
	parameters: {
		queries: [{ key: deploymentSSHConfigQueryKey, data: MockDeploymentSSH }],
	},
	decorators: [
		withDesktopViewport,
		(Story) => {
			useQuery({ ...deploymentSSHConfig(), refetchOnMount: "always" });
			return <Story />;
		},
	],
};

const mockDeploymentSSHWithoutHostnameSuffix = {
	...MockDeploymentSSH,
	hostname_suffix: "",
};

/** Deployments without a hostname suffix do not require SSH configuration. */
export const OpenedWithoutHostnameSuffix: Story = {
	...Opened,
	parameters: {
		queries: [
			{
				key: deploymentSSHConfigQueryKey,
				data: mockDeploymentSSHWithoutHostnameSuffix,
			},
		],
	},
};
