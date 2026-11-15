import type { Meta, StoryObj } from "@storybook/react-vite";
import { useQuery } from "react-query";
import { spyOn, userEvent, within } from "storybook/test";
import { API } from "#/api/api";
import {
	deploymentSSHConfig,
	deploymentSSHConfigQueryKey,
} from "#/api/queries/deployment";
import type { Workspace } from "#/api/typesGenerated";
import {
	MockDeploymentSSH,
	MockPermissions,
	MockUserMember,
	MockUserOwner,
	MockWorkspace,
	MockWorkspaceAgent,
	MockWorkspaceResourceMultipleAgents,
	mockApiError,
} from "#/testHelpers/entities";
import { withAuthProvider, withDesktopViewport } from "#/testHelpers/storybook";
import { AgentSSHButton } from "./SSHButton";

const mockWorkspaceWithMultipleAgents: Workspace = {
	...MockWorkspace,
	latest_build: {
		...MockWorkspace.latest_build,
		resources: [MockWorkspaceResourceMultipleAgents],
	},
};

const meta: Meta<typeof AgentSSHButton> = {
	title: "modules/resources/AgentSSHButton",
	component: AgentSSHButton,
	args: {
		workspace: mockWorkspaceWithMultipleAgents,
		agentName: MockWorkspaceAgent.name,
	},
	parameters: {
		user: MockUserOwner,
		permissions: MockPermissions,
	},
	decorators: [withAuthProvider],
};

export default meta;
type Story = StoryObj<typeof AgentSSHButton>;

export const Closed: Story = {};

export const Opened: Story = {
	beforeEach: () => {
		spyOn(API, "getDeploymentSSHConfig").mockResolvedValue(MockDeploymentSSH);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const button = canvas.getByRole("button", { name: "Connect via SSH" });
		await userEvent.click(button);
	},
	decorators: [withDesktopViewport],
};

/** The owner's only agent can be reached through the workspace hostname. */
export const OpenedSingleAgent: Story = {
	...Opened,
	args: { workspace: MockWorkspace },
};

const mockWorkspaceWithPrefixCollision: Workspace = {
	...MockWorkspace,
	name: "coder",
	latest_build: {
		...MockWorkspace.latest_build,
		workspace_name: "coder",
	},
};

const mockDeploymentSSHWithPrefixCollision = {
	...MockDeploymentSSH,
	hostname_prefix: "coder.",
	hostname_suffix: "dev",
};

/** The legacy prefix must not capture a workspace's suffix-based hostname. */
export const OpenedWithLegacyPrefixCollision: Story = {
	...OpenedSingleAgent,
	args: { workspace: mockWorkspaceWithPrefixCollision },
	parameters: {
		queries: [
			{
				key: deploymentSSHConfigQueryKey,
				data: mockDeploymentSSHWithPrefixCollision,
			},
		],
	},
};

/** Shared SSH hostnames must keep the agent and workspace owner explicit. */
export const OpenedSharedSingleAgent: Story = {
	...OpenedSingleAgent,
	parameters: { user: MockUserMember },
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

/** The CLI selects the only agent without an explicit agent suffix. */
export const OpenedSingleAgentWithoutHostnameSuffix: Story = {
	...OpenedWithoutHostnameSuffix,
	args: { workspace: MockWorkspace },
};

/** Unlike SSH hostnames, CLI targets can name an owner without naming an agent. */
export const OpenedSharedSingleAgentWithoutHostnameSuffix: Story = {
	...OpenedSingleAgentWithoutHostnameSuffix,
	parameters: {
		...OpenedSingleAgentWithoutHostnameSuffix.parameters,
		user: MockUserMember,
	},
};
